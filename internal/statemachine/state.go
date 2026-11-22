package statemachine

import (
	"errors"
	"sort"
)

type Record struct {
	Key   string
	Value []byte
}
type StateImage struct {
	Records      []Record
	LogicalBytes uint64
}

var ErrStateImage = errors.New("statemachine: invalid isolated state image")

// CopyState is owner-only, like Apply. A parked M02 worker may call it; it is
// not a concurrent map-read API. Output is sorted and owns every value buffer.
func (m *Machine) CopyState() StateImage {
	out := StateImage{Records: make([]Record, 0, len(m.values)), LogicalBytes: m.logicalBytes}
	for k, v := range m.values {
		out.Records = append(out.Records, Record{k, copyBytes(v)})
	}
	sort.Slice(out.Records, func(i, j int) bool { return out.Records[i].Key < out.Records[j].Key })
	return out
}

// FromState constructs a NEW isolated machine, never replacing a live owner.
func FromState(s StateImage) (*Machine, error) {
	if len(s.Records) > MaxKeys || s.LogicalBytes > MaxLogicalBytes {
		return nil, ErrStateImage
	}
	var sum uint64
	for i, r := range s.Records {
		if !validKey(r.Key) || len(r.Value) > MaxValueBytes || i > 0 && s.Records[i-1].Key >= r.Key {
			return nil, ErrStateImage
		}
		sum += uint64(len(r.Key) + len(r.Value))
		if sum > MaxLogicalBytes {
			return nil, ErrStateImage
		}
	}
	if sum != s.LogicalBytes {
		return nil, ErrStateImage
	}
	m := New()
	m.values = make(map[string][]byte, len(s.Records))
	m.logicalBytes = sum
	for _, r := range s.Records {
		m.values[r.Key] = copyBytes(r.Value)
	}
	return m, nil
}
