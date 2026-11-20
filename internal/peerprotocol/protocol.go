// Package peerprotocol validates v1 RPC envelopes/bodies without consensus, I/O
// or state mutation. Message bytes are canonical owned copies of validated JSON.
package peerprotocol

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

const MaxBody = 384 * 1024

type Kind string

const (
	Vote     Kind = "request-vote"
	Append   Kind = "append-entries"
	Snapshot Kind = "install-snapshot"
)

func (k Kind) Path() string { return "/raft/v1/" + string(k) }

var ErrMessage = errors.New("peerprotocol: invalid RPC schema")
var ErrIdentity = errors.New("peerprotocol: incompatible envelope or correlation")

type Envelope struct {
	ClusterID, ConfigHash, From, To, RPCID string
	Term                                   uint64
}
type Message struct {
	kind     Kind
	envelope Envelope
	raw      []byte
	request  bool
}

func (m Message) Kind() Kind         { return m.kind }
func (m Message) Envelope() Envelope { return m.envelope }
func (m Message) Bytes() []byte      { return bytes.Clone(m.raw) }

func envelope(o map[string]any, c config.Config, to string) (Envelope, error) {
	bad := func() (Envelope, error) { return Envelope{}, ErrIdentity }
	v, err := strictjson.Integer(o["protocol_version"])
	if !c.Valid() || err != nil || v != 1 || o["cluster_id"] != c.ClusterID() || o["config_hash"] != c.Hash() {
		return bad()
	}
	from, ok := o["from"].(string)
	if !ok {
		return bad()
	}
	dest, ok := o["to"].(string)
	if !ok || dest != to || from == dest {
		return bad()
	}
	if _, ok = c.Member(from); !ok {
		return bad()
	}
	if _, ok = c.Member(dest); !ok {
		return bad()
	}
	id, ok := o["rpc_id"].(string)
	if !ok || len(id) == 0 || len(id) > 128 {
		return bad()
	}
	for _, b := range []byte(id) {
		if !(b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.ContainsRune("-_.:", rune(b))) {
			return bad()
		}
	}
	term, err := strictjson.Uint(o["term"])
	if err != nil {
		return bad()
	}
	return Envelope{c.ClusterID(), c.Hash(), from, dest, id, term}, nil
}

const common = "protocol_version cluster_id config_hash from to rpc_id term"

func number(o map[string]any, k string) (uint64, bool) {
	n, err := strictjson.Uint(o[k])
	return n, err == nil
}
func flag(o map[string]any, k string) (bool, bool) { b, ok := o[k].(bool); return b, ok }
func pair(o map[string]any, i, t string) bool {
	a, ok := number(o, i)
	b, ok2 := number(o, t)
	return ok && ok2 && (a == 0) == (b == 0)
}

