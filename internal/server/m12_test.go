package server

import (
	"bytes"
	"context"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiagnosticRotationLifecycleAndDataIsolation(t *testing.T) {
	cs, paths := fixtureConfigs(t)
	logs := t.TempDir()
	_ = os.Chmod(logs, 0700)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	var out, errOut bytes.Buffer
	go func() { done <- Run(ctx, []string{"-config", paths[0], "-bootstrap", "-log-dir", logs}, &out, &errOut) }()
	poll(t, func() bool { _, ok := status(cs[0]); return ok })
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatal(code, errOut.String())
		}
	case <-testContext(t).Done():
		t.Fatal("shutdown timed out")
	}
	raw, e := os.ReadFile(filepath.Join(logs, "quorumkv.jsonl"))
	if e != nil || !bytes.Contains(raw, []byte(`"event":"startup"`)) || !bytes.Contains(raw, []byte(`"event":"shutdown"`)) {
		t.Fatal(string(raw), e)
	}
	if bytes.Contains(raw, []byte(cs[0].DataDir())) {
		t.Fatal("private path leaked")
	}
	// Rotation and startup leave the active directory's original layout intact.
	store, e := wal.Open(cs[0].DataDir(), id(cs[0]))
	if e != nil {
		t.Fatal(e)
	}
	_ = store.Close()
	before := directoryImage(t, cs[0].DataDir())
	errOut.Reset()
	if code := Run(context.Background(), []string{"-config", paths[0], "-log-dir", cs[0].DataDir()}, &out, &errOut); code != 2 {
		t.Fatal(code)
	}
	after := directoryImage(t, cs[0].DataDir())
	for name, h := range before {
		if after[name] != h {
			t.Fatal("data changed")
		}
	}
	if strings.Contains(errOut.String(), cs[0].DataDir()) {
		t.Fatal("path echoed")
	}
}
