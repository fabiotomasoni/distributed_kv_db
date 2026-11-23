// Package snapshotinstall is a dormant M13 isolated protocol harness. No node,
// service, HTTP handler, automatic transfer or compaction trigger imports it.
package snapshotinstall

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/snapshot"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

var ErrInput = errors.New("snapshotinstall: invalid isolated state/clock/correlation")
var ErrClosed = errors.New("snapshotinstall: receiver closed")

type stream struct {
	id, leader                                    string
	term, index, boundaryTerm, total, last, epoch uint64
	data                                          []byte
}
type pendingApply struct {
	future      *applier.Future
	index, term uint64
}
type Applied struct {
	Index, Original uint64
	Result          statemachine.Result
}
type Status struct {
	Raft           raft.Status
	Generation     string
	BufferedBytes  int
	ApplyEpoch     uint64
	Closed, Failed bool
}
type View struct {
	Image  storageformat.Snapshot
	Log    wal.State
	Status Status
	Last   Applied
}

// Receiver owns one WAL/LOCK and one M02 worker, serializing all protocol events.
// Time is explicit monotonic milliseconds. Partial chunk ACKs are memory-only;
// they prove neither durability nor replication. Close/expiry discard buffering.
type Receiver struct {
	mu         sync.Mutex
	cfg        config.Config
	store      *wal.Store
	core       *raft.Core
	app        *applier.Applier
	pending    *pendingApply
	stage      *stream
	epoch, now uint64
	last       Applied
	closed     bool
	failure    error
	cut        func(string) error // private deterministic tests only
}

