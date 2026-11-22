package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExplicitSimulationCommandAndExclusiveEvidence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.json")
	var out, errOut bytes.Buffer
	if code := run([]string{"-scenario", "../../examples/m12/session-retry.json", "-output", path}, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
	if code := run([]string{"-replay", path}, &out, &errOut); code != 0 {
		t.Fatal(code, errOut.String())
	}
	if code := run([]string{"-scenario", "../../examples/m12/session-retry.json", "-output", path}, &out, &errOut); code != 1 {
		t.Fatal("overwrote evidence", code)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(original, after) {
		t.Fatal("existing evidence changed")
	}
	for _, args := range [][]string{nil, {"-scenario", "private-secret"}, {"-replay", "private-secret"}, {"-bad=private-secret"}} {
		errOut.Reset()
		if code := run(args, &out, &errOut); code != 2 {
			t.Fatal(code)
		}
		if bytes.Contains(errOut.Bytes(), []byte("private-secret")) {
			t.Fatal("path leaked")
		}
	}
}
