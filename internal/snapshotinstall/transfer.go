package snapshotinstall

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/snapshot"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type HigherTerm struct{ Term uint64 }

func (e HigherTerm) Error() string {
	return fmt.Sprintf("snapshotinstall: leader must persist step-down to term %d", e.Term)
}

type Progress struct {
	NextIndex, MatchIndex, Offset uint64
	Finished, AwaitAppend         bool
}

// Transfer is a single-owner, bounded isolated leader controller, never a
// network worker or ordinary replication fallback. Its view is immutable. Every
// observation requires the owner's CURRENT term/generation. HigherTerm is an
// action for the real M05 core, not permission to alter term without persistence.
type Transfer struct {
	cfg                                config.Config
	to, generation, epoch, id          string
	term, index, boundaryTerm, counter uint64
	image                              []byte
	log                                wal.State
	chunk                              int
	progress                           Progress
	pending                            peerprotocol.Message
}

func NewTransfer(c config.Config, to, generation string, image []byte, log wal.State, next, match uint64, chunk int) (*Transfer, error) {
	s, e := storageformat.DecodeSnapshot(image)
	if e != nil {
		return nil, e
	}
	n, e := parseUint(strings.TrimPrefix(generation, "gen-"))
	if e != nil || n == 0 || generation != "gen-"+fmt.Sprint(n) {
		return nil, ErrInput
	}
	if _, ok := c.Member(to); !ok || to == c.NodeID() || s.Index == 0 || log.BaseIndex != s.Index || log.BaseTerm != s.Term || log.LastIndex() == math.MaxUint64 || next == 0 || next > s.Index || match >= next || chunk < 1 || chunk > 64*1024 {
		return nil, ErrInput
	}
	if _, e = snapshot.RecoverCommitted(context.Background(), c, image, log, 1); e != nil {
		return nil, e
	}
	epoch, e := epochID()
	if e != nil {
		return nil, e
	}
	sum := sha256.Sum256(image)
	log.Entries = append([]storageformat.LogEntry(nil), log.Entries...)
	for i := range log.Entries {
		log.Entries[i].Raw = bytes.Clone(log.Entries[i].Raw)
	}
	return &Transfer{cfg: c, to: to, generation: generation, epoch: epoch, id: hex.EncodeToString(sum[:]), term: log.HardState.Term, index: s.Index, boundaryTerm: s.Term, image: bytes.Clone(image), log: log, chunk: chunk, progress: Progress{NextIndex: next, MatchIndex: match}}, nil
}
func (t *Transfer) Progress() Progress { return t.progress }
func (t *Transfer) valid(term uint64, generation string) bool {
	return term == t.term && generation == t.generation
}
func (t *Transfer) request(kind peerprotocol.Kind, body map[string]any) (peerprotocol.Message, error) {
	t.counter++
	body["protocol_version"] = 1
	body["cluster_id"] = t.cfg.ClusterID()
	body["config_hash"] = t.cfg.Hash()
	body["from"] = t.cfg.NodeID()
	body["to"] = t.to
	body["rpc_id"] = t.epoch + "-" + fmt.Sprint(t.counter)
	body["term"] = fmt.Sprint(t.term)
	raw, _ := json.Marshal(body)
	msg, e := peerprotocol.DecodeRequest(t.cfg, kind, raw, t.to)
	if e == nil {
		t.pending = msg
	}
	return msg, e
}
func (t *Transfer) NextSnapshot(term uint64, generation string) (peerprotocol.Message, error) {
	if !t.valid(term, generation) || t.progress.Finished {
		return peerprotocol.Message{}, ErrInput
	}
	if len(t.pending.Bytes()) != 0 {
		return t.pending, nil
	}
	offset := t.progress.Offset
	if offset == uint64(len(t.image)) {
		offset = uint64(len(t.image) - min(t.chunk, len(t.image)))
	}
	end := min(int(offset)+t.chunk, len(t.image))
	return t.request(peerprotocol.Snapshot, map[string]any{"leader_id": t.cfg.NodeID(), "snapshot_id": t.id, "last_included_index": fmt.Sprint(t.index), "last_included_term": fmt.Sprint(t.boundaryTerm), "offset": fmt.Sprint(offset), "data_b64": base64.StdEncoding.EncodeToString(t.image[int(offset):end]), "done": end == len(t.image), "total_bytes": fmt.Sprint(len(t.image)), "sha256": t.id})
}
func (t *Transfer) NextAppend(term uint64, generation string) (peerprotocol.Message, error) {
	if !t.valid(term, generation) || !t.progress.Finished || len(t.pending.Bytes()) != 0 {
		return peerprotocol.Message{}, ErrInput
	}
	next := t.progress.NextIndex
	if next <= t.log.BaseIndex || next > t.log.LastIndex()+1 {
		return peerprotocol.Message{}, ErrInput
	}
	prev := next - 1
	pt, ok := termAt(t.log, prev)
	if !ok {
		return peerprotocol.Message{}, ErrInput
	}
	rows := []json.RawMessage{}
	size := 0
	for i := next; i <= t.log.LastIndex() && len(rows) < 16; i++ {
		e := t.log.Entries[i-t.log.BaseIndex-1]
		if size+len(e.Raw) > 256*1024 {
			break
		}
		rows = append(rows, e.Raw)
		size += len(e.Raw)
	}
	return t.request(peerprotocol.Append, map[string]any{"leader_id": t.cfg.NodeID(), "prev_log_index": fmt.Sprint(prev), "prev_log_term": fmt.Sprint(pt), "entries": rows, "leader_commit": fmt.Sprint(t.log.CommitIndex)})
}
func (t *Transfer) Observe(term uint64, generation string, res peerprotocol.Message) error {
	if !t.valid(term, generation) {
		return nil
	} // obsolete local epoch/generation
	if len(t.pending.Bytes()) == 0 {
		return ErrInput
	}
	checked, e := peerprotocol.DecodeResponse(t.cfg, t.pending, res.Bytes())
	if e != nil {
		return e
	}
	if checked.Envelope().Term > term {
		return HigherTerm{checked.Envelope().Term}
	}
	if checked.Envelope().Term != term {
		return nil
	}
	body, e := strictjson.Decode(checked.Bytes(), peerprotocol.MaxBody)
	if e != nil {
		return e
	}
	if t.pending.Kind() == peerprotocol.Append {
		if body["success"] != true {
			return ErrInput
		}
		match, _ := strictjson.Uint(body["match_index"])
		if match > t.log.LastIndex() {
			return ErrInput
		}
		if match > t.progress.MatchIndex {
			t.progress.MatchIndex = match
		}
		t.progress.NextIndex = t.progress.MatchIndex + 1
		t.progress.AwaitAppend = false
		t.pending = peerprotocol.Message{}
		return nil
	}
	if body["reason"] == "obsolete" {
		index, _ := strictjson.Uint(body["resume_index"])
		rt, _ := strictjson.Uint(body["resume_term"])
		lt, ok := termAt(t.log, index)
		if !ok || lt != rt || index == math.MaxUint64 {
			return ErrInput
		}
		t.progress.NextIndex = index + 1
		t.progress.Finished = true
		t.progress.AwaitAppend = true
		t.pending = peerprotocol.Message{}
		return nil
	}
	next, _ := strictjson.Uint(body["next_offset"])
	if body["accepted"] == true {
		if _, ok := body["installed_index"]; ok {
			if t.index > t.progress.MatchIndex {
				t.progress.MatchIndex = t.index
			}
			t.progress.NextIndex = t.progress.MatchIndex + 1
			t.progress.Finished = true
		}
	}
	t.progress.Offset = next
	t.pending = peerprotocol.Message{}
	return nil
}