func DecodeRequest(c config.Config, kind Kind, raw []byte, recipient string) (Message, error) {
	o, err := strictjson.Decode(raw, MaxBody)
	if err != nil {
		return Message{}, ErrMessage
	}
	env, err := envelope(o, c, recipient)
	if err != nil {
		return Message{}, err
	}
	required := ""
	switch kind {
	case Vote:
		required = "candidate_id last_log_index last_log_term"
	case Append:
		required = "leader_id prev_log_index prev_log_term entries leader_commit"
	case Snapshot:
		required = "leader_id snapshot_id last_included_index last_included_term offset data_b64 done total_bytes sha256"
	default:
		return Message{}, ErrMessage
	}
	if _, err = strictjson.Fields(o, common+" "+required, ""); err != nil {
		return Message{}, ErrMessage
	}
	valid := false
	switch kind {
	case Vote:
		valid = o["candidate_id"] == env.From && pair(o, "last_log_index", "last_log_term")
		if term, ok := number(o, "last_log_term"); !ok || term > env.Term {
			valid = false
		}
	case Append:
		prev, ok := number(o, "prev_log_index")
		pt, ok2 := number(o, "prev_log_term")
		_, commitOK := number(o, "leader_commit")
		entries, arrayOK := o["entries"].([]any)
		valid = o["leader_id"] == env.From && ok && ok2 && (prev == 0) == (pt == 0) && pt <= env.Term && commitOK && arrayOK && len(entries) <= 16
		size := 0
		lastTerm := pt
		for j, e := range entries {
			if !valid {
				break
			}
			b, _ := json.Marshal(e)
			if storageformat.ValidateEntry(b) != nil {
				valid = false
				break
			}
			entry := e.(map[string]any)
			i, _ := number(entry, "index")
			t, _ := number(entry, "term")
			if uint64(j)+1 > math.MaxUint64-prev || i != prev+uint64(j)+1 || t < lastTerm || t > env.Term {
				valid = false
				break
			}
			lastTerm = t
			if command, exists := entry["command"]; exists {
				b, _ = json.Marshal(command)
				size += len(b)
			}
			if size > 256*1024 {
				valid = false
			}
		}
	case Snapshot:
		i, ok := number(o, "last_included_index")
		t, ok2 := number(o, "last_included_term")
		offset, ok3 := number(o, "offset")
		total, ok4 := number(o, "total_bytes")
		done, ok5 := flag(o, "done")
		data, derr := strictjson.Base64(o["data_b64"], 64*1024)
		valid = o["leader_id"] == env.From && ok && ok2 && i > 0 && t > 0 && t <= env.Term && ok3 && ok4 && total >= 44 && total <= 8*1024*1024 && offset <= total && ok5 && derr == nil && len(data) > 0 && uint64(len(data)) <= total-offset && strictjson.Hex(o["sha256"], 64) && o["snapshot_id"] == o["sha256"]
		if valid {
			end := offset + uint64(len(data))
			valid = (done && end == total) || (!done && end < total)
		}
	}
	if !valid {
		return Message{}, ErrMessage
	}
	b, _ := json.Marshal(o)
	return Message{kind: kind, envelope: env, raw: b, request: true}, nil
}

// DecodeResponse validates identity and correlation, not the safety of a vote,
// replication/installation decision. That authority belongs to the future core.
func DecodeResponse(c config.Config, request Message, raw []byte) (Message, error) {
	if !request.request || len(request.raw) == 0 {
		return Message{}, ErrMessage
	}
	o, err := strictjson.Decode(raw, MaxBody)
	if err != nil {
		return Message{}, ErrMessage
	}
	req := request.envelope
	env, err := envelope(o, c, req.From)
	if err != nil {
		return Message{}, err
	}
	if env.From != req.To || env.RPCID != req.RPCID {
		return Message{}, ErrIdentity
	}
	var input map[string]any
	if err = json.Unmarshal(request.raw, &input); err != nil {
		return Message{}, ErrMessage
	}
	required, optional := "", ""
	valid := false
	switch request.kind {
	case Vote:
		required = "vote_granted"
		_, valid = flag(o, "vote_granted")
	case Append:
		success, ok := flag(o, "success")
		valid = ok
		required = "success"
		if success {
			required += " match_index"
			match, mok := number(o, "match_index")
			prev, _ := number(input, "prev_log_index")
			entries := input["entries"].([]any)
			valid = valid && mok && match == prev+uint64(len(entries))
		} else {
			required += " conflict_index"
			optional = "conflict_term"
			i, iok := number(o, "conflict_index")
			valid = valid && iok && i > 0
			if _, has := o["conflict_term"]; has {
				t, tok := number(o, "conflict_term")
				valid = valid && tok && t > 0
			}
		}
	case Snapshot:
		accepted, aok := flag(o, "accepted")
		next, nok := number(o, "next_offset")
		total, _ := number(input, "total_bytes")
		valid = aok && nok && next <= total
		required = "accepted next_offset"
		if reason, has := o["reason"]; has {
			required += " reason resume_index resume_term"
			i, iok := number(o, "resume_index")
			t, tok := number(o, "resume_term")
			s, _ := number(input, "last_included_index")
			valid = valid && !accepted && reason == "obsolete" && next == 0 && iok && tok && i >= s && t > 0
		} else {
			optional = "installed_index"
			if _, has := o["installed_index"]; has {
				i, iok := number(o, "installed_index")
				s, _ := number(input, "last_included_index")
				done, _ := input["done"].(bool)
				valid = valid && accepted && iok && i == s && done && next == total
			}
		}
	default:
		return Message{}, ErrMessage
	}
	if _, err = strictjson.Fields(o, common+" "+required, optional); err != nil || !valid {
		return Message{}, ErrMessage
	}
	b, _ := json.Marshal(o)
	return Message{kind: request.kind, envelope: env, raw: b}, nil
}
