//go:build darwin

package storagefs

import (
	"errors"
	"io"
	"os"
	"syscall"
)

// OpenRegular reuses the private, no-follow file checks. create requires a new
// inode; existing files are opened without truncation. Caller owns Close and
// must hold the directory lock for every mutation.
func OpenRegular(path string, create bool) (*os.File, error) {
	flags := syscall.O_RDWR
	if create {
		flags |= syscall.O_CREAT | syscall.O_EXCL
	}
	return openPrivate(path, flags)
}

// ReadPrivate bounds small control-file reads; WAL input is streamed instead.
func ReadPrivate(path string, limit int64) ([]byte, error) {
	if limit < 1 || limit > 8*1024*1024 {
		return nil, ErrUnsafe
	}
	f, err := openPrivate(path, syscall.O_RDONLY)
	if err != nil {
		return nil, err
	}
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	err = errors.Join(err, f.Close())
	if int64(len(b)) > limit {
		return nil, errors.Join(ErrUnsafe, err)
	}
	return b, err
}
