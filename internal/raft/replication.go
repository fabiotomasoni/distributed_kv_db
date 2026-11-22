package raft

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

var ErrNotLeader = errors.New("raft: local append requires leader")

type Progress struct {
	ID                    string
	NextIndex, MatchIndex uint64
}

// LocalAppend certifies only a locally synchronized entry, never commit/quorum.
type LocalAppend struct{ Index, Term uint64 }

// EnableReplication is a startup-only capability: batch size 1..16. Zero/default
// preserves the M05 election harness and its approved fixtures unchanged.
func (x *Core) EnableReplication(batch int) error {
	if batch < 1 || batch > 16 || x.stepped || x.state.Role != Follower || x.pending != nil || x.state.Closed || x.state.Failure != nil || x.batchSize != 0 || x.state.LastIndex == math.MaxUint64 {
		return ErrInput
	}
	for _, e := range x.entries {
		if storageformat.ValidateEntry(e.Raw) != nil {
			return ErrInput
		}
		v, _ := strictjson.Decode(e.Raw, storageformat.MaxEntry)
		if numeric(v, "index") != e.Index || numeric(v, "term") != e.Term {
			return ErrInput
		}
	}
	x.batchSize = batch
	return nil
}
func (x *Core) LogEntries() []storageformat.LogEntry {
	out := append([]storageformat.LogEntry(nil), x.entries...)
	for i := range out {
		out[i].Raw = bytes.Clone(out[i].Raw)
	}
	return out
}
func (x *Core) initProgress() {
	x.progress = map[string]Progress{}
	x.active = map[string]peerprotocol.Message{}
	for _, m := range x.cfg.Members() {
		p := Progress{ID: m.ID, NextIndex: x.state.LastIndex + 1}
		if m.ID == x.cfg.NodeID() {
			p.MatchIndex = x.state.LastIndex
		}
		x.progress[m.ID] = p
	}
}
func (x *Core) entry(index uint64) storageformat.LogEntry { return x.entries[index-x.baseIndex-1] }
func (x *Core) installSuffix(from uint64, entries []storageformat.LogEntry) {
	cut := int(from - x.baseIndex - 1)
	x.entries = append(x.entries[:cut:cut], entries...)
	x.terms = x.terms[:cut]
	for _, e := range entries {
		x.terms = append(x.terms, e.Term)
	}
	x.state.LastIndex = x.baseIndex + uint64(len(x.entries))
	x.state.LastTerm = x.baseTerm
	if len(x.terms) > 0 {
		x.state.LastTerm = x.terms[len(x.terms)-1]
	}
}
func (x *Core) saveSuffix(from uint64, entries []storageformat.LogEntry, proposal bool, after func() (Effects, error)) (Effects, error) {
	count, size := 0, 0
	for count < len(entries) && count < 16 {
		n := len(entries[count].Raw)
		if size+n > 256*1024 {
			break
		}
		size += n
		count++
	}
	if count == 0 {
		return x.fail(ErrInput)
	}
	chunk := entries[:count]
	raw := make([]json.RawMessage, count)
	for i, e := range chunk {
		raw[i] = e.Raw
	}
	var vote any
	if x.state.Vote != "" {
		vote = x.state.Vote
	}
	payload := map[string]any{"hard_state": map[string]any{"current_term": fmt.Sprint(x.state.Term), "voted_for": vote}, "replace_suffix": map[string]any{"from_index": fmt.Sprint(from), "entries": raw}}
	return x.persistBatch(payload, proposal, func() (Effects, error) {
		x.installSuffix(from, chunk)
		if count < len(entries) {
			return x.saveSuffix(from+uint64(count), entries[count:], proposal, after)
		}
		return after()
	})
}
func (x *Core) AppendLocal(now uint64, kind string, command json.RawMessage) (Effects, error) {
	if err := x.enter(now); err != nil {
		return Effects{}, err
	}
	if x.batchSize == 0 {
		return Effects{}, ErrUnsupported
	}
	if x.commitEnabled && !x.readsEnabled && kind == "read" {
		return Effects{}, ErrUnsupported
	}
	if x.state.Role != Leader {
		return Effects{}, ErrNotLeader
	}
	if x.state.LastIndex >= math.MaxUint64-1 {
		return Effects{}, ErrInput
	}
	i := x.state.LastIndex + 1
	v := map[string]any{"index": fmt.Sprint(i), "term": fmt.Sprint(x.state.Term), "kind": kind}
	if command != nil {
		c, err := strictjson.Decode(command, storageformat.MaxEntry)
		if err != nil {
			return Effects{}, ErrInput
		}
		v["command"] = c
	}
	raw, err := json.Marshal(v)
	if err != nil || storageformat.ValidateEntry(raw) != nil {
		return Effects{}, ErrInput
	}
	e := storageformat.LogEntry{Index: i, Term: x.state.Term, Raw: raw}
	return x.saveSuffix(i, []storageformat.LogEntry{e}, true, func() (Effects, error) {
		p := x.progress[x.cfg.NodeID()]
		p.MatchIndex = i
		p.NextIndex = i + 1
		x.progress[p.ID] = p
		effects, err := x.broadcast()
		effects.Local = &LocalAppend{Index: i, Term: e.Term}
		return effects, err
	})
}
func (x *Core) appendRequest(to string) (peerprotocol.Message, error) {
	p := x.progress[to]
	if p.NextIndex <= x.baseIndex || p.NextIndex > x.state.LastIndex+1 {
		return peerprotocol.Message{}, ErrUnsupported
	}
	prev := p.NextIndex - 1
	t, ok := x.term(prev)
	if !ok {
		return peerprotocol.Message{}, ErrInput
	}
	entries := []json.RawMessage{}
	size := 0
	for index := p.NextIndex; index <= x.state.LastIndex && len(entries) < x.batchSize; index++ {
		e := x.entry(index)
		if size+len(e.Raw) > 256*1024 {
			break
		}
		entries = append(entries, json.RawMessage(e.Raw))
		size += len(e.Raw)
	}
	return x.request(peerprotocol.Append, to, map[string]any{"leader_id": x.cfg.NodeID(), "prev_log_index": fmt.Sprint(prev), "prev_log_term": fmt.Sprint(t), "entries": entries, "leader_commit": fmt.Sprint(x.state.CommitIndex)})
}

