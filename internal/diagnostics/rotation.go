package diagnostics

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

const MaxLogBytes int64 = 256 * 1024

// Rotation owns only four fixed files in an explicitly supplied private folder.
// It never removes arbitrary paths and never shares the active data directory.
type Rotation struct {
	dir  string
	file *os.File
	size int64
}

func OpenRotation(dir string) (*Rotation, error) {
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() || st.Mode().Perm() != 0700 {
		return nil, errors.New("diagnostics: private log directory required")
	}
	r := &Rotation{dir: dir}
	for i := 0; i < 4; i++ {
		p := r.path(i)
		v, e := os.Lstat(p)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || !v.Mode().IsRegular() || v.Mode().Perm() != 0600 || v.Size() > MaxLogBytes {
			return nil, errors.New("diagnostics: invalid log file")
		}
	}
	f, e := os.OpenFile(r.path(0), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if e != nil {
		return nil, e
	}
	r.file = f
	v, e := f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	r.size = v.Size()
	return r, nil
}
func (r *Rotation) path(i int) string {
	return filepath.Join(r.dir, []string{"quorumkv.jsonl", "quorumkv.1.jsonl", "quorumkv.2.jsonl", "quorumkv.3.jsonl"}[i])
}
func (r *Rotation) Write(p []byte) (int, error) {
	if r.file == nil {
		return 0, os.ErrClosed
	}
	if len(p) > 1024 {
		return 0, errors.New("diagnostics: record too large")
	}
	if r.size+int64(len(p)) > MaxLogBytes {
		if e := r.file.Close(); e != nil {
			return 0, e
		}
		r.file = nil
		for i := 3; i > 0; i-- {
			if e := os.Rename(r.path(i-1), r.path(i)); e != nil && !os.IsNotExist(e) {
				return 0, e
			}
		}
		f, e := os.OpenFile(r.path(0), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
		if e != nil {
			return 0, e
		}
		r.file = f
		r.size = 0
	}
	if r.file == nil {
		return 0, os.ErrClosed
	}
	n, e := r.file.Write(p)
	r.size += int64(n)
	if n != len(p) && e == nil {
		e = io.ErrShortWrite
	}
	return n, e
}
func (r *Rotation) Close() error {
	if r.file == nil {
		return nil
	}
	e := r.file.Close()
	r.file = nil
	return e
}
