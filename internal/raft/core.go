// Package raft implements only M05 elections. Core is a deterministic single-
// owner event machine: no sockets, filesystem calls, real clock or goroutines.
package raft

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

var (
	ErrBusy        = errors.New("raft: persistence completion required")
	ErrClosed      = errors.New("raft: closed")
	ErrFatal       = errors.New("raft: fatal, restart required")
	ErrInput       = errors.New("raft: invalid event, recovered view or clock")
	ErrUnsupported = errors.New("raft: outside M05 election scope")
	ErrFull        = errors.New("raft: peer event capacity exhausted")
)

type Role string

const (
	Follower  Role = "follower"
	Candidate Role = "candidate"
	Leader    Role = "leader"
)

// Draw returns an inclusive uniform election timeout in milliseconds. Production
// uses crypto/rand; tests inject deterministic draws. It must not block/panic.
type Draw func(min, max uint64) (uint64, error)
type Status struct {
	Role                                Role
	Term                                uint64
	Vote, Leader                        string
	LastIndex, LastTerm, CommitIndex    uint64
	ElectionDeadline, HeartbeatDeadline uint64
	Pending, Closed                     bool
	Failure                             error
}
type Persistence struct {
	ID    uint64
	Bytes []byte
}
type Effects struct {
	Persist  *Persistence
	Send     []peerprotocol.Message
	Reply    *peerprotocol.Message
	Rejected error
}
type pending struct {
	id    uint64
	after func() (Effects, error)
}
type Core struct {
	cfg                    config.Config
	state                  Status
	baseIndex, baseTerm    uint64
	terms                  []uint64
	generation, epoch      string
	sequence, counter, now uint64
	draw                   Draw
	votes                  map[string]bool
	pending                *pending
}

// New recovers log metadata, never application state. The supplied WAL view must
// come from validated recovery. Core still checks its structural bounds/copies.
func New(c config.Config, s wal.State, generation, epoch string, draw Draw, now uint64) (*Core, error) {
	if !c.Valid() || draw == nil || !strictjson.Hex(epoch, 32) || !strings.HasPrefix(generation, "gen-") {
		return nil, ErrInput
	}
	n, err := strconv.ParseUint(strings.TrimPrefix(generation, "gen-"), 10, 64)
	if err != nil || n == 0 || generation != "gen-"+strconv.FormatUint(n, 10) {
		return nil, ErrInput
	}
	if s.Sequence == 0 || s.BaseIndex > math.MaxUint64-uint64(len(s.Entries)) || (s.BaseIndex == 0) != (s.BaseTerm == 0) || s.HardState.Term < s.BaseTerm || s.CommitIndex < s.BaseIndex || s.CommitIndex > s.LastIndex() {
		return nil, ErrInput
	}
	if s.HardState.Vote != "" {
		if _, ok := c.Member(s.HardState.Vote); !ok || s.HardState.Term == 0 {
			return nil, ErrInput
		}
	}
	x := &Core{cfg: c, generation: generation, epoch: epoch, draw: draw, sequence: s.Sequence, baseIndex: s.BaseIndex, baseTerm: s.BaseTerm, now: now}
	x.state = Status{Role: Follower, Term: s.HardState.Term, Vote: s.HardState.Vote, LastIndex: s.LastIndex(), LastTerm: s.BaseTerm, CommitIndex: s.CommitIndex}
	for j, e := range s.Entries {
		if e.Index != s.BaseIndex+uint64(j)+1 || e.Term == 0 || e.Term < x.state.LastTerm || e.Term > s.HardState.Term {
			return nil, ErrInput
		}
		x.terms = append(x.terms, e.Term)
		x.state.LastTerm = e.Term
	}
	if err = x.reset(); err != nil {
		return nil, err
	}
	return x, nil
}
func (x *Core) Status() Status { return x.state }
func (x *Core) fail(err error) (Effects, error) {
	x.state.Failure = errors.Join(ErrFatal, err)
	x.state.Role = Follower
	x.state.Leader = ""
	x.state.Pending = false
	x.pending = nil
	return Effects{}, x.state.Failure
}
func (x *Core) enter(now uint64) error {
	if x.state.Closed {
		return ErrClosed
	}
	if x.state.Failure != nil {
		return x.state.Failure
	}
	if x.pending != nil {
		return ErrBusy
	}
	if now < x.now {
		return ErrInput
	}
	x.now = now
	return nil
}
func (x *Core) reset() error {
	_, min, max := x.cfg.Timing()
	delay, err := x.draw(min, max)
	if err != nil || delay < min || delay > max || delay > math.MaxUint64-x.now {
		return ErrInput
	}
	x.state.ElectionDeadline = x.now + delay
	return nil
}
func (x *Core) nextRound() error {
	h, _, _ := x.cfg.Timing()
	if h > math.MaxUint64-x.now {
		return ErrInput
	}
	x.state.HeartbeatDeadline = x.now + h
	return nil
}
func (x *Core) persist(noop bool, after func() (Effects, error)) (Effects, error) {
	if x.sequence == math.MaxUint64 {
		return x.fail(ErrInput)
	}
	var vote any
	if x.state.Vote != "" {
		vote = x.state.Vote
	}
	payload := map[string]any{"hard_state": map[string]any{"current_term": fmt.Sprint(x.state.Term), "voted_for": vote}}
	if noop {
		if x.state.LastIndex == math.MaxUint64 {
			return x.fail(ErrInput)
		}
		i := x.state.LastIndex + 1
		payload["replace_suffix"] = map[string]any{"from_index": fmt.Sprint(i), "entries": []any{map[string]any{"index": fmt.Sprint(i), "term": fmt.Sprint(x.state.Term), "kind": "noop"}}}
	}
	raw, _ := json.Marshal(payload)
	if _, err := storageformat.DecodeTransaction(raw); err != nil {
		return x.fail(err)
	}
	id := x.sequence + 1
	x.pending = &pending{id, after}
	x.state.Pending = true
	return Effects{Persist: &Persistence{id, raw}}, nil
}