func epochID() (string, error) {
	var b [16]byte
	_, e := rand.Read(b[:])
	return hex.EncodeToString(b[:]), e
}
func Open(root string, c config.Config) (*Receiver, error) { return open(root, c, nil) }
func open(root string, c config.Config, wrap func(applier.Machine) applier.Machine) (*Receiver, error) {
	if !c.Valid() || !c.Sessions() {
		return nil, snapshot.ErrProfile
	}
	s, e := wal.OpenSnapshotSessions(root, wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
	if e != nil {
		return nil, e
	}
	r := &Receiver{cfg: c, store: s, epoch: 1}
	recovered, e := snapshot.RecoverCommitted(context.Background(), c, s.SnapshotImage(), s.State(), 1)
	if e != nil {
		return nil, errors.Join(e, s.Close())
	}
	if e = r.activate(recovered.Machine, recovered.LastApplied, wrap); e != nil {
		return nil, errors.Join(e, s.Close())
	}
	return r, nil
}
func (r *Receiver) activate(machine *snapshot.Machine, applied uint64, wrap func(applier.Machine) applier.Machine) error {
	epoch, e := epochID()
	if e != nil {
		return e
	}
	core, e := raft.New(r.cfg, r.store.State(), r.store.Status().Generation, epoch, func(min, max uint64) (uint64, error) { return min, nil }, r.now)
	if e != nil {
		return e
	}
	if e = core.EnableReplication(16); e != nil {
		return e
	}
	if e = core.EnableRecoveredCommit(applied, r.epoch); e != nil {
		return e
	}
	var owned applier.Machine = machine
	if wrap != nil {
		owned = wrap(owned)
	}
	app, e := applier.New(owned, applier.Config{Capacity: 1, InitialIndex: applied})
	if e != nil {
		return e
	}
	r.core = core
	r.app = app
	r.pending = nil
	r.last = Applied{}
	return nil
}
func (r *Receiver) admit(ctx context.Context, now uint64) error {
	if ctx == nil {
		return ErrInput
	}
	if r.closed {
		return ErrClosed
	}
	if r.failure != nil {
		return r.failure
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if now < r.now {
		return ErrInput
	}
	r.now = now
	if r.stage != nil && now-r.stage.last >= 5000 {
		r.stage = nil
	}
	return nil
}
func (r *Receiver) failed(e error) error { r.failure = e; r.stage = nil; return e }
func (r *Receiver) hook(stage string) error {
	if r.cut != nil {
		return r.cut(stage)
	}
	return nil
}
func (r *Receiver) drive(out raft.Effects, e error) (raft.Effects, error) {
	if e != nil {
		return out, r.failed(e)
	}
	for out.Persist != nil {
		effect := out.Persist
		var token wal.Token
		if effect.Proposal {
			token, e = r.store.AppendProposal(effect.Bytes)
		} else {
			token, e = r.store.Append(effect.Bytes)
		}
		out, e = r.core.Durable(effect.ID, token, e)
		if e != nil {
			return out, r.failed(e)
		}
	}
	return out, nil
}
func termAt(s wal.State, index uint64) (uint64, bool) {
	if index == s.BaseIndex {
		return s.BaseTerm, true
	}
	if index > s.BaseIndex && index-s.BaseIndex <= uint64(len(s.Entries)) {
		return s.Entries[index-s.BaseIndex-1].Term, true
	}
	return 0, false
}
func (r *Receiver) scheduleOne() error {
	st := r.core.Status()
	if r.pending != nil || st.ScheduledApplyIndex >= st.CommitIndex {
		return nil
	}
	index := st.ScheduledApplyIndex + 1
	log := r.store.State()
	if index <= log.BaseIndex || index-log.BaseIndex > uint64(len(log.Entries)) {
		return r.failed(ErrInput)
	}
	entry := log.Entries[index-log.BaseIndex-1]
	command, e := committed.ProjectWithSessions(entry)
	if e != nil {
		return r.failed(e)
	}
	f, e := r.app.TrySubmit(applier.Entry{Index: index, Command: command})
	if e != nil {
		return r.failed(e)
	}
	if e = r.core.Scheduled(index); e != nil {
		return r.failed(e)
	}
	r.pending = &pendingApply{f, index, entry.Term}
	return nil
}
func (r *Receiver) settle() error {
	if r.pending == nil {
		return nil
	}
	p := r.pending
	// Cancellation cannot revoke accepted work; consume its REAL completion.
	out, e := p.future.Wait(context.Background())
	if e != nil {
		return r.failed(e)
	}
	result, original, e := committed.DecodeSessionResult(out.Result, p.index)
	if e != nil {
		return r.failed(e)
	}
	if e = r.core.Applied(r.epoch, p.index, nil); e != nil {
		return r.failed(e)
	}
	r.last = Applied{p.index, original, result}
	r.pending = nil
	return nil
}
func (r *Receiver) drain() error {
	for r.core.Status().LastApplied < r.core.Status().CommitIndex {
		if e := r.scheduleOne(); e != nil {
			return e
		}
		if e := r.settle(); e != nil {
			return e
		}
	}
	return nil
}
func (r *Receiver) reply(req peerprotocol.Message, body map[string]any) (peerprotocol.Message, error) {
	env := req.Envelope()
	body["protocol_version"] = 1
	body["cluster_id"] = r.cfg.ClusterID()
	body["config_hash"] = r.cfg.Hash()
	body["from"] = r.cfg.NodeID()
	body["to"] = env.From
	body["rpc_id"] = env.RPCID
	body["term"] = fmt.Sprint(r.core.Status().Term)
	raw, _ := json.Marshal(body)
	return peerprotocol.DecodeResponse(r.cfg, req, raw)
}
func (r *Receiver) reject(req peerprotocol.Message, next uint64) (peerprotocol.Message, error) {
	return r.reply(req, map[string]any{"accepted": false, "next_offset": fmt.Sprint(next)})
}
func (r *Receiver) obsolete(req peerprotocol.Message, index uint64) (peerprotocol.Message, error) {
	log := r.store.State()
	if index < log.BaseIndex {
		index = log.BaseIndex
	}
	term, ok := termAt(log, index)
	if !ok || term == 0 {
		return peerprotocol.Message{}, r.failed(ErrInput)
	}
	r.stage = nil
	return r.reply(req, map[string]any{"accepted": false, "next_offset": "0", "reason": "obsolete", "resume_index": fmt.Sprint(index), "resume_term": fmt.Sprint(term)})
}

// Handle uses the real M05/M06/M07 core and exact synchronized WAL tokens.
// No timers, network workers or server listeners run. Append ACK may precede
// apply exactly as in Raft; the harness schedules one bounded slot per event.
func (r *Receiver) Handle(ctx context.Context, now uint64, msg peerprotocol.Message) (peerprotocol.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	checked, e := peerprotocol.DecodeRequest(r.cfg, msg.Kind(), msg.Bytes(), r.cfg.NodeID())
	if e != nil {
		return peerprotocol.Message{}, e
	}
	if e = r.admit(ctx, now); e != nil {
		return peerprotocol.Message{}, e
	}
	out, e := r.drive(r.core.Request(now, checked))
	if e != nil {
		return peerprotocol.Message{}, e
	}
	st := r.core.Status()
	if r.stage != nil && (r.stage.term != st.Term || r.stage.leader != st.Leader || r.stage.epoch != r.epoch) {
		r.stage = nil
	}
	if checked.Kind() != peerprotocol.Snapshot {
		if e = r.scheduleOne(); e != nil {
			return peerprotocol.Message{}, e
		}
		if out.Reply == nil {
			return peerprotocol.Message{}, ErrInput
		}
		return *out.Reply, nil
	}
	if out.Rejected != nil && !errors.Is(out.Rejected, raft.ErrUnsupported) {
		return peerprotocol.Message{}, out.Rejected
	}
	if checked.Envelope().Term != st.Term || st.Leader != checked.Envelope().From {
		return r.reject(checked, 0)
	}
	var body struct {
		ID     string `json:"snapshot_id"`
		Index  string `json:"last_included_index"`
		Term   string `json:"last_included_term"`
		Offset string `json:"offset"`
		Data   []byte `json:"data_b64"`
		Done   bool   `json:"done"`
		Total  string `json:"total_bytes"`
	}
	if e = json.Unmarshal(checked.Bytes(), &body); e != nil {
		return peerprotocol.Message{}, e
	}
	index, _ := parseUint(body.Index)
	term, _ := parseUint(body.Term)
	offset, _ := parseUint(body.Offset)
	total, _ := parseUint(body.Total)
	if index <= st.LastApplied {
		return r.obsolete(checked, index)
	}
	if r.stage == nil {
		if offset != 0 {
			return r.reject(checked, 0)
		}
		r.stage = &stream{id: body.ID, leader: st.Leader, term: st.Term, index: index, boundaryTerm: term, total: total, last: now, epoch: r.epoch}
	}
	s := r.stage
	if s.id != body.ID {
		return r.reject(checked, 0)
	}
	if s.index != index || s.boundaryTerm != term || s.total != total {
		r.stage = nil
		return r.reject(checked, 0)
	}
	end := offset + uint64(len(body.Data))
	next := uint64(len(s.data))
	if offset > next {
		return r.reject(checked, next)
	}
	if offset < next {
		if end > next || !bytes.Equal(s.data[int(offset):int(end)], body.Data) {
			r.stage = nil
			return r.reject(checked, 0)
		}
	} else {
		s.data = append(s.data, body.Data...)
	}
	s.last = now
	if !body.Done {
		return r.reply(checked, map[string]any{"accepted": true, "next_offset": fmt.Sprint(len(s.data))})
	}
	sum := sha256.Sum256(s.data)
	image, e := storageformat.DecodeSnapshot(s.data)
	if e != nil || hex.EncodeToString(sum[:]) != s.id || image.Index != s.index || image.Term != s.boundaryTerm || image.ClusterID != r.cfg.ClusterID() || image.ConfigHash != r.cfg.Hash() {
		r.stage = nil
		return r.reject(checked, 0)
	}
	if e = r.hook("image.validated"); e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	return r.install(ctx, checked, s)
}
func (r *Receiver) install(ctx context.Context, req peerprotocol.Message, s *stream) (peerprotocol.Message, error) {
	b, e := r.app.Park(ctx)
	if e != nil {
		return peerprotocol.Message{}, e
	}
	defer b.Release(context.Background())
	if e = r.settle(); e != nil {
		return peerprotocol.Message{}, e
	}
	st := r.core.Status()
	if st.LastApplied != b.Index() || st.ScheduledApplyIndex != b.Index() || st.Term != s.term || st.Leader != s.leader || r.epoch != s.epoch {
		return peerprotocol.Message{}, r.failed(ErrInput)
	}
	if s.index <= st.LastApplied {
		return r.obsolete(req, s.index)
	}
	plan, e := snapshot.PlanInstallation(r.cfg, s.data, r.store.State(), st.LastApplied, s.term)
	if e != nil {
		r.stage = nil
		return peerprotocol.Message{}, e
	}
	if plan.LastIndex() == math.MaxUint64 || r.epoch == math.MaxUint64 {
		r.stage = nil
		return r.reject(req, 0)
	}
	if e = b.Release(context.Background()); e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	if e = r.app.Shutdown(context.Background()); e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	if e = r.hook("before-generation"); e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	e = r.store.TransitionSnapshot(func(root string, owner *storagefs.Lock, local wal.State) error {
		_, e := snapshot.PublishReceived(ctx, r.cfg, root, owner, local, s.data, st.LastApplied, s.term)
		return e
	})
	if e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	installedView := r.store.State()
	installedHash := sha256.Sum256(r.store.SnapshotImage())
	if installedView.BaseIndex != s.index || installedView.BaseTerm != s.boundaryTerm || hex.EncodeToString(installedHash[:]) != s.id {
		return peerprotocol.Message{}, r.failed(ErrInput)
	}
	if e = r.hook("generation.published"); e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	machine, point, e := snapshot.Restore(r.cfg, r.store.SnapshotImage(), r.store.State().HardState.Term)
	if e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	r.epoch++
	if e = r.activate(machine, point.Index, nil); e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	if e = r.hook("memory.installed"); e != nil {
		return peerprotocol.Message{}, r.failed(e)
	}
	if e = r.drain(); e != nil {
		return peerprotocol.Message{}, e
	}
	r.stage = nil
	return r.reply(req, map[string]any{"accepted": true, "next_offset": fmt.Sprint(s.total), "installed_index": fmt.Sprint(s.index)})
}
func (r *Receiver) status() Status {
	buffered := 0
	if r.stage != nil {
		buffered = len(r.stage.data)
	}
	return Status{r.core.Status(), r.store.Status().Generation, buffered, r.epoch, r.closed, r.failure != nil}
}
func (r *Receiver) Status() Status { r.mu.Lock(); defer r.mu.Unlock(); return r.status() }
func (r *Receiver) Expire(now uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.admit(context.Background(), now)
}
func (r *Receiver) View(ctx context.Context) (View, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if e := r.admit(ctx, r.now); e != nil {
		return View{}, e
	}
	if e := r.drain(); e != nil {
		return View{}, e
	}
	b, e := r.app.Park(ctx)
	if e != nil {
		return View{}, e
	}
	defer b.Release(context.Background())
	st := r.core.Status()
	log := r.store.State()
	term, ok := termAt(log, st.LastApplied)
	if !ok {
		return View{}, ErrInput
	}
	image, e := snapshot.Capture(ctx, b, snapshot.Proof{LastApplied: st.LastApplied, ScheduledApplyIndex: st.ScheduledApplyIndex, CommitIndex: st.CommitIndex, ApplyEpoch: 1, TermAt: func(uint64) (uint64, error) { return term, nil }})
	if e != nil {
		return View{}, e
	}
	last := r.last
	last.Result.Value = bytes.Clone(last.Result.Value)
	return View{image, log, r.status(), last}, nil
}
func (r *Receiver) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.failure
	}
	r.closed = true
	r.stage = nil
	e := r.settle()
	e = errors.Join(e, r.app.Shutdown(context.Background()), r.store.Close())
	r.core.Stop()
	r.failure = errors.Join(r.failure, e)
	return r.failure
}

func parseUint(s string) (uint64, error) { return strconv.ParseUint(s, 10, 64) }
