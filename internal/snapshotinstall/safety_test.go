package snapshotinstall

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/snapshot"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func chunk(t *testing.T, image []byte, offset, n int, from string, term uint64, alter func(map[string]any)) peerprotocol.Message {
	t.Helper()
	sum := sha256.Sum256(image)
	end := offset + n
	b := map[string]any{"leader_id": from, "snapshot_id": fmt.Sprintf("%x", sum), "last_included_index": "4", "last_included_term": "2", "offset": fmt.Sprint(offset), "data_b64": base64.StdEncoding.EncodeToString(image[offset:end]), "done": end == len(image), "total_bytes": fmt.Sprint(len(image)), "sha256": fmt.Sprintf("%x", sum)}
	if alter != nil {
		alter(b)
	}
	return request(t, peerprotocol.Snapshot, from, term, b)
}
func response(t *testing.T, m peerprotocol.Message) map[string]any {
	t.Helper()
	b, e := strictjson.Decode(m.Bytes(), peerprotocol.MaxBody)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func current(t *testing.T, root string) []byte {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(root, "CURRENT"))
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func TestChunkOrderingDuplicatesConflictsAndSingleTransferBound(t *testing.T) {
	image, _ := source(t)
	for _, mode := range []string{"duplicate", "gap", "overlap", "conflict", "metadata", "other-transfer", "retry-range"} {
		t.Run(mode, func(t *testing.T) {
			root := seed(t, nil, 0)
			r := receiver(t, root)
			old := current(t, root)
			first := chunk(t, image, 0, 64, "n1", 5, nil)
			res, e := r.Handle(deadline(t), 1, first)
			if e != nil {
				t.Fatal(e)
			}
			if response(t, res)["accepted"] != true || r.Status().BufferedBytes != 64 || r.store.State().HardState.Term != 5 || r.store.State().HardState.Vote != "" {
				t.Fatal(r.Status())
			}
			req := first
			wantAccepted := false
			wantBuffer := 64
			switch mode {
			case "duplicate":
				wantAccepted = true
			case "gap":
				req = chunk(t, image, 128, 64, "n1", 5, nil)
			case "overlap":
				req = chunk(t, image, 32, 64, "n1", 5, nil)
				wantBuffer = 0
			case "conflict":
				req = chunk(t, image, 0, 64, "n1", 5, func(b map[string]any) {
					bad := bytes.Clone(image[:64])
					bad[20] ^= 1
					b["data_b64"] = base64.StdEncoding.EncodeToString(bad)
				})
				wantBuffer = 0
			case "metadata":
				req = chunk(t, image, 0, 64, "n1", 5, func(b map[string]any) { b["total_bytes"] = fmt.Sprint(len(image) + 1) })
				wantBuffer = 0
			case "other-transfer":
				req = chunk(t, image, 0, 64, "n1", 5, func(b map[string]any) {
					b["snapshot_id"] = strings.Repeat("f", 64)
					b["sha256"] = strings.Repeat("f", 64)
				})
			case "retry-range":
				req = chunk(t, image, 16, 16, "n1", 5, nil)
				wantAccepted = true
			}
			res, e = r.Handle(deadline(t), 2, req)
			if e != nil {
				t.Fatal(e)
			}
			if (response(t, res)["accepted"] == true) != wantAccepted || r.Status().BufferedBytes != wantBuffer {
				t.Fatal(mode, response(t, res), r.Status())
			}
			if !bytes.Equal(old, current(t, root)) {
				t.Fatal("partial transfer published")
			}
		})
	}
}
func TestReceiveExpiryShutdownAndRestartRequireOffsetZero(t *testing.T) {
	root := seed(t, history()[:2], 2)
	r := receiver(t, root)
	image, _ := source(t)
	if _, e := r.Handle(deadline(t), 1, chunk(t, image, 0, 64, "n1", 5, nil)); e != nil {
		t.Fatal(e)
	}
	if e := r.Expire(5001); e != nil || r.Status().BufferedBytes != 0 {
		t.Fatal(e)
	}
	res, e := r.Handle(deadline(t), 5002, chunk(t, image, 64, 64, "n1", 5, nil))
	if e != nil || response(t, res)["next_offset"] != "0" {
		t.Fatal(e)
	}
	if _, e = r.Handle(deadline(t), 5003, chunk(t, image, 0, 64, "n1", 5, nil)); e != nil {
		t.Fatal(e)
	}
	if e = r.Close(); e != nil || r.Status().BufferedBytes != 0 {
		t.Fatal(e)
	}
	if _, e = r.Handle(deadline(t), 5004, chunk(t, image, 64, 64, "n1", 5, nil)); e != ErrClosed {
		t.Fatal(e)
	}
	r = receiver(t, root)
	res, e = r.Handle(deadline(t), 1, chunk(t, image, 64, 64, "n1", 5, nil))
	if e != nil || response(t, res)["accepted"] != false || response(t, res)["next_offset"] != "0" {
		t.Fatal(e)
	}
	if r.store.State().HardState.Term != 5 {
		t.Fatal("term lost during partial-receive restart")
	}
	if e = r.Expire(0); e == nil {
		t.Fatal("clock regressed")
	}
}
func TestReceivedImageValidationNeverChangesCurrent(t *testing.T) {
	image, _ := source(t)
	for _, mode := range []string{"digest", "corrupt", "version", "profile", "boundary"} {
		t.Run(mode, func(t *testing.T) {
			root := seed(t, history()[:2], 2)
			r := receiver(t, root)
			old := current(t, root)
			raw := bytes.Clone(image)
			if mode == "corrupt" {
				raw[len(raw)-1] ^= 1
			}
			if mode == "version" {
				raw[3] = '2'
			}
			if mode == "profile" {
				s, e := storageformat.DecodeSnapshot(raw)
				if e != nil {
					t.Fatal(e)
				}
				s.ConfigHash = strings.Repeat("f", 64)
				raw = imageBytes(t, s)
			}
			req := chunk(t, raw, 0, len(raw), "n1", 5, func(b map[string]any) {
				if mode == "digest" {
					b["snapshot_id"] = strings.Repeat("f", 64)
					b["sha256"] = strings.Repeat("f", 64)
				}
				if mode == "boundary" {
					b["last_included_index"] = "5"
				}
			})
			res, e := r.Handle(deadline(t), 1, req)
			if e != nil || response(t, res)["accepted"] != false {
				t.Fatal(mode, e)
			}
			if r.Status().BufferedBytes != 0 || !bytes.Equal(old, current(t, root)) || r.Status().Raft.Term != 5 {
				t.Fatal("invalid image published or unpersisted term")
			}
		})
	}
}
func TestNewElectionInvalidatesOldTransferAndPreservesLocalVote(t *testing.T) {
	r := receiver(t, seed(t, history()[:2], 2))
	image, log := source(t)
	tr, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 64)
	if e != nil {
		t.Fatal(e)
	}
	req, e := tr.NextSnapshot(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	res, e := r.Handle(deadline(t), 1, req)
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Observe(5, "gen-9", res); e != nil {
		t.Fatal(e)
	}
	vote := request(t, peerprotocol.Vote, "n3", 6, map[string]any{"candidate_id": "n3", "last_log_index": "12", "last_log_term": "6"})
	res, e = r.Handle(deadline(t), 2, vote)
	if e != nil || response(t, res)["vote_granted"] != true {
		t.Fatal(e)
	}
	if r.Status().BufferedBytes != 0 || r.store.State().HardState.Vote != "n3" {
		t.Fatal(r.Status())
	}
	old, e := tr.NextSnapshot(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	res, e = r.Handle(deadline(t), 3, old)
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Observe(5, "gen-9", res); e == nil {
		t.Fatal("higher term response ignored")
	} else if higher, ok := e.(HigherTerm); !ok || higher.Term != 6 {
		t.Fatal(e)
	}
	if tr.Progress().MatchIndex != 0 {
		t.Fatal("old leader advanced match")
	}
	res, e = r.Handle(deadline(t), 4, chunk(t, image, 0, len(image), "n3", 6, nil))
	if e != nil || response(t, res)["installed_index"] != "4" {
		t.Fatal(e)
	}
	v, e := r.View(deadline(t))
	if e != nil || v.Log.HardState.Term != 6 || v.Log.HardState.Vote != "n3" {
		t.Fatal(v.Log.HardState, e)
	}
	// A delayed old-term chunk cannot change the newly installed generation.
	gen := v.Status.Generation
	res, e = r.Handle(deadline(t), 5, old)
	if e != nil || response(t, res)["accepted"] != false || r.Status().Generation != gen {
		t.Fatal(e)
	}
}
func TestSameTermConflictingLeaderFailsClosed(t *testing.T) {
	r := receiver(t, seed(t, nil, 0))
	image, _ := source(t)
	if _, e := r.Handle(deadline(t), 1, chunk(t, image, 0, 64, "n1", 5, nil)); e != nil {
		t.Fatal(e)
	}
	if _, e := r.Handle(deadline(t), 2, chunk(t, image, 0, 64, "n3", 5, nil)); e == nil || !r.Status().Failed {
		t.Fatal("two leaders accepted")
	}
}
func TestTransferIgnoresOldLocalGenerationAndRejectsForeignCorrelation(t *testing.T) {
	r := receiver(t, seed(t, history()[:2], 2))
	image, log := source(t)
	tr, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 64)
	if e != nil {
		t.Fatal(e)
	}
	req, e := tr.NextSnapshot(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	res, e := r.Handle(deadline(t), 1, req)
	if e != nil {
		t.Fatal(e)
	}
	before := tr.Progress()
	if e = tr.Observe(5, "gen-10", res); e != nil || tr.Progress() != before {
		t.Fatal(e)
	}
	if _, e = tr.NextSnapshot(6, "gen-9"); e == nil {
		t.Fatal("obsolete local term reused")
	}
	other, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 64)
	if e != nil {
		t.Fatal(e)
	}
	q, e := other.NextSnapshot(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	foreign, e := r.Handle(deadline(t), 2, q)
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Observe(5, "gen-9", foreign); e == nil || tr.Progress() != before {
		t.Fatal("foreign RPC changed progress")
	}
	if e = tr.Observe(5, "gen-9", res); e != nil || tr.Progress().Offset != 64 {
		t.Fatal(e)
	}
}
func TestRequestVoteAndAppendBoundaryAfterSnapshotRestart(t *testing.T) {
	root := seed(t, history()[:2], 2)
	r := receiver(t, root)
	image, _ := source(t)
	if _, e := r.Handle(deadline(t), 1, chunk(t, image, 0, len(image), "n1", 5, nil)); e != nil {
		t.Fatal(e)
	}
	for i, tc := range []struct {
		index, term uint64
		grant       bool
	}{{3, 2, false}, {4, 1, false}, {4, 2, true}} {
		// Fresh term each time avoids conflating log freshness with an earlier vote.
		req := request(t, peerprotocol.Vote, "n3", uint64(6+i), map[string]any{"candidate_id": "n3", "last_log_index": fmt.Sprint(tc.index), "last_log_term": fmt.Sprint(tc.term)})
		res, e := r.Handle(deadline(t), uint64(2+i), req)
		if e != nil || response(t, res)["vote_granted"] != tc.grant {
			t.Fatal(tc, e)
		}
	}
	if e := r.Close(); e != nil {
		t.Fatal(e)
	}
	r = receiver(t, root)
	if r.Status().Raft.LastIndex != 4 || r.Status().Raft.LastTerm != 2 || r.Status().Raft.Term != 8 || r.store.State().HardState.Vote != "n3" {
		t.Fatal(r.Status())
	}
	for i, tc := range []struct {
		prev, term uint64
		ok         bool
	}{{3, 2, false}, {4, 1, false}, {5, 2, false}, {4, 2, true}} {
		req := appendMessage(t, tc.prev, tc.term, 4, nil, 9)
		res, e := r.Handle(deadline(t), uint64(1+i), req)
		if e != nil || response(t, res)["success"] != tc.ok {
			t.Fatal(tc, e)
		}
	}
}

func TestInstallDiscardsOnlyDivergentUncommittedSuffix(t *testing.T) {
	local := append([]storageformat.LogEntry(nil), history()[:2]...)
	for i := uint64(3); i <= 6; i++ {
		local = append(local, entry(i, 3, "put", "f", i-2, "divergent", nil))
	}
	root := seed(t, local, 2)
	r := receiver(t, root)
	image, _ := source(t)
	res, e := r.Handle(deadline(t), 1, chunk(t, image, 0, len(image), "n1", 5, nil))
	if e != nil || response(t, res)["installed_index"] != "4" {
		t.Fatal(e)
	}
	v, e := r.View(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if v.Log.LastIndex() != 4 || v.Log.CommitIndex != 4 || len(v.Log.Entries) != 0 {
		t.Fatal(v.Log)
	}
	assertRows(t, makeImage(t, cfg(t, "n1"), history(), 4), v.Image)
	if _, e = os.Stat(filepath.Join(root, "gen-1", "wal")); e != nil {
		t.Fatal("old divergent WAL deleted", e)
	}
}
func TestInstallRejectsMismatchWithinCommittedAfterApplyDrain(t *testing.T) {
	root := seed(t, nil, 0)
	r := receiver(t, root)
	image, _ := source(t)
	local := append([]storageformat.LogEntry(nil), history()...)
	for i := 2; i < len(local); i++ {
		var body map[string]any
		_ = json.Unmarshal(local[i].Raw, &body)
		body["term"] = "3"
		raw, _ := json.Marshal(body)
		local[i].Raw = raw
		local[i].Term = 3
	}
	if _, e := r.Handle(deadline(t), 1, appendMessage(t, 0, 0, 10, local, 5)); e != nil {
		t.Fatal(e)
	}
	old := current(t, root)
	if _, e := r.Handle(deadline(t), 2, chunk(t, image, 0, len(image), "n1", 5, nil)); !errors.Is(e, snapshot.ErrBoundary) {
		t.Fatal("committed mismatch accepted", e)
	}
	if !bytes.Equal(old, current(t, root)) || r.store.State().CommitIndex != 10 || r.store.State().LastIndex() != 11 {
		t.Fatal("committed log lost")
	}
	v, e := r.View(deadline(t))
	if e != nil || v.Status.Raft.LastApplied != 10 || v.Log.BaseIndex != 0 {
		t.Fatal(v.Status, e)
	}
}
func TestObsoleteSnapshotBelowLocalBaseUsesMatchingResumeBoundary(t *testing.T) {
	root := seed(t, history()[:2], 2)
	r := receiver(t, root)
	image, log := source(t)
	if _, e := r.Handle(deadline(t), 1, chunk(t, image, 0, len(image), "n1", 5, nil)); e != nil {
		t.Fatal(e)
	}
	newer := imageBytes(t, makeImage(t, cfg(t, "n1"), history(), 7))
	req := chunk(t, newer, 0, len(newer), "n1", 5, func(b map[string]any) { b["last_included_index"] = "7" })
	if _, e := r.Handle(deadline(t), 2, req); e != nil {
		t.Fatal(e)
	}
	tr, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 65536)
	if e != nil {
		t.Fatal(e)
	}
	req, e = tr.NextSnapshot(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	res, e := r.Handle(deadline(t), 3, req)
	if e != nil {
		t.Fatal(e)
	}
	b := response(t, res)
	if b["reason"] != "obsolete" || b["resume_index"] != "7" || b["resume_term"] != "2" {
		t.Fatal(b)
	}
	if e = tr.Observe(5, "gen-9", res); e != nil || tr.Progress().MatchIndex != 0 || tr.Progress().NextIndex != 8 {
		t.Fatal(tr.Progress(), e)
	}
}
func TestMalformedWireAndMaximumBoundaryNeverPublish(t *testing.T) {
	root := seed(t, nil, 0)
	r := receiver(t, root)
	image, _ := source(t)
	old := current(t, root)
	good := chunk(t, image, 0, 64, "n1", 5, nil)
	for _, mode := range []string{"total", "chunk", "cluster", "version", "identity"} {
		var b map[string]any
		_ = json.Unmarshal(good.Bytes(), &b)
		switch mode {
		case "total":
			b["total_bytes"] = "8388609"
		case "chunk":
			b["data_b64"] = base64.StdEncoding.EncodeToString(make([]byte, 65537))
			b["total_bytes"] = "100000"
		case "cluster":
			b["cluster_id"] = strings.Repeat("f", 32)
		case "version":
			b["protocol_version"] = 2
		case "identity":
			b["from"] = "n4"
			b["leader_id"] = "n4"
		}
		raw, _ := json.Marshal(b)
		if _, e := peerprotocol.DecodeRequest(cfg(t, "n2"), peerprotocol.Snapshot, raw, "n2"); e == nil {
			t.Fatal("wire accepted", mode)
		}
	}
	if r.Status().Raft.Term != 4 {
		t.Fatal("invalid envelope changed term")
	}
	maximum := storageformat.Snapshot{ClusterID: cfg(t, "n1").ClusterID(), ConfigHash: cfg(t, "n1").Hash(), Index: ^uint64(0), Term: 2}
	raw := imageBytes(t, maximum)
	req := chunk(t, raw, 0, len(raw), "n1", 5, func(b map[string]any) { b["last_included_index"] = fmt.Sprint(^uint64(0)) })
	res, e := r.Handle(deadline(t), 1, req)
	if e != nil || response(t, res)["accepted"] != false || !bytes.Equal(old, current(t, root)) {
		t.Fatal("overflow boundary activated", e)
	}
}
func TestInstallActualSessionCapAndRetryWithoutEviction(t *testing.T) {
	entries := make([]storageformat.LogEntry, 1024)
	for i := range entries {
		raw, _ := json.Marshal(map[string]any{"index": fmt.Sprint(i + 1), "term": "2", "kind": "put", "command": map[string]any{"key": "x", "value_b64": "QQ==", "client_id": fmt.Sprintf("%032x", i), "sequence": 1}})
		entries[i] = storageformat.LogEntry{Index: uint64(i + 1), Term: 2, Raw: raw}
	}
	c := cfg(t, "n1")
	image := imageBytes(t, makeImage(t, c, entries, 1024))
	log := wal.State{BaseIndex: 1024, BaseTerm: 2, HardState: storageformat.HardState{Term: 5}, CommitIndex: 1024, Sequence: 1}
	r := receiver(t, seed(t, nil, 0))
	tr, e := NewTransfer(c, "n2", "gen-9", image, log, 1, 0, 65536)
	if e != nil {
		t.Fatal(e)
	}
	now := uint64(1)
	sendTransfer(t, r, tr, &now)
	raw, _ := json.Marshal(map[string]any{"index": "1025", "term": "5", "kind": "put", "command": map[string]any{"key": "x", "value_b64": "Qg==", "client_id": fmt.Sprintf("%032x", 2048), "sequence": 1}})
	req := appendMessage(t, 1024, 2, 1025, []storageformat.LogEntry{{Index: 1025, Term: 5, Raw: raw}}, 5)
	now++
	if _, e = r.Handle(deadline(t), now, req); e != nil {
		t.Fatal(e)
	}
	v, e := r.View(deadline(t))
	if e != nil || len(v.Image.Sessions) != 1024 || v.Last.Result.Code != committed.SessionLimit || string(v.Image.KV[0].Value) != "A" {
		t.Fatal(v.Last, e)
	}
	raw, _ = json.Marshal(map[string]any{"index": "1026", "term": "5", "kind": "put", "command": map[string]any{"key": "x", "value_b64": "Sw==", "client_id": fmt.Sprintf("%032x", 0), "sequence": 2}})
	req = appendMessage(t, 1025, 5, 1026, []storageformat.LogEntry{{Index: 1026, Term: 5, Raw: raw}}, 5)
	now++
	if _, e = r.Handle(deadline(t), now, req); e != nil {
		t.Fatal(e)
	}
	v, e = r.View(deadline(t))
	if e != nil || len(v.Image.Sessions) != 1024 || v.Image.Sessions[0].Sequence != 2 || string(v.Image.KV[0].Value) != "K" {
		t.Fatal(v.Image, e)
	}
}
