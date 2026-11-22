package server

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

func TestCommandValidationIsBoundedAndRedacted(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{{nil, 2}, {[]string{"-unknown=PRIVATE"}, 2}, {[]string{"node", "-h"}, 0}, {[]string{"-config", "missing"}, 2}, {[]string{"-config", "missing", "extra"}, 2}, {[]string{"-bootstrap=invalid"}, 2}} {
		var out, errOut bytes.Buffer
		if code := Run(context.Background(), tc.args, &out, &errOut); code != tc.code {
			t.Fatal(code, tc)
		}
		if strings.Contains(errOut.String(), "PRIVATE") || strings.Contains(errOut.String(), "missing") {
			t.Fatal("raw argument leaked", errOut.String())
		}
	}
	var out bytes.Buffer
	if code := Run(nil, nil, &out, &out); code != 2 {
		t.Fatal(code)
	}
	for _, raw := range []string{`{"secret":"PRIVATE"}`, strings.Repeat(" ", 16385), "{} {}"} {
		p := filepath.Join(t.TempDir(), "config.json")
		if e := os.WriteFile(p, []byte(raw), 0600); e != nil {
			t.Fatal(e)
		}
		out.Reset()
		if code := Run(context.Background(), []string{"-config", p}, &out, &out); code != 2 || strings.Contains(out.String(), "PRIVATE") {
			t.Fatal(code, out.String())
		}
	}
}
func directoryImage(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	image := map[string][32]byte{}
	e := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			rel, e := filepath.Rel(root, p)
			if e != nil {
				return e
			}
			image[rel] = sha256.Sum256(b)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return image
}
func TestServerStartupFailsClosedAndDoesNotRebootstrap(t *testing.T) {
	cs, paths := fixtureConfigs(t)
	c := cs[0]
	var out bytes.Buffer
	// Ordinary restart of an empty directory never initializes it.
	if code := Run(context.Background(), []string{"-config", paths[0]}, &out, &out); code != 1 {
		t.Fatal(code)
	}
	if _, e := os.Stat(filepath.Join(c.DataDir(), "CURRENT")); !os.IsNotExist(e) {
		t.Fatal("implicit bootstrap")
	}
	store, e := wal.Bootstrap(c.DataDir(), id(c))
	if e != nil {
		t.Fatal(e)
	}
	_ = store.Close()
	before := directoryImage(t, c.DataDir())
	out.Reset()
	if code := Run(context.Background(), []string{"-config", paths[0], "-bootstrap"}, &out, &out); code != 1 {
		t.Fatal(code)
	}
	after := directoryImage(t, c.DataDir())
	if len(after) != len(before) {
		t.Fatal("bootstrap changed initialized directory")
	}
	for p, h := range before {
		if after[p] != h {
			t.Fatal("bootstrap overwrote data", p)
		}
	}
	// The lock must be released after failed startup, but a second owner cannot
	// start while another process owns it.
	p := launch(t, paths[0], false)
	poll(t, func() bool { _, ok := status(c); return ok })
	out.Reset()
	if code := Run(context.Background(), []string{"-config", paths[0]}, &out, &out); code != 1 {
		t.Fatal("duplicate owner admitted", code)
	}
	p.stop(t, true)
	// Full-frame corruption must fail before binding either endpoint; no fallback.
	walPath := filepath.Join(c.DataDir(), "gen-1", "wal")
	b, e := os.ReadFile(walPath)
	if e != nil {
		t.Fatal(e)
	}
	b[len(b)-1] ^= 1
	if e = os.WriteFile(walPath, b, 0600); e != nil {
		t.Fatal(e)
	}
	before = directoryImage(t, c.DataDir())
	out.Reset()
	if code := Run(context.Background(), []string{"-config", paths[0]}, &out, &out); code != 1 {
		t.Fatal("corrupt state started", code)
	}
	after = directoryImage(t, c.DataDir())
	for p, h := range before {
		if after[p] != h {
			t.Fatal("corruption was silently repaired", p)
		}
	}
	for _, addr := range []string{c.Local().Peer, c.Local().Client} {
		l, e := net.Listen("tcp", addr)
		if e != nil {
			t.Fatal("startup listened before validation", e)
		}
		_ = l.Close()
	}
}
func TestCanceledServerDoesNotInitializeStorage(t *testing.T) {
	cs, paths := fixtureConfigs(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out bytes.Buffer
	if code := Run(ctx, []string{"node", "-config", paths[0], "-bootstrap"}, &out, &out); code != 0 {
		t.Fatal(code)
	}
	if _, e := os.Stat(filepath.Join(cs[0].DataDir(), "CURRENT")); !os.IsNotExist(e) {
		t.Fatal("canceled server initialized storage")
	}
}

func TestExampleConfigurationsShareIdentityAndUseDistinctDirectories(t *testing.T) {
	var hash string
	dirs := map[string]bool{}
	for i := 1; i <= 3; i++ {
		c, e := readConfig(fmt.Sprintf("../../examples/n%d.json", i))
		if e != nil {
			t.Fatal(e)
		}
		if c.NodeID() != fmt.Sprintf("n%d", i) || dirs[c.DataDir()] {
			t.Fatal("example identity")
		}
		dirs[c.DataDir()] = true
		if hash != "" && hash != c.Hash() {
			t.Fatal("membership differs")
		}
		hash = c.Hash()
	}
}

type failedOutput struct{}

func (failedOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestEmbeddedRunCancellationAndOutputFailureReleaseResources(t *testing.T) {
	for _, broken := range []bool{false, true} {
		t.Run(fmt.Sprint(broken), func(t *testing.T) {
			cs, paths := fixtureConfigs(t)
			c := cs[0]
			ctx, cancel := context.WithCancel(testContext(t))
			defer cancel()
			var output boundedLog
			var out io.Writer = &output
			if broken {
				out = failedOutput{}
			}
			done := make(chan int, 1)
			go func() { done <- Run(ctx, []string{"node", "-config", paths[0], "-bootstrap"}, out, &output) }()
			if !broken {
				poll(t, func() bool { _, ok := status(c); return ok })
				// An incomplete peer request is live when the process context is canceled.
				conn, e := net.DialTimeout("tcp", c.Local().Peer, time.Second)
				if e != nil {
					t.Fatal(e)
				}
				_, _ = io.WriteString(conn, "POST /raft/append HTTP/1.1\r\nHost: "+c.Local().Peer+"\r\n")
				defer conn.Close()
				cancel()
			}
			select {
			case code := <-done:
				want := 0
				if broken {
					want = 1
				}
				if code != want {
					t.Fatal(code, output.String())
				}
			case <-testContext(t).Done():
				t.Fatal("server did not release resources")
			}
			w := readStore(t, c)
			_ = w.Close()
			for _, addr := range []string{c.Local().Peer, c.Local().Client} {
				l, e := net.Listen("tcp", addr)
				if e != nil {
					t.Fatal("resource leaked", e)
				}
				_ = l.Close()
			}
		})
	}
}
