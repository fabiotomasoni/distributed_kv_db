package snapshotinstall

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/snapshot"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func deadline(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	t.Cleanup(cancel)
	return c
}
func cfg(t *testing.T, node string) config.Config {
	t.Helper()
	b, e := os.ReadFile("../../docs/fixtures/configuration-v2-sessions.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct{ Configuration map[string]any }
	if e = json.Unmarshal(b, &f); e != nil {
		t.Fatal(e)
	}
	for _, v := range f.Configuration["members"].([]any) {
		m := v.(map[string]any)
		if m["id"] == node {
			f.Configuration["node_id"] = node
			f.Configuration["client_addr"] = m["client"]
			f.Configuration["peer_addr"] = m["peer"]
		}
	}
	b, _ = json.Marshal(f.Configuration)
	c, e := config.Parse(b)
	if e != nil {
		t.Fatal(e)
	}
	return c
}
func entry(index, term uint64, kind, client string, sequence uint64, value string, expected any) storageformat.LogEntry {
	o := map[string]any{"index": fmt.Sprint(index), "term": fmt.Sprint(term), "kind": kind}
	if kind != "noop" {
		command := map[string]any{"key": "x"}
		if kind != "read" {
			command["client_id"] = strings.Repeat(client, 32)
			command["sequence"] = sequence
		}
		if kind == "put" {
			command["value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
		}
		if kind == "cas" {
			command["expected"] = expected
			command["new_value_b64"] = base64.StdEncoding.EncodeToString([]byte(value))
		}
		o["command"] = command
	}
	b, _ := json.Marshal(o)
	return storageformat.LogEntry{Index: index, Term: term, Raw: b}
}
func present(v string) any {
	return map[string]any{"exists": true, "value_b64": base64.StdEncoding.EncodeToString([]byte(v))}
}
func history() []storageformat.LogEntry {
	return []storageformat.LogEntry{
		entry(1, 1, "put", "a", 1, "A", nil), entry(2, 1, "cas", "b", 1, "B", present("A")), entry(3, 2, "cas", "c", 1, "C", present("A")), entry(4, 2, "read", "", 0, "", nil), entry(5, 2, "delete", "d", 1, "", nil), entry(6, 2, "put", "a", 2, "D", nil), entry(7, 2, "put", "a", 2, "D", nil), entry(8, 2, "put", "a", 1, "A", nil), entry(9, 2, "noop", "", 0, "", nil), entry(10, 2, "read", "", 0, "", nil), entry(11, 2, "put", "e", 1, "speculative", nil),
	}
}
func makeImage(t *testing.T, c config.Config, entries []storageformat.LogEntry, index uint64) storageformat.Snapshot {
	t.Helper()
	m, e := snapshot.NewMachine(c)
	if e != nil {
		t.Fatal(e)
	}
	a, e := applier.New(m, applier.Config{Capacity: 1})
	if e != nil {
		t.Fatal(e)
	}
	defer a.Shutdown(context.Background())
	for _, v := range entries {
		if v.Index > index {
			break
		}
		command, e := committed.ProjectWithSessions(v)
		if e != nil {
			t.Fatal(e)
		}
		f, e := a.Submit(deadline(t), applier.Entry{Index: v.Index, Command: command})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Wait(deadline(t)); e != nil {
			t.Fatal(e)
		}
	}
	b, e := a.Park(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	defer b.Release(context.Background())
	term := uint64(0)
	if index > 0 {
		term = entries[index-1].Term
	}
	s, e := snapshot.Capture(deadline(t), b, snapshot.Proof{LastApplied: index, ScheduledApplyIndex: index, CommitIndex: index, ApplyEpoch: 1, TermAt: func(uint64) (uint64, error) { return term, nil }})
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func imageBytes(t *testing.T, s storageformat.Snapshot) []byte {
	t.Helper()
	b, e := storageformat.EncodeSnapshot(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func source(t *testing.T) ([]byte, wal.State) {
	t.Helper()
	c := cfg(t, "n1")
	s := makeImage(t, c, history(), 4)
	return imageBytes(t, s), wal.State{BaseIndex: 4, BaseTerm: 2, HardState: storageformat.HardState{Term: 5, Vote: "n1"}, CommitIndex: 10, Sequence: 3, Entries: history()[4:]}
}
func seed(t *testing.T, entries []storageformat.LogEntry, commit uint64) string {
	t.Helper()
	root := t.TempDir()
	if e := os.Chmod(root, 0700); e != nil {
		t.Fatal(e)
	}
	c := cfg(t, "n2")
	s, e := wal.BootstrapSessions(root, wal.Identity{ClusterID: c.ClusterID(), NodeID: c.NodeID(), ConfigHash: c.Hash()})
	if e != nil {
		t.Fatal(e)
	}
	tx := map[string]any{"hard_state": map[string]any{"current_term": "4", "voted_for": "n2"}, "commit_index": fmt.Sprint(commit)}
	if len(entries) > 0 {
		rows := make([]json.RawMessage, len(entries))
		for i, e := range entries {
			rows[i] = e.Raw
		}
		tx["replace_suffix"] = map[string]any{"from_index": "1", "entries": rows}
	}
	raw, _ := json.Marshal(tx)
	if _, e = s.Append(raw); e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	return root
}
func receiver(t *testing.T, root string) *Receiver {
	t.Helper()
	r, e := Open(root, cfg(t, "n2"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = r.Close() })
	return r
}
func sendTransfer(t *testing.T, r *Receiver, tr *Transfer, now *uint64) peerprotocol.Message {
	t.Helper()
	var final peerprotocol.Message
	for n := 0; n < 1000 && !tr.Progress().Finished; n++ {
		req, e := tr.NextSnapshot(5, "gen-9")
		if e != nil {
			t.Fatal(e)
		}
		*now++
		res, e := r.Handle(deadline(t), *now, req)
		if e != nil {
			t.Fatal(e)
		}
		if e = tr.Observe(5, "gen-9", res); e != nil {
			t.Fatal(e)
		}
		if !tr.Progress().Finished && tr.Progress().MatchIndex != 0 {
			t.Fatal("chunk ack counted as replication")
		}
		final = req
	}
	if !tr.Progress().Finished {
		t.Fatal("transfer budget exhausted")
	}
	return final
}
func assertRows(t *testing.T, a, b storageformat.Snapshot) {
	t.Helper()
	if a.Index != b.Index || a.Term != b.Term || a.LogicalBytes != b.LogicalBytes || !reflect.DeepEqual(a.KV, b.KV) || !reflect.DeepEqual(a.Sessions, b.Sessions) {
		t.Fatalf("first application divergence: expected=%#v actual=%#v", a, b)
	}
}
func TestInstallCatchupAndOrdinaryReplicationPreserveFullReference(t *testing.T) {
	root := seed(t, history()[:2], 2)
	r := receiver(t, root)
	image, log := source(t)
	tr, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 71)
	if e != nil {
		t.Fatal(e)
	}
	now := uint64(1)
	final := sendTransfer(t, r, tr, &now)
	if tr.Progress().MatchIndex != 4 || tr.Progress().NextIndex != 5 {
		t.Fatal(tr.Progress())
	}
	before, e := r.View(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if before.Log.BaseIndex != 4 || before.Log.CommitIndex != 4 || before.Log.LastIndex() != 4 || before.Status.ApplyEpoch != 2 || before.Log.HardState.Term != 5 || before.Log.HardState.Vote != "" {
		t.Fatal(before)
	}
	assertRows(t, makeImage(t, cfg(t, "n1"), history(), 4), before.Image)
	req, e := tr.NextAppend(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	now++
	res, e := r.Handle(deadline(t), now, req)
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Observe(5, "gen-9", res); e != nil {
		t.Fatal(e)
	}
	v, e := r.View(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	assertRows(t, makeImage(t, cfg(t, "n1"), history(), 10), v.Image)
	if v.Log.LastIndex() != 11 || v.Log.CommitIndex != 10 || v.Status.Raft.LastApplied != 10 || tr.Progress().MatchIndex != 11 {
		t.Fatal(v.Status, tr.Progress())
	}
	if e = r.core.Applied(1, 100, errors.New("obsolete completion")); e != nil || r.Status().Raft.LastApplied != 10 {
		t.Fatal("stale epoch changed state", e)
	}
	now++
	res, e = r.Handle(deadline(t), now, final)
	if e != nil {
		t.Fatal(e)
	}
	var body map[string]any
	_ = json.Unmarshal(res.Bytes(), &body)
	if body["reason"] != "obsolete" || r.Status().ApplyEpoch != 2 {
		t.Fatal("duplicate installed twice", body)
	}
	for i := 0; i < 2; i++ {
		if e = r.Close(); e != nil {
			t.Fatal(e)
		}
		r = receiver(t, root)
		v, e = r.View(deadline(t))
		if e != nil {
			t.Fatal(e)
		}
		assertRows(t, makeImage(t, cfg(t, "n1"), history(), 10), v.Image)
	}
	// Replace only the still speculative entry 11, then retry both cached CAS
	// outcomes. Failed CAS at 3 must stay failed even when x becomes A again.
	entries := []storageformat.LogEntry{entry(11, 5, "put", "f", 1, "A", nil), entry(12, 5, "cas", "c", 1, "C", present("A")), entry(13, 5, "cas", "b", 1, "B", present("A")), entry(14, 5, "put", "a", 1, "A", nil), entry(15, 5, "read", "", 0, "", nil)}
	req = appendMessage(t, 10, 2, 15, entries, 5)
	now++
	if _, e = r.Handle(deadline(t), now, req); e != nil {
		t.Fatal(e)
	}
	v, e = r.View(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if string(v.Image.KV[0].Value) != "A" || v.Image.Sessions[1].Result.Index != 2 || v.Image.Sessions[2].Result.Index != 3 || v.Image.Sessions[2].Result.Code != "COMPARE_FAILED" || v.Image.Sessions[0].Sequence != 2 {
		t.Fatal(v.Image)
	}
}
func appendMessage(t *testing.T, prev, pt, commit uint64, entries []storageformat.LogEntry, term uint64) peerprotocol.Message {
	t.Helper()
	rows := []json.RawMessage{}
	for _, e := range entries {
		rows = append(rows, e.Raw)
	}
	return request(t, peerprotocol.Append, "n1", term, map[string]any{"leader_id": "n1", "prev_log_index": fmt.Sprint(prev), "prev_log_term": fmt.Sprint(pt), "entries": rows, "leader_commit": fmt.Sprint(commit)})
}
func request(t *testing.T, kind peerprotocol.Kind, from string, term uint64, body map[string]any) peerprotocol.Message {
	t.Helper()
	c := cfg(t, "n2")
	body["protocol_version"] = 1
	body["cluster_id"] = c.ClusterID()
	body["config_hash"] = c.Hash()
	body["from"] = from
	body["to"] = "n2"
	body["rpc_id"] = "test-request"
	body["term"] = fmt.Sprint(term)
	raw, _ := json.Marshal(body)
	m, e := peerprotocol.DecodeRequest(c, kind, raw, "n2")
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func TestInstallLostFinalResponseUsesObsoleteThenVerifiedAppend(t *testing.T) {
	r := receiver(t, seed(t, history()[:2], 2))
	image, log := source(t)
	tr, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 64*1024)
	if e != nil {
		t.Fatal(e)
	}
	req, e := tr.NextSnapshot(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Handle(deadline(t), 1, req); e != nil {
		t.Fatal(e)
	} // final response lost
	if tr.Progress().MatchIndex != 0 {
		t.Fatal("lost response advanced match")
	}
	repeat, e := tr.NextSnapshot(5, "gen-9")
	if e != nil || !bytes.Equal(req.Bytes(), repeat.Bytes()) {
		t.Fatal(e)
	}
	res, e := r.Handle(deadline(t), 2, repeat)
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Observe(5, "gen-9", res); e != nil {
		t.Fatal(e)
	}
	if tr.Progress().MatchIndex != 0 || tr.Progress().NextIndex != 5 || !tr.Progress().AwaitAppend {
		t.Fatal(tr.Progress())
	}
	req, e = tr.NextAppend(5, "gen-9")
	if e != nil {
		t.Fatal(e)
	}
	res, e = r.Handle(deadline(t), 3, req)
	if e != nil {
		t.Fatal(e)
	}
	if e = tr.Observe(5, "gen-9", res); e != nil {
		t.Fatal(e)
	}
	if tr.Progress().MatchIndex != 11 || tr.Progress().AwaitAppend {
		t.Fatal(tr.Progress())
	}
}
func TestInstallRetainsMatchingCommittedAndSpeculativeLocalSuffix(t *testing.T) {
	root := seed(t, nil, 0)
	r := receiver(t, root)
	image, log := source(t)
	// One ordinary Append schedules only slot 1. C=10 and the remaining entries
	// are durable but not yet scheduled when the transfer starts.
	if _, e := r.Handle(deadline(t), 1, appendMessage(t, 0, 0, 10, history(), 5)); e != nil {
		t.Fatal(e)
	}
	tr, e := NewTransfer(cfg(t, "n1"), "n2", "gen-9", image, log, 1, 0, 64*1024)
	if e != nil {
		t.Fatal(e)
	}
	now := uint64(1)
	sendTransfer(t, r, tr, &now)
	v, e := r.View(deadline(t))
	if e != nil {
		t.Fatal(e)
	}
	if v.Log.CommitIndex != 10 || v.Log.LastIndex() != 11 || v.Log.BaseIndex != 4 || v.Status.Raft.LastApplied != 10 || v.Log.HardState.Vote != "" {
		t.Fatal(v)
	}
	assertRows(t, makeImage(t, cfg(t, "n1"), history(), 10), v.Image)
	for i, v := range v.Log.Entries {
		if !bytes.Equal(v.Raw, history()[i+4].Raw) {
			t.Fatal("retained suffix changed", v.Index)
		}
	}
}
