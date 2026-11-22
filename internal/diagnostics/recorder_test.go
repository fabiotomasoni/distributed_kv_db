package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestBoundedRedactedConcurrentObservations(t *testing.T) {
	d := New("n1", strings.Repeat("1", 32))
	var wg sync.WaitGroup
	for j := 0; j < 4; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 600; i++ {
				d.Inc("client_completed")
				d.Inc("secret-key")
				d.Observe("request", int64(i))
				d.Observe("secret-value", 1)
				d.Log(Record{Event: Request, Role: "secret-value", LeaderID: "secret-key", Code: "secret-value", Outcome: "secret-value", RequestID: "/private/secret"})
			}
		}()
	}
	wg.Wait()
	s := d.Snapshot()
	if s.Counters["client_completed"] != 2400 || len(s.Counters) != 1 || len(s.Durations) != 1 || s.Durations["request"].Count != 2400 || len(s.Durations["request"].Samples) != 2048 || s.DroppedLogs != 2400-256 {
		t.Fatal(s)
	}
	s.Counters["client_completed"] = 0
	s.Durations["request"].Samples[0] = -1
	if d.Snapshot().Counters["client_completed"] != 2400 || d.Snapshot().Durations["request"].Samples[0] < 0 {
		t.Fatal("snapshot aliases retained state")
	}
	logs := d.Drain()
	if len(logs) != 256 || len(d.Drain()) != 0 {
		t.Fatal("drain")
	}
	for _, log := range logs {
		raw := Encode(log)
		if len(raw) > 1024 || strings.Contains(string(raw), "secret") || !json.Valid(raw) {
			t.Fatal("redaction", string(raw))
		}
	}
	d.Log(Record{Event: StorageError, Code: "STORAGE_UNAVAILABLE"})
	for i := 0; i < 257; i++ {
		d.Log(Record{Event: Applied})
	}
	logs = d.Drain()
	if len(logs) != 257 || logs[0].Event != StorageError {
		t.Fatal("fatal record was sampled away")
	}
}
func TestRotationBoundsAndPrivatePaths(t *testing.T) {
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	r, e := OpenRotation(dir)
	if e != nil {
		t.Fatal(e)
	}
	line := []byte(strings.Repeat("x", 1023) + "\n")
	for i := 0; i < 1025; i++ {
		if _, e = r.Write(line); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = r.Write(append(line, 'x')); e == nil {
		t.Fatal("oversized record")
	}
	if e = r.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = r.Write(line); e == nil {
		t.Fatal("closed writer")
	}
	files, e := os.ReadDir(dir)
	if e != nil || len(files) != 4 {
		t.Fatal(files, e)
	}
	var total int64
	for _, f := range files {
		s, e := f.Info()
		if e != nil || s.Size() > MaxLogBytes || s.Mode().Perm() != 0600 {
			t.Fatal(s, e)
		}
		total += s.Size()
	}
	if total > 1024*1024 {
		t.Fatal("rotation cap")
	}
	bad := t.TempDir()
	_ = os.Chmod(bad, 0700)
	if e = os.Symlink(filepath.Join(dir, "quorumkv.jsonl"), filepath.Join(bad, "quorumkv.jsonl")); e != nil {
		t.Fatal(e)
	}
	if _, e = OpenRotation(bad); e == nil {
		t.Fatal("symlink accepted")
	}
	_ = os.Chmod(bad, 0755)
	if _, e = OpenRotation(bad); e == nil {
		t.Fatal("public directory accepted")
	}
}