// Sent registers actual dispatch, not an offer dropped by adapter backpressure.
// Drivers must call Failed for transport failures, or Response for replies.
func (x *Core) Sent(req peerprotocol.Message) error {
	if x.batchSize == 0 || req.Kind() != peerprotocol.Append {
		return nil
	}
	env := req.Envelope()
	if x.state.Role != Leader || env.Term != x.state.Term || env.From != x.cfg.NodeID() || x.active[env.To].Envelope().RPCID != "" {
		return ErrInput
	}
	if _, err := peerprotocol.DecodeRequest(x.cfg, req.Kind(), req.Bytes(), env.To); err != nil {
		return err
	}
	body, _ := strictjson.Decode(req.Bytes(), peerprotocol.MaxBody)
	i := numeric(body, "prev_log_index")
	t, exists := x.term(i)
	if !exists || t != numeric(body, "prev_log_term") || i+1 != x.progress[env.To].NextIndex {
		return ErrInput
	}
	for j, v := range body["entries"].([]any) {
		index := i + uint64(j) + 1
		if index > x.state.LastIndex {
			return ErrInput
		}
		b, _ := json.Marshal(v)
		if !bytes.Equal(b, x.entry(index).Raw) {
			return ErrInput
		}
	}
	x.active[env.To] = req
	return nil
}
func (x *Core) Failed(req peerprotocol.Message) {
	env := req.Envelope()
	if x.active[env.To].Envelope().RPCID == env.RPCID {
		delete(x.active, env.To)
	}
}
func (x *Core) replicationResponse(req, res peerprotocol.Message) (Effects, error) {
	env := res.Envelope()
	if x.state.Role != Leader || env.Term != x.state.Term || req.Envelope().Term != x.state.Term || (x.active[env.From].Envelope().RPCID != env.RPCID || !bytes.Equal(x.active[env.From].Bytes(), req.Bytes())) {
		return Effects{}, nil
	}
	delete(x.active, env.From)
	body, _ := strictjson.Decode(res.Bytes(), peerprotocol.MaxBody)
	p := x.progress[env.From]
	if body["success"] == true {
		match := numeric(body, "match_index")
		if match > x.state.LastIndex {
			return x.fail(ErrInput)
		}
		if match > p.MatchIndex {
			p.MatchIndex = match
		}
		p.NextIndex = p.MatchIndex + 1
	} else {
		next := numeric(body, "conflict_index")
		if _, has := body["conflict_term"]; has {
			term := numeric(body, "conflict_term")
			for i := x.state.LastIndex; i > x.baseIndex; i-- {
				t, _ := x.term(i)
				if t == term {
					next = i + 1
					break
				}
			}
		}
		if next >= p.NextIndex {
			next = p.NextIndex - 1
		}
		lower := p.MatchIndex + 1
		if lower < x.baseIndex+1 {
			lower = x.baseIndex + 1
		}
		if next < lower {
			next = lower
		}
		if next > x.state.LastIndex+1 {
			next = x.state.LastIndex + 1
		}
		p.NextIndex = next
	}
	x.progress[env.From] = p
	if body["success"] == true {
		return x.maybeCommit()
	}
	return Effects{}, nil
}
func (x *Core) receiveAppend(req peerprotocol.Message, body map[string]any, higher, obsolete bool) (Effects, error) {
	if x.commitEnabled && !x.readsEnabled {
		for _, v := range body["entries"].([]any) {
			if v.(map[string]any)["kind"] == "read" {
				after := func() (Effects, error) { return Effects{Rejected: ErrUnsupported}, nil }
				if higher {
					return x.persist(false, after)
				}
				return after()
			}
		}
	}
	prev, pt := numeric(body, "prev_log_index"), numeric(body, "prev_log_term")
	local, exists := x.term(prev)
	reply := func() (Effects, error) {
		end := prev + uint64(len(body["entries"].([]any)))
		index := numeric(body, "leader_commit")
		if index > end {
			index = end
		}
		return x.persistCommit(index, func() (Effects, error) {
			return x.respond(req, map[string]any{"success": true, "match_index": fmt.Sprint(end)})
		})
	}
	if obsolete || !exists || local != pt {
		conflict := x.state.LastIndex + 1
		hint := map[string]any{"success": false}
		if !obsolete && prev < x.baseIndex {
			conflict = x.baseIndex + 1
		} else if !obsolete && exists && local != pt {
			conflict = prev
			for conflict > x.baseIndex {
				t, _ := x.term(conflict - 1)
				if t != local {
					break
				}
				conflict--
			}
			if conflict == 0 {
				conflict = 1
			}
			if local > 0 {
				hint["conflict_term"] = fmt.Sprint(local)
			}
		}
		hint["conflict_index"] = fmt.Sprint(conflict)
		after := func() (Effects, error) { return x.respond(req, hint) }
		if higher {
			return x.persist(false, after)
		}
		return after()
	}
	var suffix []storageformat.LogEntry
	var from uint64
	for j, v := range body["entries"].([]any) {
		index := prev + uint64(j) + 1
		raw, _ := json.Marshal(v)
		term := numeric(v.(map[string]any), "term")
		old, exists := x.term(index)
		if from == 0 && exists && old == term {
			if !bytes.Equal(x.entry(index).Raw, raw) {
				return x.fail(errors.New("same index/term with different entry bytes"))
			}
			continue
		}
		if from == 0 {
			if index <= x.state.CommitIndex {
				return x.fail(errors.New("conflict in known committed prefix"))
			}
			from = index
		}
		suffix = append(suffix, storageformat.LogEntry{Index: index, Term: term, Raw: raw})
	}
	if from != 0 {
		return x.saveSuffix(from, suffix, false, reply)
	}
	if higher {
		return x.persist(false, reply)
	}
	return reply()
}
