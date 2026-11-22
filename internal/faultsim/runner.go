package faultsim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"sort"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type result struct {
	Result   statemachine.Result
	Original uint64
}
type simNode struct {
	cfg         config.Config
	core        *raft.Core
	disk        []byte
	synced      int
	state       wal.State
	applier     *applier.Applier
	results     map[uint64]result
	incarnation int
	fault, cut  string
}
type event struct {
	at, id            uint64
	kind, node        string
	action            *Action
	request, response peerprotocol.Message
	incarnation       int
}
type runner struct {
	applied                       map[uint64]result
	ctx                           context.Context
	m                             Manifest
	replay                        *Manifest
	randomState                   uint64
	drawIndex                     int
	now, sequence, networkOrdinal uint64
	queue                         []event
	nodes                         [3]*simNode
	blocked                       map[string]bool
	delay, drop, duplicate        uint64
	ghost                         map[uint64]storageformat.LogEntry
	leaders                       map[uint64]string
	votes                         map[string]string
	failure                       error
}

func index(id string) int                                   { return int(id[1] - '1') }
func Run(ctx context.Context, s Scenario) (Manifest, error) { return execute(ctx, s, nil) }
func Replay(ctx context.Context, m Manifest) (Manifest, error) {
	raw, e := json.Marshal(m)
	if e != nil {
		return Manifest{}, e
	}
	checked, e := ReadManifest(raw)
	if e != nil {
		return Manifest{}, e
	}
	return execute(ctx, checked.Scenario, &checked)
}
func execute(ctx context.Context, s Scenario, replay *Manifest) (out Manifest, err error) {
	if ctx == nil || validate(s) != nil {
		return out, ErrInput
	}
	r := &runner{ctx: ctx, replay: replay, randomState: s.Seed, blocked: map[string]bool{}, ghost: map[uint64]storageformat.LogEntry{}, applied: map[uint64]result{}, leaders: map[uint64]string{}, votes: map[string]string{}}
	revision, modified := sourceVersion()
	c, _ := config.Parse(s.Configs[0])
	r.m = Manifest{CoreVersion: "raft-m11", SourceRevision: revision, SourceModified: modified, Version: 1, Harness: Version, Runtime: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH, PRNG: PRNG, ConfigHash: c.Hash(), Scenario: s, Outcome: "passed"}
	if replay != nil {
		r.m.SourceRevision = replay.SourceRevision
		r.m.SourceModified = replay.SourceModified
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		for _, n := range r.nodes {
			if n != nil && n.applier != nil {
				if e := n.applier.Shutdown(cleanup); err == nil && e != nil {
					err = e
				}
			}
		}
		if err != nil {
			out.Outcome = "failed"
			out.Failure = err.Error()
			if errors.Is(err, ErrLimit) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				out.Outcome = "inconclusive"
			}
		}
		out.SHA = ""
		out.SHA = digest(out)
	}()

	for i, raw := range s.Configs {
		c, _ := config.Parse(raw)
		initial, _ := storageformat.EncodeFrame(1, []byte(`{"commit_index":"0","hard_state":{"current_term":"0","voted_for":null}}`))
		r.nodes[i] = &simNode{cfg: c, disk: initial, synced: len(initial)}
		if e := r.restart(c.NodeID()); e != nil {
			return r.m, e
		}
	}
	for i := range s.Events {
		a := s.Events[i]
		r.add(event{at: a.At, kind: "action", node: a.Node, action: &a})
	}
	for at := uint64(25); at <= s.Deadline; at += 25 {
		for i := range r.nodes {
			r.add(event{at: at, kind: "tick", node: fmt.Sprint("n", i+1)})
		}
	}
	for len(r.queue) > 0 && r.failure == nil {
		if e := ctx.Err(); e != nil {
			r.failure = e
			break
		}
		sort.Slice(r.queue, func(i, j int) bool { a, b := r.queue[i], r.queue[j]; return a.at < b.at || a.at == b.at && a.id < b.id })
		ev := r.queue[0]
		r.queue = r.queue[1:]
		if ev.at > s.Deadline {
			break
		}
		r.now = ev.at
		if len(r.m.Trace) >= s.MaxEvents {
			r.failure = ErrLimit
			break
		}
		e := r.step(ev)
		if e != nil {
			r.failure = e
		}
		if r.failure == nil {
			r.failure = r.invariants()
		}
		r.complete()
		r.record(ev.kind, ev.node, r.eventDetail(ev))
	}
	for i := range r.m.History {
		a := &r.m.History[i]
		if a.Outcome == "pending" {
			a.Outcome = "unknown"
			a.Code = "TIMEOUT"
			a.Return = s.Deadline
		}
	}
	if replay != nil && r.failure == nil && (len(replay.Trace) != len(r.m.Trace) || len(replay.Draws) != r.drawIndex || digest(replay.History) != digest(r.m.History) || replay.ConfigHash != r.m.ConfigHash) {
		r.failure = ErrDivergence
	}
	if r.failure != nil {
		r.m.Outcome = "failed"
		r.m.Failure = r.failure.Error()
		if errors.Is(r.failure, ErrLimit) || errors.Is(r.failure, context.DeadlineExceeded) || errors.Is(r.failure, context.Canceled) {
			r.m.Outcome = "inconclusive"
		}
	}
	_, sizeErr := r.m.Bytes()
	if sizeErr != nil {
		r.failure = sizeErr
		r.m.Outcome = "inconclusive"
	}
	r.m.SHA = ""
	r.m.SHA = digest(r.m)
	return r.m, r.failure
}
func (r *runner) add(e event) {
	if len(r.queue) >= 10000 {
		r.failure = ErrLimit
		return
	}
	r.sequence++
	e.id = r.sequence
	r.queue = append(r.queue, e)
}
func (r *runner) draw(min, max uint64) (uint64, error) {
	if r.failure != nil {
		return 0, r.failure
	}
	var v uint64
	if r.replay != nil {
		if r.drawIndex >= len(r.replay.Draws) {
			return 0, ErrDivergence
		}
		d := r.replay.Draws[r.drawIndex]
		if d.Min != min || d.Max != max || d.Value < min || d.Value > max {
			return 0, ErrDivergence
		}
		v = d.Value
	} else {
		x := r.randomState
		x ^= x << 13
		x ^= x >> 7
		x ^= x << 17
		r.randomState = x
		v = min + x%(max-min+1)
	}
	r.drawIndex++
	r.m.Draws = append(r.m.Draws, Draw{min, max, v})
	return v, nil
}
func (r *runner) restart(id string) error {
	n := r.nodes[index(id)]
	if n.core != nil {
		return ErrInput
	}
	recovered, e := wal.Replay(bytes.NewReader(n.disk), 0, 0)
	if e != nil {
		return e
	}
	n.disk = n.disk[:recovered.ValidBytes]
	if n.synced > len(n.disk) {
		n.synced = len(n.disk)
	}
	n.state = recovered.State
	n.incarnation++
	epoch := fmt.Sprintf("%032x", n.incarnation*10+index(id)+1)
	n.core, e = raft.New(n.cfg, n.state, "gen-1", epoch, r.draw, r.now)
	if e != nil {
		return e
	}
	if e = n.core.EnableReplication(16); e != nil {
		return e
	}
	if e = n.core.EnableLoggedReads(); e != nil {
		return e
	}
	if e = n.core.EnableCommit(); e != nil {
		return e
	}
	n.applier, e = applier.New(committed.Wrap(committed.NewSessions(statemachine.New())), applier.Config{Capacity: 1})
	if e != nil {
		return e
	}
	n.results = map[uint64]result{}
	return r.apply(id)
}
func (r *runner) apply(id string) error {
	n := r.nodes[index(id)]
	if n.core.Status().Failure != nil {
		return nil
	}
	for n.core.Status().LastApplied < n.core.Status().CommitIndex {
		i := n.core.Status().LastApplied + 1
		e := n.state.Entries[i-1]
		if old, ok := r.ghost[i]; ok && !bytes.Equal(old.Raw, e.Raw) {
			return fmt.Errorf("committed prefix changed at %d", i)
		}
		r.ghost[i] = e
		cmd, err := committed.ProjectWithSessions(e)
		if err != nil {
			return err
		}
		f, err := n.applier.TrySubmit(applier.Entry{Index: i, Command: cmd})
		if err != nil {
			return err
		}
		if err = n.core.Scheduled(i); err != nil {
			return err
		}
		o, err := f.Wait(r.ctx)
		if err != nil {
			return err
		}
		v, original, err := committed.DecodeSessionResult(o.Result, i)
		if err != nil {
			return err
		}
		if err = n.core.Applied(1, i, nil); err != nil {
			return err
		}
		current := result{v, original}
		if old, ok := r.applied[i]; ok && digest(old) != digest(current) {
			return fmt.Errorf("state machine result invariant at %d", i)
		}
		r.applied[i] = current
		n.results[i] = current
	}
	return nil
}
func (r *runner) drive(id string, e raft.Effects, err error, request *peerprotocol.Message) error {
	n := r.nodes[index(id)]
	// A rejected/stale protocol input may legitimately return an error; fatal
	// storage errors are represented in core status and exercised through restart.
	if err != nil {
		if n.core.Status().Failure != nil {
			return nil
		}
		return err
	}
	for e.Persist != nil {
		p := e.Persist
		frame, er := storageformat.EncodeFrame(n.state.Sequence+1, p.Bytes)
		if er != nil {
			return er
		}
		var ioErr error
		fault := n.fault
		matches := fault != "" && (n.cut != "commit.sync" || bytes.Contains(p.Bytes, []byte(`"commit_index"`)))
		if matches {
			n.fault = ""
			switch fault {
			case "short_write":
				cut := 12
				if n.cut != "wal.header" {
					cut = len(frame) - 1
				}
				n.disk = append(n.disk, frame[:cut]...)
				ioErr = wal.ErrFailed
			case "sync_fail":
				n.disk = append(n.disk, frame...)
				ioErr = wal.ErrFailed
			case "enospc":
				ioErr = wal.ErrFailed
				if p.Proposal {
					ioErr = wal.ErrLimit
				}
			}
			r.record("io_fault", id, fault+":"+n.cut)
		} else {
			n.disk = append(n.disk, frame...)
			recovery, er := wal.Replay(bytes.NewReader(n.disk), 0, 0)
			if er != nil {
				return er
			}
			n.state = recovery.State
			n.synced = len(n.disk)
		}
		token := wal.Token{Generation: "gen-1", Sequence: n.state.Sequence}
		e, err = n.core.Durable(p.ID, token, ioErr)
		if ioErr == nil {
			r.record("disk_sync", id, fmt.Sprint(n.state.Sequence))
		}
		if err != nil {
			if n.core.Status().Failure != nil {
				return nil
			}
			return err
		}
	}
	if err := r.apply(id); err != nil {
		return err
	}
	for _, m := range e.Send {
		if err := n.core.Sent(m); err != nil {
			return err
		}
		r.send(m, peerprotocol.Message{})
	}
	if e.Reply != nil {
		if request == nil {
			return ErrInput
		}
		r.send(*request, *e.Reply)
	}
	return nil
}
func (r *runner) send(req, res peerprotocol.Message) {
	msg := req
	kind := "request"
	if len(res.Bytes()) > 0 {
		msg = res
		kind = "response"
	}
	env := msg.Envelope()
	if kind == "request" {
		r.add(event{at: r.now + 250, kind: "rpc_timeout", node: env.From, request: req, incarnation: r.nodes[index(env.From)].incarnation})
	}
	r.networkOrdinal++
	n := r.nodes[index(env.To)]
	drop := r.blocked[env.From+env.To] || n.core == nil || (r.drop > 0 && r.networkOrdinal%r.drop == 0)
	if drop {
		r.record("drop", env.From, env.RPCID)
		return
	}
	delay, err := r.draw(1, r.delay+1)
	if err != nil {
		r.failure = err
		return
	}
	ev := event{at: r.now + delay, kind: kind, node: env.To, request: req, response: res, incarnation: n.incarnation}
	r.add(ev)
	r.record("send", env.From, env.RPCID)
	if r.duplicate > 0 && r.networkOrdinal%r.duplicate == 0 {
		ev.at++
		r.add(ev)
		r.record("duplicate", env.From, env.RPCID)
	}
}
func (r *runner) step(ev event) error {
	if ev.kind == "action" {
		return r.action(*ev.action)
	}
	n := r.nodes[index(ev.node)]
	if n.core == nil {
		return nil
	}
	if ev.kind == "rpc_timeout" {
		n.core.Failed(ev.request)
		return nil
	}
	if ev.kind == "tick" {
		if n.core.Status().Failure != nil {
			return nil
		}
		e, err := n.core.Tick(r.now)
		return r.drive(ev.node, e, err, nil)
	}
	// Requests sent to the previous dead instance are discarded. Responses already
	// in the network are intentionally delivered across restart; wire correlation
	// and higher-term rules, not simulator incarnation, decide their usefulness.
	if ev.kind == "request" && ev.incarnation != n.incarnation {
		return nil
	}
	if n.core.Status().Failure != nil {
		return nil
	}
	if ev.kind == "request" {
		e, err := n.core.Request(r.now, ev.request)
		return r.drive(ev.node, e, err, &ev.request)
	}
	e, err := n.core.Response(r.now, ev.request, ev.response)
	return r.drive(ev.node, e, err, nil)
}
func (r *runner) action(a Action) error {
	switch a.Action {
	case "partition":
		r.blocked[a.From+a.To] = true
		if a.Both {
			r.blocked[a.To+a.From] = true
		}
	case "heal":
		delete(r.blocked, a.From+a.To)
		if a.Both {
			delete(r.blocked, a.To+a.From)
		}
	case "network":
		r.delay = a.Delay
		r.drop = a.DropEvery
		r.duplicate = a.DuplicateEvery
	case "storage":
		n := r.nodes[index(a.Node)]
		n.fault = a.Fault
		n.cut = a.Cutpoint
	case "crash":
		n := r.nodes[index(a.Node)]
		if n.core == nil {
			return ErrInput
		}
		n.core.Stop()
		n.core = nil
		if e := n.applier.Shutdown(r.ctx); e != nil {
			return e
		}
		n.applier = nil
		n.results = nil
		if r.m.Scenario.Model == "SystemCrashModel" && a.Survival == "synced" {
			n.disk = n.disk[:n.synced]
		}
	case "restart":
		return r.restart(a.Node)
	case "propose":
		id := a.Node
		if id == "leader" {
			id = ""
			for _, n := range r.nodes {
				if n.core != nil && n.core.Status().Role == raft.Leader && n.core.Status().Failure == nil {
					if id != "" {
						return fmt.Errorf("ambiguous leader precondition")
					}
					id = n.cfg.NodeID()
				}
			}
		}
		h := Attempt{ID: len(r.m.History) + 1, Node: id, Kind: a.Kind, Command: bytes.Clone(a.Command), Invoke: r.now, Deadline: r.now + a.Timeout, LoseResponse: a.LoseResponse, Outcome: "not_admitted", Code: "NO_LEADER"}
		if id != "" {
			n := r.nodes[index(id)]
			if n.core != nil && n.core.Status().Role == raft.Leader && n.core.Status().Failure == nil {
				e, err := n.core.AppendLocal(r.now, a.Kind, a.Command)
				var receipt raft.LocalAppend
				if e.Local != nil {
					receipt = *e.Local
				}
				// Local receipt emerges after Durable; the new index is known before I/O.
				h.Index = n.core.Status().LastIndex + 1
				h.Term = n.core.Status().Term
				h.Incarnation = n.incarnation
				h.Outcome = "pending"
				h.Code = ""
				if err != nil {
					h.Outcome = "not_admitted"
					h.Code = "INVALID_ARGUMENT"
				} else {
					if er := r.drive(id, e, nil, nil); er != nil {
						return er
					}
					if receipt.Index > 0 {
						h.Index = receipt.Index
					}
					if n.core.Status().LastIndex < h.Index && n.core.Status().Failure == nil {
						h.Outcome = "not_admitted"
						h.Code = "STORAGE_UNAVAILABLE"
					}
				}
			}
		}
		if h.Outcome == "not_admitted" {
			h.Return = r.now
		}
		r.m.History = append(r.m.History, h)
	}
	return nil
}
func (r *runner) complete() {
	for i := range r.m.History {
		h := &r.m.History[i]
		if h.Outcome != "pending" {
			continue
		}
		n := r.nodes[index(h.Node)]
		if n.core == nil || n.incarnation != h.Incarnation || n.core.Status().Failure != nil || n.core.Status().Role != raft.Leader || n.core.Status().Term != h.Term {
			h.Outcome = "unknown"
			h.Code = "UNAVAILABLE"
			h.Return = r.now
			continue
		}
		if r.now >= h.Deadline {
			h.Outcome = "unknown"
			h.Code = "TIMEOUT"
			h.Return = r.now
			continue
		}
		if result, ok := n.results[h.Index]; ok && !h.LoseResponse && n.core.Status().CommitIndex >= h.Index {
			h.Outcome = "completed"
			h.Code = string(result.Result.Code)
			h.Return = r.now
			h.OriginalIndex = result.Original
			h.Applied = result.Result.Applied
			h.Deleted = result.Result.Deleted
			h.Exists = result.Result.Exists
			h.Value = bytes.Clone(result.Result.Value)
		}
	}
}
func (r *runner) invariants() error {
	for _, n := range r.nodes {
		// Replay validation independently enforces vote/term and committed-prefix
		// storage transitions, including speculative suffix repair boundaries.
		disk, e := wal.Replay(bytes.NewReader(n.disk), 0, 0)
		if e != nil {
			return e
		}
		if n.core == nil {
			continue
		}
		s := n.core.Status()
		if s.LastApplied > s.ScheduledApplyIndex || s.ScheduledApplyIndex > s.CommitIndex || s.CommitIndex > s.LastIndex || s.CommitIndex > disk.State.CommitIndex {
			return fmt.Errorf("apply/commit invariant")
		}
		if disk.State.HardState.Vote != "" {
			k := fmt.Sprintf("%s/%d", n.cfg.NodeID(), disk.State.HardState.Term)
			if old := r.votes[k]; old != "" && old != disk.State.HardState.Vote {
				return fmt.Errorf("vote invariant")
			}
			r.votes[k] = disk.State.HardState.Vote
		}
		entries := n.core.LogEntries()
		for i := uint64(1); i <= s.CommitIndex; i++ {
			entry := entries[i-1]
			if old, ok := r.ghost[i]; ok && !bytes.Equal(old.Raw, entry.Raw) {
				return fmt.Errorf("committed prefix invariant")
			}
			r.ghost[i] = entry
		}
		if s.Role == raft.Leader {
			if old := r.leaders[s.Term]; old != "" && old != n.cfg.NodeID() {
				return fmt.Errorf("election safety")
			}
			first := r.leaders[s.Term] == ""
			r.leaders[s.Term] = n.cfg.NodeID()
			for i, g := range r.ghost {
				if !first {
					break
				}
				if i > uint64(len(entries)) || !bytes.Equal(g.Raw, entries[i-1].Raw) {
					return fmt.Errorf("leader completeness")
				}
			}
		}
	}
	for i, a := range r.nodes {
		if a.core == nil {
			continue
		}
		for _, b := range r.nodes[i+1:] {
			if b.core == nil {
				continue
			}
			aa, bb := a.core.LogEntries(), b.core.LogEntries()
			for k := 0; k < len(aa) && k < len(bb); k++ {
				if aa[k].Term == bb[k].Term {
					for j := 0; j <= k; j++ {
						if !bytes.Equal(aa[j].Raw, bb[j].Raw) {
							return fmt.Errorf("log matching")
						}
					}
				}
			}
		}
	}
	return nil
}
func (r *runner) eventDetail(ev event) string {
	if ev.action != nil {
		return ev.action.Action
	}
	if ev.kind == "tick" {
		return ""
	}
	return ev.request.Envelope().RPCID
}
func (r *runner) record(kind, node, detail string) {
	if r.failure != nil {
		return
	}
	if len(r.m.Trace) >= r.m.Scenario.MaxEvents {
		r.failure = ErrLimit
		return
	}
	states := make([]any, 0, 3)
	for _, n := range r.nodes {
		if n == nil {
			continue
		}
		var s any
		if n.core != nil {
			st := n.core.Status()
			s = struct {
				Role                        raft.Role
				Term, Commit, Applied, Last uint64
				Vote, Leader                string
				Failed                      bool
				Progress                    [3]raft.Progress
			}{st.Role, st.Term, st.CommitIndex, st.LastApplied, st.LastIndex, st.Vote, st.Leader, st.Failure != nil, st.Progress}
		}
		states = append(states, []any{n.cfg.NodeID(), n.incarnation, digest(n.disk), s, n.results, n.fault, n.cut})
	}
	queue := make([]any, 0, len(r.queue))
	for _, e := range r.queue {
		queue = append(queue, []any{e.at, e.id, e.kind, e.node, e.incarnation, digest(e.request.Bytes()), digest(e.response.Bytes())})
	}
	t := Trace{r.now, uint64(len(r.m.Trace) + 1), kind, node, detail, digest([]any{states, queue, r.blocked, r.m.History})}
	if r.replay != nil {
		i := len(r.m.Trace)
		if i >= len(r.replay.Trace) || t != r.replay.Trace[i] {
			r.failure = fmt.Errorf("%w at event %d", ErrDivergence, i+1)
			return
		}
	}
	r.m.Trace = append(r.m.Trace, t)
	if len(r.m.Trace)%128 == 0 {
		if b, _ := json.Marshal(r.m); len(b) > MaxEvidence {
			r.failure = ErrLimit
		}
	}
}

// Minimize removes explicit actions only when the supplied failure predicate
// still holds and the runner's preconditions remain valid. It is a bounded aid,
// not a general history or linearizability checker.
func Minimize(s Scenario, fails func(Scenario) bool) Scenario {
	if fails == nil {
		return s
	}
	for i := 0; i < len(s.Events); {
		candidate := s
		candidate.Events = append(append([]Action{}, s.Events[:i]...), s.Events[i+1:]...)
		if validate(candidate) == nil && fails(candidate) {
			s = candidate
		} else {
			i++
		}
	}
	return s
}
