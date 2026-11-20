package wal

import (
	"encoding/binary"
	"errors"
	"io"
	"math"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

// Recovery reports a validated prefix and an optional incomplete physical tail.
// A non-nil error returns no usable partial state; Replay never mutates a file.
type Recovery struct {
	State                 State
	ValidBytes, TailBytes int64
}

func Replay(r io.Reader, baseIndex, baseTerm uint64) (Recovery, error) {
	if (baseIndex == 0) != (baseTerm == 0) {
		return Recovery{}, ErrCorrupt
	}
	s := State{BaseIndex: baseIndex, BaseTerm: baseTerm, CommitIndex: baseIndex}
	offset := int64(0)
	for {
		var header [24]byte
		n, err := io.ReadFull(r, header[:])
		if int64(n) > MaxWALBytes-offset {
			return Recovery{}, ErrLimit
		}
		if err == io.EOF && n == 0 {
			if s.Sequence == 0 {
				return Recovery{}, errors.Join(ErrCorrupt, ErrIncomplete)
			}
			return Recovery{State: s, ValidBytes: offset}, nil
		}
		if err != nil && err != io.ErrUnexpectedEOF {
			return Recovery{}, err
		}
		size, herr := storageformat.FrameSize(header[:n])
		// A fully available sequence already contradicts a torn-write origin
		// when it is not the next one, even if CRC/payload bytes are missing.
		if n >= 20 && (herr == nil || errors.Is(herr, storageformat.ErrIncomplete)) {
			if s.Sequence == math.MaxUint64 || binary.BigEndian.Uint64(header[12:20]) != s.Sequence+1 {
				return Recovery{}, errors.Join(ErrCorrupt, ErrTransition)
			}
		}
		if herr != nil {
			if errors.Is(herr, storageformat.ErrIncomplete) && err == io.ErrUnexpectedEOF && s.Sequence > 0 {
				return Recovery{State: s, ValidBytes: offset, TailBytes: int64(n)}, nil
			}
			return Recovery{}, errors.Join(ErrCorrupt, herr)
		}
		if int64(size) > MaxWALBytes-offset {
			return Recovery{}, ErrLimit
		}
		frame := make([]byte, size)
		copy(frame, header[:])
		read, err := io.ReadFull(r, frame[24:])
		if err != nil {
			if (err == io.EOF || err == io.ErrUnexpectedEOF) && s.Sequence > 0 {
				return Recovery{State: s, ValidBytes: offset, TailBytes: int64(24 + read)}, nil
			}
			return Recovery{}, errors.Join(ErrCorrupt, err)
		}
		f, _, err := storageformat.DecodeFrame(frame)
		if err != nil {
			return Recovery{}, errors.Join(ErrCorrupt, err)
		}
		tx, err := storageformat.DecodeTransaction(f.Payload)
		if err != nil {
			return Recovery{}, errors.Join(ErrCorrupt, err)
		}
		next, err := s.next(tx, f.Sequence)
		if err != nil {
			return Recovery{}, errors.Join(ErrCorrupt, err)
		}
		s = next
		offset += int64(size)
	}
}