// Durable correlates the exact generation/physical sequence, never an entry
// index. No dependent reply, self vote or leadership is emitted before this.
func (x *Core) Durable(id uint64, token wal.Token, err error) (Effects, error) {
	if x.state.Closed {
		return Effects{}, ErrClosed
	}
	if x.pending == nil {
		return Effects{}, ErrInput
	}
	if err != nil {
		return x.fail(err)
	}
	if id != x.pending.id || token.Generation != x.generation || token.Sequence != id {
		return x.fail(ErrInput)
	}
	p := x.pending
	x.pending = nil
	x.state.Pending = false
	x.sequence = id
	return p.after()
}
func (x *Core) Stop() {
	x.state.Closed = true
	x.state.Role = Follower
	x.state.Leader = ""
	x.state.Pending = false
	x.pending = nil
}

func (x *Core) request(kind peerprotocol.Kind, to string, fields map[string]any) (peerprotocol.Message, error) {
	if x.counter == math.MaxUint64 {
		return peerprotocol.Message{}, ErrInput
	}
	x.counter++
	fields["protocol_version"] = 1
	fields["cluster_id"] = x.cfg.ClusterID()
	fields["config_hash"] = x.cfg.Hash()
	fields["from"] = x.cfg.NodeID()
	fields["to"] = to
	fields["rpc_id"] = fmt.Sprintf("%s-%s-%d", x.cfg.NodeID(), x.epoch, x.counter)
	fields["term"] = fmt.Sprint(x.state.Term)
	raw, _ := json.Marshal(fields)
	return peerprotocol.DecodeRequest(x.cfg, kind, raw, to)
}
func (x *Core) broadcast() (Effects, error) {
	var e Effects
	for _, m := range x.cfg.Members() {
		if m.ID == x.cfg.NodeID() {
			continue
		}
		kind := peerprotocol.Vote
		fields := map[string]any{"candidate_id": x.cfg.NodeID(), "last_log_index": fmt.Sprint(x.state.LastIndex), "last_log_term": fmt.Sprint(x.state.LastTerm)}
		if x.state.Role == Leader {
			kind = peerprotocol.Append
			fields = map[string]any{"leader_id": x.cfg.NodeID(), "prev_log_index": fmt.Sprint(x.state.LastIndex), "prev_log_term": fmt.Sprint(x.state.LastTerm), "entries": []any{}, "leader_commit": fmt.Sprint(x.state.CommitIndex)}
		}
		msg, err := x.request(kind, m.ID, fields)
		if err != nil {
			return x.fail(err)
		}
		e.Send = append(e.Send, msg)
	}
	if err := x.nextRound(); err != nil {
		return x.fail(err)
	}
	return e, nil
}
func (x *Core) Tick(now uint64) (Effects, error) {
	if err := x.enter(now); err != nil {
		return Effects{}, err
	}
	if x.state.Role == Leader {
		if now >= x.state.HeartbeatDeadline {
			return x.broadcast()
		}
		return Effects{}, nil
	}
	if now >= x.state.ElectionDeadline {
		if x.state.Term == math.MaxUint64 {
			return x.fail(ErrInput)
		}
		x.state.Term++
		x.state.Vote = x.cfg.NodeID()
		x.state.Role = Candidate
		x.state.Leader = ""
		x.votes = map[string]bool{}
		if err := x.reset(); err != nil {
			return x.fail(err)
		}
		return x.persist(false, func() (Effects, error) { x.votes[x.cfg.NodeID()] = true; return x.broadcast() })
	}
	if x.state.Role == Candidate && now >= x.state.HeartbeatDeadline {
		return x.broadcast()
	}
	return Effects{}, nil
}
func (x *Core) respond(req peerprotocol.Message, fields map[string]any) (Effects, error) {
	env := req.Envelope()
	fields["protocol_version"] = 1
	fields["cluster_id"] = x.cfg.ClusterID()
	fields["config_hash"] = x.cfg.Hash()
	fields["from"] = x.cfg.NodeID()
	fields["to"] = env.From
	fields["rpc_id"] = env.RPCID
	fields["term"] = fmt.Sprint(x.state.Term)
	raw, _ := json.Marshal(fields)
	r, err := peerprotocol.DecodeResponse(x.cfg, req, raw)
	if err != nil {
		return x.fail(err)
	}
	return Effects{Reply: &r}, nil
}
func numeric(m map[string]any, key string) uint64 { n, _ := strictjson.Uint(m[key]); return n }
func (x *Core) Request(now uint64, req peerprotocol.Message) (Effects, error) {
	if err := x.enter(now); err != nil {
		return Effects{}, err
	}
	checked, err := peerprotocol.DecodeRequest(x.cfg, req.Kind(), req.Bytes(), x.cfg.NodeID())
	if err != nil {
		return Effects{}, err
	}
	req = checked
	env := req.Envelope()
	body, _ := strictjson.Decode(req.Bytes(), peerprotocol.MaxBody)
	higher := env.Term > x.state.Term
	obsolete := env.Term < x.state.Term
	if higher {
		x.state.Term = env.Term
		x.state.Vote = ""
		x.state.Role = Follower
		x.state.Leader = ""
		x.votes = nil
	}
	var after func() (Effects, error)
	switch req.Kind() {
	case peerprotocol.Vote:
		lt, li := numeric(body, "last_log_term"), numeric(body, "last_log_index")
		fresh := lt > x.state.LastTerm || lt == x.state.LastTerm && li >= x.state.LastIndex
		grant := !obsolete && fresh && (x.state.Vote == "" || x.state.Vote == env.From)
		changed := grant && x.state.Vote != env.From
		if grant {
			x.state.Vote = env.From
			if err = x.reset(); err != nil {
				return x.fail(err)
			}
		}
		after = func() (Effects, error) { return x.respond(req, map[string]any{"vote_granted": grant}) }
		if higher || changed {
			return x.persist(false, after)
		}
		return after()
	case peerprotocol.Append, peerprotocol.Snapshot:
		if !obsolete {
			if x.state.Leader != "" && x.state.Leader != env.From {
				return x.fail(errors.New("two leaders in the same term"))
			}
			x.state.Role = Follower
			x.state.Leader = env.From
			x.votes = nil
			if err = x.reset(); err != nil {
				return x.fail(err)
			}
		}
		after = func() (Effects, error) {
			if req.Kind() == peerprotocol.Snapshot || len(body["entries"].([]any)) != 0 {
				return Effects{Rejected: ErrUnsupported}, nil
			}
			i, t := numeric(body, "prev_log_index"), numeric(body, "prev_log_term")
			local, exists := x.term(i)
			if !obsolete && exists && local == t {
				return x.respond(req, map[string]any{"success": true, "match_index": fmt.Sprint(i)})
			}
			conflict := x.state.LastIndex
			if conflict < math.MaxUint64 {
				conflict++
			}
			if conflict == 0 {
				conflict = 1
			}
			return x.respond(req, map[string]any{"success": false, "conflict_index": fmt.Sprint(conflict)})
		}
		if higher {
			return x.persist(false, after)
		}
		return after()
	}
	return Effects{}, ErrUnsupported
}
func (x *Core) term(index uint64) (uint64, bool) {
	if index == x.baseIndex {
		return x.baseTerm, true
	}
	if index > x.baseIndex && index-x.baseIndex <= uint64(len(x.terms)) {
		return x.terms[index-x.baseIndex-1], true
	}
	return 0, false
}
func (x *Core) Response(now uint64, req, res peerprotocol.Message) (Effects, error) {
	if err := x.enter(now); err != nil {
		return Effects{}, err
	}
	if req.Envelope().From != x.cfg.NodeID() {
		return Effects{}, ErrInput
	}
	if _, err := peerprotocol.DecodeRequest(x.cfg, req.Kind(), req.Bytes(), req.Envelope().To); err != nil {
		return Effects{}, err
	}
	checked, err := peerprotocol.DecodeResponse(x.cfg, req, res.Bytes())
	if err != nil {
		return Effects{}, err
	}
	env := checked.Envelope()
	if env.Term > x.state.Term {
		x.state.Term = env.Term
		x.state.Vote = ""
		x.state.Role = Follower
		x.state.Leader = ""
		x.votes = nil
		return x.persist(false, func() (Effects, error) { return Effects{}, nil })
	}
	if x.state.Role != Candidate || req.Kind() != peerprotocol.Vote || req.Envelope().Term != x.state.Term || env.Term != x.state.Term {
		return Effects{}, nil
	}
	body, _ := strictjson.Decode(checked.Bytes(), peerprotocol.MaxBody)
	if body["vote_granted"] != true {
		return Effects{}, nil
	}
	x.votes[env.From] = true
	if len(x.votes) < 2 {
		return Effects{}, nil
	}
	// Only a local new-term noop is added. No replication, matchIndex, commit or
	// applier path is present in M05. Leadership is published after its Sync.
	return x.persist(true, func() (Effects, error) {
		x.terms = append(x.terms, x.state.Term)
		x.state.LastIndex++
		x.state.LastTerm = x.state.Term
		x.state.Role = Leader
		x.state.Leader = x.cfg.NodeID()
		return x.broadcast()
	})
}
