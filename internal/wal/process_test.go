package wal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storagefs"
)

func TestProcessRecovery(t *testing.T) {
	for _, stage := range []string{"written", "synced", "closed"} {
		t.Run(stage, func(t *testing.T) {
			s, root := newStore(t)
			closeOK(t, s)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWALChild$")
			cmd.Env = append(os.Environ(), "QUORUMKV_WAL_ROOT="+root, "QUORUMKV_WAL_STAGE="+stage)
			out, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			in, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stderr = os.Stderr
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = in.Close()
				cancel()
				if cmd.ProcessState == nil {
					_ = cmd.Wait()
				}
			})
			line, err := bufio.NewReader(out).ReadString('\n')
			if err != nil || line != stage+"\n" {
				t.Fatalf("child readiness %q %v", line, err)
			}
			if stage != "closed" {
				if r, err := Open(root, testIdentity); r != nil || !errors.Is(err, storagefs.ErrLocked) {
					t.Fatal("live child lock lost", err)
				}
			}
			if stage == "closed" {
				_ = in.Close()
				if err = cmd.Wait(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err = cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				if err = cmd.Wait(); err == nil {
					t.Fatal("forced exit unexpectedly succeeded")
				}
			}
			r, err := Open(root, testIdentity)
			if err != nil {
				t.Fatal(err)
			}
			if r.Status().Sequence != 2 || r.Status().LastIndex != 1 {
				t.Fatal("complete process-surviving append lost", r.Status())
			}
			want := uint64(1)
			if stage == "written" {
				want = 0
			}
			if r.Status().CommitIndex != want {
				t.Fatal("presence inferred commit", r.Status())
			}
			if h := r.State().HardState; h.Term != 1 || h.Vote != "n1" {
				t.Fatal("recovered vote lost", h)
			}
			if _, err := r.Append(payload(t, map[string]any{"hard_state": hard(1, "n2")})); !errors.Is(err, ErrTransition) {
				t.Fatal("second vote allowed after restart", err)
			}
			closeOK(t, r)
		})
	}
}

type pauseAfterWrite struct{ walFile }

func (f *pauseAfterWrite) Write(b []byte) (int, error) {
	n, err := f.walFile.Write(b)
	if err == nil {
		fmt.Println("written")
		_, _ = io.Copy(io.Discard, os.Stdin)
	}
	return n, err
}

func TestWALChild(t *testing.T) {
	root := os.Getenv("QUORUMKV_WAL_ROOT")
	if root == "" {
		return
	}
	s, err := Open(root, testIdentity)
	if err != nil {
		t.Fatal(err)
	}
	stage := os.Getenv("QUORUMKV_WAL_STAGE")
	if stage == "written" {
		s.file = &pauseAfterWrite{s.file}
		_, err = s.Append(appendTx(t, 1, 1, false))
		if err != nil {
			t.Fatal(err)
		}
		t.Fatal("parent did not interrupt before Sync")
	}
	token, err := s.Append(appendTx(t, 1, 1, true))
	if err != nil || token.Sequence != 2 {
		t.Fatal(token, err)
	}
	if stage == "closed" {
		closeOK(t, s)
	}
	fmt.Println(stage)
	_, _ = io.Copy(io.Discard, os.Stdin)
	if stage != "closed" {
		closeOK(t, s)
	}
}
