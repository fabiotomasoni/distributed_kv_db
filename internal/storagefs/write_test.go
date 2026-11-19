package storagefs

import (
	"bytes"
	"errors"
	"io"
	"os"
	"reflect"
	"syscall"
	"testing"
)

type step struct {
	n   int
	err error
}
type plannedWriter struct {
	steps []step
	calls int
	bytes []byte
}

func (w *plannedWriter) Write(b []byte) (int, error) {
	s := w.steps[w.calls]
	w.calls++
	if s.n >= 0 && s.n <= len(b) {
		w.bytes = append(w.bytes, b[:s.n]...)
	}
	return s.n, s.err
}

func TestShortWritesAndTerminalErrors(t *testing.T) {
	failure := errors.New("injected write error")
	for _, c := range []struct {
		name  string
		steps []step
		err   error
		calls int
		want  string
	}{
		{"short-success", []step{{2, nil}, {1, nil}}, nil, 2, "abc"},
		{"partial-error", []step{{1, failure}}, failure, 1, "a"},
		{"zero", []step{{0, nil}}, io.ErrNoProgress, 1, ""},
		{"negative", []step{{-1, nil}}, io.ErrShortWrite, 1, ""},
		{"over-count", []step{{4, nil}}, io.ErrShortWrite, 1, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := &plannedWriter{steps: c.steps}
			err := WriteAll(w, []byte("abc"))
			if !errors.Is(err, c.err) || w.calls != c.calls || string(w.bytes) != c.want {
				t.Fatalf("%v %+v", err, w)
			}
		})
	}
}

type fakeFile struct {
	events   *[]string
	data     bytes.Buffer
	fail     string
	injected error
}

func (f *fakeFile) Write(b []byte) (int, error) {
	*f.events = append(*f.events, "write")
	if f.fail == "write" {
		return 0, f.injected
	}
	return f.data.Write(b)
}
func (f *fakeFile) Sync() error {
	*f.events = append(*f.events, "sync")
	if f.fail == "sync" {
		return f.injected
	}
	return nil
}
func (f *fakeFile) Close() error {
	*f.events = append(*f.events, "close")
	if f.fail == "close" {
		return f.injected
	}
	return nil
}

func TestPublicationErrorOrderingAndUncertainty(t *testing.T) {
	for _, c := range []struct {
		fail      string
		want      []string
		uncertain bool
	}{
		{"create", []string{"create"}, false},
		{"write", []string{"create", "write", "close"}, false},
		{"sync", []string{"create", "write", "sync", "close"}, false},
		{"close", []string{"create", "write", "sync", "close"}, false},
		{"rename", []string{"create", "write", "sync", "close", "rename"}, true},
		{"directory", []string{"create", "write", "sync", "close", "rename", "directory"}, true},
		{"success", []string{"create", "write", "sync", "close", "rename", "directory"}, false},
	} {
		t.Run(c.fail, func(t *testing.T) {
			var events []string
			injected := map[string]error{"create": os.ErrPermission, "write": syscall.ENOSPC, "sync": syscall.EIO, "close": syscall.EIO, "rename": syscall.EXDEV, "directory": syscall.EINVAL}[c.fail]
			file := &fakeFile{events: &events, fail: c.fail, injected: injected}
			final := []byte("old")
			ops := publicationOps{
				create: func() (syncFile, error) {
					events = append(events, "create")
					if c.fail == "create" {
						return nil, injected
					}
					return file, nil
				},
				rename: func() error {
					events = append(events, "rename")
					if c.fail == "rename" {
						return injected
					}
					final = bytes.Clone(file.data.Bytes())
					return nil
				},
				syncDir: func() error {
					events = append(events, "directory")
					if c.fail == "directory" {
						return injected
					}
					return nil
				},
			}
			err := publish([]byte("new"), ops)
			if (c.fail == "success") != (err == nil) || !reflect.DeepEqual(events, c.want) || errors.Is(err, ErrUncertain) != c.uncertain {
				t.Fatalf("error %v events %v", err, events)
			}
			if err != nil && !errors.Is(err, injected) {
				t.Fatal("cause lost")
			}
			want := "old"
			if c.fail == "directory" || c.fail == "success" {
				want = "new"
			}
			if string(final) != want {
				t.Fatal("wrong publication boundary")
			}
		})
	}
}

func TestWriteAndCloseErrorsBothPreserved(t *testing.T) {
	events := []string{}
	writeErr, closeErr := errors.New("write"), errors.New("close")
	f := &dualErrorFile{fakeFile: fakeFile{events: &events}, writeErr: writeErr, closeErr: closeErr}
	err := writeSyncClose(f, []byte("x"))
	if !errors.Is(err, writeErr) || !errors.Is(err, closeErr) || !reflect.DeepEqual(events, []string{"write", "close"}) {
		t.Fatal(err, events)
	}
}

type dualErrorFile struct {
	fakeFile
	writeErr, closeErr error
}

func (f *dualErrorFile) Write([]byte) (int, error) {
	*f.events = append(*f.events, "write")
	return 0, f.writeErr
}
func (f *dualErrorFile) Close() error { *f.events = append(*f.events, "close"); return f.closeErr }
