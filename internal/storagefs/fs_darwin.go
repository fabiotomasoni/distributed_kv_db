//go:build darwin

package storagefs

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
)

// Volume identifies the actual mount seen by Statfs, not physical durability.
type Volume struct{ Filesystem, Mount, Device string }

func cString(v []int8) string {
	b := make([]byte, 0, len(v))
	for _, c := range v {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

func Inspect(path string) (Volume, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return Volume{}, err
	}
	return Volume{cString(st.Fstypename[:]), cString(st.Mntonname[:]), cString(st.Mntfromname[:])}, nil
}

// Lock owns a stable LOCK inode. Close and PublishCurrent are serialized; do not
// copy a Lock. Close never unlinks LOCK and is safe to repeat.
type Lock struct {
	mu      sync.Mutex
	file    *os.File
	root    string
	failure error
}

func privateDirectory(path string) error {
	s, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !s.IsDir() || s.Mode()&os.ModeSymlink != 0 || s.Mode().Perm()&0077 != 0 {
		return ErrUnsafe
	}
	v, err := Inspect(path)
	if err != nil {
		return err
	}
	if v.Filesystem != "apfs" {
		return ErrUnsupported
	}
	return nil
}

func regular(f *os.File) error {
	s, err := f.Stat()
	if err != nil {
		return err
	}
	if !s.Mode().IsRegular() || s.Mode().Perm()&0077 != 0 {
		return ErrUnsafe
	}
	if st, ok := s.Sys().(*syscall.Stat_t); !ok || st.Nlink != 1 {
		return ErrUnsafe
	}
	return nil
}

func openPrivate(path string, flags int) (*os.File, error) {
	// Reject existing special files before opening them; fstat below rechecks
	// the resulting descriptor. O_NOFOLLOW still rejects a raced symlink.
	if s, err := os.Lstat(path); err == nil {
		if !s.Mode().IsRegular() {
			return nil, ErrUnsafe
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	fd, err := syscall.Open(path, flags|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	if err = regular(f); err != nil {
		return nil, errors.Join(err, f.Close())
	}
	return f, nil
}

func interrupted(fn func() error) error {
	for {
		err := fn()
		if !errors.Is(err, syscall.EINTR) {
			return err
		}
	}
}

func Acquire(path string) (*Lock, error) {
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = privateDirectory(root); err != nil {
		return nil, err
	}
	f, err := openPrivate(filepath.Join(root, "LOCK"), syscall.O_RDWR|syscall.O_CREAT)
	if err != nil {
		return nil, err
	}
	err = interrupted(func() error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB) })
	if err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			err = errors.Join(ErrLocked, err)
		}
		return nil, errors.Join(err, f.Close())
	}
	return &Lock{file: f, root: root}, nil
}

func (l *Lock) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	f := l.file
	l.file = nil
	return f.Close()
}

// SyncDirectory uses plain directory fsync, not regular-file F_FULLFSYNC. Every
// error including close is surfaced. Unsupported barriers must block publication.
func SyncDirectory(path string) error {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(fd), path)
	return syncDirectoryFile(fd, f.Close, syscall.Fsync)
}

func syncDirectoryFile(fd int, closeFile func() error, syncFile func(int) error) error {
	err := interrupted(func() error { return syncFile(fd) })
	return errors.Join(err, closeFile())
}

func (l *Lock) ops() publicationOps {
	return publicationOps{
		create: func() (syncFile, error) {
			return openPrivate(filepath.Join(l.root, "CURRENT.tmp"), syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL)
		},
		rename:  func() error { return os.Rename(filepath.Join(l.root, "CURRENT.tmp"), filepath.Join(l.root, "CURRENT")) },
		syncDir: func() error { return SyncDirectory(l.root) },
	}
}

// PublishCurrent validates <=4 KiB of immutable bytes before touching files.
// validate must be finite, non-reentrant and must not mutate the input.
// A failed I/O poisons this owner;
// staging is retained for explicit recovery, never removed automatically.
// Success proves completed OS operations, not power-loss survival or that the
// caller correctly published the referenced generation beforehand.
func (l *Lock) PublishCurrent(data []byte, validate func([]byte) error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return ErrClosed
	}
	if l.failure != nil {
		return errors.Join(ErrFailed, l.failure)
	}
	if len(data) == 0 || len(data) > 4096 || validate == nil {
		return ErrUnsafe
	}
	if err := validate(data); err != nil {
		return err
	}
	if err := privateDirectory(l.root); err != nil {
		l.failure = err
		return err
	}
	if s, err := os.Lstat(filepath.Join(l.root, "CURRENT")); err == nil {
		if !s.Mode().IsRegular() || s.Mode().Perm()&0077 != 0 {
			l.failure = ErrUnsafe
			return ErrUnsafe
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		l.failure = err
		return err
	}
	err := publish(data, l.ops())
	if err != nil {
		l.failure = err
	}
	return err
}
