package storagefs

import (
	"errors"
	"io"
)

var (
	ErrLocked      = errors.New("storagefs: directory locked")
	ErrUnsafe      = errors.New("storagefs: unsafe path, type, permissions or input")
	ErrUnsupported = errors.New("storagefs: unsupported filesystem")
	ErrClosed      = errors.New("storagefs: owner closed")
	ErrFailed      = errors.New("storagefs: owner failed; explicit restart required")
	ErrUncertain   = errors.New("storagefs: publication outcome uncertain")
)

// WriteAll continues only a successful short write. A partial write with an
// error is terminal; its prefix must never be blindly appended a second time.
func WriteAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if n < 0 || n > len(data) {
			return errors.Join(io.ErrShortWrite, err)
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
		data = data[n:]
	}
	return nil
}

type syncFile interface {
	io.Writer
	Sync() error
	Close() error
}

func writeSyncClose(f syncFile, data []byte) error {
	err := WriteAll(f, data)
	if err == nil {
		err = f.Sync()
	}
	return errors.Join(err, f.Close())
}

type publicationOps struct {
	create  func() (syncFile, error)
	rename  func() error
	syncDir func() error
}

// publish preserves staging on error. Once rename is attempted, failure is
// conservatively uncertain; no rollback or cleanup may destroy either candidate.
func publish(data []byte, ops publicationOps) error {
	f, err := ops.create()
	if err != nil {
		return err
	}
	if err = writeSyncClose(f, data); err != nil {
		return err
	}
	if err = ops.rename(); err != nil {
		return errors.Join(ErrUncertain, err)
	}
	if err = ops.syncDir(); err != nil {
		return errors.Join(ErrUncertain, err)
	}
	return nil
}
