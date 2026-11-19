//go:build darwin

package storagefs

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

func current(t *testing.T, gen string) []byte {
	t.Helper()
	b, err := os.ReadFile("../../docs/fixtures/canonical-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Current string `json:"current_json"`
	}
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	var o map[string]any
	json.Unmarshal([]byte(f.Current), &o)
	delete(o, "checksum")
	o["generation"] = gen
	b, _ = json.Marshal(o)
	b, err = storageformat.SealManifest("CURRENT", b)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func validateCurrent(b []byte) error { return storageformat.ValidateManifest("CURRENT", b) }

// Test setup only: copy the approved empty image/frame into tiny pre-synced
// generations. This is not a production generation builder or snapshot engine.
func stageFixtureGenerations(t *testing.T, root string) {
	t.Helper()
	b, err := os.ReadFile("../../docs/fixtures/canonical-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Snapshot struct {
			Hex string `json:"image_hex"`
		}
		WAL []struct {
			Hex string `json:"frame_hex"`
		}
	}
	if err = json.Unmarshal(b, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, generation := range []string{"gen-1", "gen-2"} {
		dir := filepath.Join(root, generation)
		if err = os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string]string{"snapshot": fixtures.Snapshot.Hex, "wal": fixtures.WAL[0].Hex} {
			raw, err := hex.DecodeString(data)
			if err != nil {
				t.Fatal(err)
			}
			f, err := openPrivate(filepath.Join(dir, name), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL)
			if err != nil {
				t.Fatal(err)
			}
			if err = writeSyncClose(f, raw); err != nil {
				t.Fatal(err)
			}
		}
		if err = SyncDirectory(dir); err != nil {
			t.Fatal(err)
		}
	}
	if err = SyncDirectory(root); err != nil {
		t.Fatal(err)
	}
}

func verifyPublishedFixture(t *testing.T, root string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "CURRENT"))
	if err != nil || validateCurrent(b) != nil {
		t.Fatal("invalid CURRENT", err)
	}
	var manifest struct {
		Generation string
		SHA        string `json:"snapshot_sha256"`
		Index      string `json:"base_index"`
		Term       string `json:"base_term"`
	}
	if err = json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	image, err := os.ReadFile(filepath.Join(root, manifest.Generation, "snapshot"))
	if err != nil {
		t.Fatal(err)
	}
	if len(image) < 44 || string(image[:4]) != "QSN1" || binary.BigEndian.Uint64(image[4:12]) != uint64(len(image)-44) || fmt.Sprintf("%x", sha256.Sum256(image)) != manifest.SHA || !bytes.Equal(image[len(image)-32:], hashBytes(image[:len(image)-32])) || manifest.Index != "0" || manifest.Term != "0" {
		t.Fatal("referenced image not complete or coherent")
	}
	wal, err := os.ReadFile(filepath.Join(root, manifest.Generation, "wal"))
	if err != nil {
		t.Fatal(err)
	}
	f, n, err := storageformat.DecodeFrame(wal)
	if err != nil || n != len(wal) || f.Sequence != 1 || string(f.Payload) != `{"commit_index":"0","hard_state":{"current_term":"0","voted_for":null}}` {
		t.Fatal("referenced bootstrap frame invalid", err)
	}
}

func hashBytes(b []byte) []byte { h := sha256.Sum256(b); return h[:] }

func privateTemp(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	return root
}

func owner(t *testing.T, root string) *Lock {
	t.Helper()
	l, err := Acquire(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := l.Close(); err != nil {
			t.Error(err)
		}
	})
	return l
}

func TestLocalCapabilitiesAndPublication(t *testing.T) {
	root := privateTemp(t)
	v, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Go=%s OS=%s arch=%s filesystem=%s mount=%s device=%s test_root=%s", runtime.Version(), runtime.GOOS, runtime.GOARCH, v.Filesystem, v.Mount, v.Device, root)
	project, err := Inspect("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("project filesystem=%s mount=%s device=%s (read-only inspection)", project.Filesystem, project.Mount, project.Device)
	if v.Filesystem != "apfs" {
		t.Fatal("required local APFS profile unavailable")
	}
	l := owner(t, root)
	stageFixtureGenerations(t, root)
	if fd, _, err := syscall.Syscall(syscall.SYS_FCNTL, l.file.Fd(), syscall.F_GETFD, 0); err != 0 || fd&syscall.FD_CLOEXEC == 0 {
		t.Fatalf("close-on-exec: %d %v", fd, err)
	}
	f, err := openPrivate(filepath.Join(root, "wal"), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL)
	if err != nil {
		t.Fatal(err)
	}
	if err = WriteAll(f, []byte("small-synthetic-probe")); err != nil {
		t.Fatal(err)
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		t.Fatal("regular File.Sync:", err)
	}
	// One explicit capability probe distinguishes supported F_FULLFSYNC from
	// File.Sync's ENOTSUP fallback. Production never performs this extra flush.
	_, _, fullErr := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_FULLFSYNC, 0)
	closeErr := f.Close()
	if fullErr != 0 || closeErr != nil {
		t.Fatalf("F_FULLFSYNC=%v close=%v", fullErr, closeErr)
	}
	t.Log("regular File.Sync, direct F_FULLFSYNC and directory fsync completed successfully")
	if err = SyncDirectory(root); err != nil {
		t.Fatal("directory fsync:", err)
	}
	for _, name := range []string{"gen-1", "gen-2"} {
		want := current(t, name)
		if err = l.PublishCurrent(want, validateCurrent); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(root, "CURRENT"))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatal("incomplete replacement", err)
		}
		verifyPublishedFixture(t, root)
		if _, err = os.Lstat(filepath.Join(root, "CURRENT.tmp")); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("staging remained after success", err)
		}
	}
	for _, name := range []string{"LOCK", "wal", "CURRENT"} {
		st, err := os.Lstat(filepath.Join(root, name))
		if err != nil || st.Mode().Perm() != 0600 {
			t.Fatal(name, st, err)
		}
	}
}

