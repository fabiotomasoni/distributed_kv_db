package storageformat

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"math"
)

var castagnoli = crc32.MakeTable(crc32.Castagnoli)

// Frame is one structurally valid transaction. Sequence continuity and all
// pre-state-dependent invariants still require the future WAL owner.
type Frame struct {
	Sequence uint64
	Payload  []byte
}

func transaction(raw []byte) error {
	o, err := object(raw, MaxPayload)
	if err != nil {
		return err
	}
	o, ok := fields(o, "", "hard_state replace_suffix commit_index")
	if !ok || len(o) == 0 {
		return ErrMalformed
	}
	var currentTerm uint64
	hasTerm := false
	if h, has := o["hard_state"]; has {
		hs, ok := fields(h, "current_term voted_for", "")
		if !ok {
			return ErrMalformed
		}
		currentTerm, ok = uintString(hs["current_term"])
		if !ok {
			return ErrMalformed
		}
		hasTerm = true
		v := hs["voted_for"]
		if v != nil && (!member(v) || currentTerm == 0) {
			return ErrMalformed
		}
	}
	if c, has := o["commit_index"]; has {
		if _, ok := uintString(c); !ok {
			return ErrMalformed
		}
	}
	if r, has := o["replace_suffix"]; has {
		rs, ok := fields(r, "from_index entries", "")
		if !ok {
			return ErrMalformed
		}
		from, ok := uintString(rs["from_index"])
		if !ok || from == 0 {
			return ErrMalformed
		}
		entries, ok := rs["entries"].([]any)
		if !ok || len(entries) > 16 {
			return ErrMalformed
		}
		var lastTerm uint64
		size := 0
		for j, e := range entries {
			if !entry(e) {
				return ErrMalformed
			}
			b, _ := json.Marshal(e)
			size += len(b)
			if len(b) > MaxEntry || size > 262144 {
				return ErrMalformed
			}
			x := e.(map[string]any)
			i, _ := uintString(x["index"])
			t, _ := uintString(x["term"])
			if uint64(j) > math.MaxUint64-from || i != from+uint64(j) || t < lastTerm || hasTerm && t > currentTerm {
				return ErrMalformed
			}
			lastTerm = t
		}
	}
	return nil
}

func checksum(header, payload []byte) uint32 {
	c := crc32.Update(0, castagnoli, header)
	return crc32.Update(c, castagnoli, payload)
}

// EncodeFrame accepts an already canonical transaction; it never normalizes an
// ambiguous payload. The returned frame is independent of the caller's buffer.
func EncodeFrame(sequence uint64, payload []byte) ([]byte, error) {
	if sequence == 0 {
		return nil, ErrMalformed
	}
	if err := transaction(payload); err != nil {
		return nil, err
	}
	b := make([]byte, 24+len(payload))
	copy(b, "QWAL")
	binary.BigEndian.PutUint16(b[4:], 1)
	binary.BigEndian.PutUint16(b[6:], 1)
	binary.BigEndian.PutUint32(b[8:], uint32(len(payload)))
	binary.BigEndian.PutUint64(b[12:], sequence)
	copy(b[24:], payload)
	binary.BigEndian.PutUint32(b[20:], checksum(b[:20], payload))
	return b, nil
}

// DecodeFrame consumes one frame without I/O. ErrIncomplete is not permission to
// truncate a file: only a future recovery owner can establish a valid final tail.
func DecodeFrame(raw []byte) (Frame, int, error) {
	bad := func(err error) (Frame, int, error) { return Frame{}, 0, err }
	if len(raw) < 24 {
		return bad(ErrIncomplete)
	}
	if !bytes.Equal(raw[:4], []byte("QWAL")) {
		return bad(ErrMalformed)
	}
	if binary.BigEndian.Uint16(raw[4:]) != 1 || binary.BigEndian.Uint16(raw[6:]) != 1 {
		return bad(ErrVersion)
	}
	n := binary.BigEndian.Uint32(raw[8:])
	seq := binary.BigEndian.Uint64(raw[12:])
	if n == 0 || n > MaxPayload || seq == 0 {
		return bad(ErrMalformed)
	}
	end := 24 + int(n)
	if len(raw) < end {
		return bad(ErrIncomplete)
	}
	p := raw[24:end]
	if checksum(raw[:20], p) != binary.BigEndian.Uint32(raw[20:]) {
		return bad(ErrChecksum)
	}
	if err := transaction(p); err != nil {
		return bad(err)
	}
	return Frame{seq, bytes.Clone(p)}, end, nil
}