func TestLockSeparateOpensAndClose(t *testing.T) {
	root := privateTemp(t)
	l := owner(t, root)
	st, err := os.Stat(filepath.Join(root, "LOCK"))
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Acquire(root); other != nil || !errors.Is(err, ErrLocked) {
		t.Fatalf("double open: %v", err)
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	if err = l.Close(); err != nil {
		t.Fatal(err)
	}
	other := owner(t, root)
	now, err := os.Stat(filepath.Join(root, "LOCK"))
	if err != nil || !os.SameFile(st, now) {
		t.Fatal("lock inode changed", err)
	}
	if err = l.PublishCurrent(current(t, "gen-1"), validateCurrent); !errors.Is(err, ErrClosed) {
		t.Fatal(err)
	}
	if err = other.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRejectUnsafeAndFailedOwner(t *testing.T) {
	root := privateTemp(t)
	link := filepath.Join(t.TempDir(), "linked")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	if l, err := Acquire(link); l != nil || !errors.Is(err, ErrUnsafe) {
		t.Fatal("accepted symlink root", err)
	}
	if err := os.Chmod(root, 0755); err != nil {
		t.Fatal(err)
	}
	if l, err := Acquire(root); l != nil || !errors.Is(err, ErrUnsafe) {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	l := owner(t, root)
	if err := l.PublishCurrent([]byte("bad"), validateCurrent); err == nil {
		t.Fatal("invalid data accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "CURRENT.tmp")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("validation touched staging", err)
	}
	// Existing staging is evidence, never truncated or automatically removed.
	if err := os.WriteFile(filepath.Join(root, "CURRENT.tmp"), []byte("retained"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := l.PublishCurrent(current(t, "gen-1"), validateCurrent); !errors.Is(err, os.ErrExist) {
		t.Fatal(err)
	}
	if err := l.PublishCurrent(current(t, "gen-1"), validateCurrent); !errors.Is(err, ErrFailed) {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "CURRENT.tmp"))
	if err != nil || string(got) != "retained" {
		t.Fatal("staging lost", err)
	}
}

func TestSymlinksAndMissingPathsAreRejected(t *testing.T) {
	root := privateTemp(t)
	target := filepath.Join(privateTemp(t), "target")
	if err := os.WriteFile(target, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "LOCK")); err != nil {
		t.Fatal(err)
	}
	if l, err := Acquire(root); l != nil || err == nil {
		t.Fatal("LOCK symlink accepted")
	}
	other := privateTemp(t)
	l := owner(t, other)
	if err := os.Symlink(target, filepath.Join(other, "CURRENT")); err != nil {
		t.Fatal(err)
	}
	if err := l.PublishCurrent(current(t, "gen-1"), validateCurrent); !errors.Is(err, ErrUnsafe) {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "untouched" {
		t.Fatal("symlink target modified", err)
	}
	if err := SyncDirectory(filepath.Join(other, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if _, err := Inspect(filepath.Join(other, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	fifoRoot := privateTemp(t)
	if err := syscall.Mkfifo(filepath.Join(fifoRoot, "LOCK"), 0600); err != nil {
		t.Fatal(err)
	}
	if l, err := Acquire(fifoRoot); l != nil || !errors.Is(err, ErrUnsafe) {
		t.Fatal("special LOCK file accepted", err)
	}
}

func TestInterruptedSyscallsRetryOnlyEINTR(t *testing.T) {
	calls := 0
	err := interrupted(func() error {
		calls++
		if calls < 3 {
			return syscall.EINTR
		}
		return syscall.EIO
	})
	if calls != 3 || !errors.Is(err, syscall.EIO) {
		t.Fatal(calls, err)
	}
}

func child(t *testing.T, root, action, cut string) (*exec.Cmd, *bufio.Reader, io.WriteCloser) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStorageChild$")
	cmd.Env = append(os.Environ(), "QUORUMKV_STORAGE_CHILD="+action, "QUORUMKV_STORAGE_ROOT="+root, "QUORUMKV_STORAGE_CUT="+cut)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// Every caller joins; cancellation is a final bounded cleanup for failed assertions.
	t.Cleanup(func() {
		_ = in.Close()
		cancel()
		if cmd.ProcessState == nil {
			_ = cmd.Wait()
		}
	})
	return cmd, bufio.NewReader(out), in
}

func TestLockAcrossProcessAndAbruptExit(t *testing.T) {
	for _, action := range []string{"close", "kill"} {
		t.Run(action, func(t *testing.T) {
			root := privateTemp(t)
			cmd, out, in := child(t, root, "hold", "")
			line, err := out.ReadString('\n')
			if err != nil || line != "LOCKED\n" {
				t.Fatalf("child readiness %q %v", line, err)
			}
			if l, err := Acquire(root); l != nil || !errors.Is(err, ErrLocked) {
				t.Fatal("interprocess exclusion failed", err)
			}
			if action == "kill" {
				if err = cmd.Process.Kill(); err != nil {
					t.Fatal(err)
				}
				if err = cmd.Wait(); err == nil {
					t.Fatal("killed child returned success")
				}
			} else {
				// A signal is unnecessary for orderly close: the child exits its controlled wait via a pipe.
				if err = in.Close(); err != nil {
					t.Fatal(err)
				}
				if err = cmd.Wait(); err != nil {
					t.Fatal(err)
				}
			}
			owner(t, root)
		})
	}
}

func TestPublicationProcessCutpoints(t *testing.T) {
	for _, cut := range []string{"before-rename", "after-rename"} {
		t.Run(cut, func(t *testing.T) {
			root := privateTemp(t)
			l := owner(t, root)
			stageFixtureGenerations(t, root)
			old := current(t, "gen-1")
			if err := l.PublishCurrent(old, validateCurrent); err != nil {
				t.Fatal(err)
			}
			if err := l.Close(); err != nil {
				t.Fatal(err)
			}
			cmd, out, _ := child(t, root, "publish", cut)
			line, err := out.ReadString('\n')
			if err != nil || line != "CUT\n" {
				t.Fatalf("cutpoint %q %v", line, err)
			}
			if err = cmd.Wait(); err == nil {
				t.Fatal("child cut did not interrupt")
			}
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != 77 {
				t.Fatalf("unexpected child termination: %v", err)
			}
			got, err := os.ReadFile(filepath.Join(root, "CURRENT"))
			if err != nil {
				t.Fatal(err)
			}
			want := old
			if cut == "after-rename" {
				want = current(t, "gen-2")
			}
			if !bytes.Equal(got, want) || validateCurrent(got) != nil {
				t.Fatal("torn or wrong visible CURRENT")
			}
			verifyPublishedFixture(t, root)
			owner(t, root) // The abruptly exited child cannot retain ownership.
		})
	}
}

func TestConcurrentCloseAndPublication(t *testing.T) {
	root := privateTemp(t)
	l := owner(t, root)
	stageFixtureGenerations(t, root)
	data := current(t, "gen-1")
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); <-start; results <- l.PublishCurrent(data, validateCurrent) }()
	go func() { defer wg.Done(); <-start; results <- l.Close() }()
	close(start)
	for i := 0; i < 2; i++ {
		if err := <-results; err != nil && !errors.Is(err, ErrClosed) {
			t.Fatal(err)
		}
	}
	wg.Wait()
	if got, err := os.ReadFile(filepath.Join(root, "CURRENT")); err == nil {
		if !bytes.Equal(got, data) {
			t.Fatal("partial file")
		}
		verifyPublishedFixture(t, root)
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	owner(t, root)
}

func TestDirectoryBarrierAndCloseErrors(t *testing.T) {
	closed := false
	err := syncDirectoryFile(123, func() error { closed = true; return syscall.EBADF }, func(fd int) error {
		if fd != 123 {
			t.Fatal(fd)
		}
		return syscall.EIO
	})
	if !closed || !errors.Is(err, syscall.EBADF) || !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
}

// Only the run-owned helper process executes these branches. No user process or
// active directory is discovered or signalled by name.
func TestStorageChild(t *testing.T) {
	action := os.Getenv("QUORUMKV_STORAGE_CHILD")
	if action == "" {
		return
	}
	l, err := Acquire(os.Getenv("QUORUMKV_STORAGE_ROOT"))
	if err != nil {
		t.Fatal(err)
	}
	if action == "hold" {
		fmt.Println("LOCKED")
		_, _ = io.Copy(io.Discard, os.Stdin)
		_ = l.Close()
		return
	}
	if action != "publish" {
		t.Fatal("invalid helper action")
	}
	ops := l.ops()
	rename := ops.rename
	ops.rename = func() error {
		if os.Getenv("QUORUMKV_STORAGE_CUT") == "before-rename" {
			fmt.Println("CUT")
			os.Exit(77)
		}
		if err := rename(); err != nil {
			return err
		}
		fmt.Println("CUT")
		os.Exit(77)
		return nil
	}
	if err = publish(current(t, "gen-2"), ops); err != nil {
		t.Fatal(err)
	}
	t.Fatal("cutpoint not reached")
}
