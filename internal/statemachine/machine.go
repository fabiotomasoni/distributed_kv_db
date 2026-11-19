package statemachine

import "bytes"

// Machine owns the map and its exact sum of key and value lengths. Its zero
// value is ready to use. A Machine must not be copied after its first use.
type Machine struct {
	values       map[string][]byte
	logicalBytes uint64
}

// New returns an empty, independent machine with the approved fixed limits.
func New() *Machine {
	return &Machine{}
}

// Apply executes one complete, validated command synchronously. CAS comparison
// and replacement belong to this single transition. Calls must be serialized by
// the owner; this method provides no concurrent-access guarantee.
func (m *Machine) Apply(c Command) Result {
	if !c.valid() {
		return Result{Code: InvalidArgument}
	}

	value, exists := m.values[c.Key]
	switch c.Operation {
	case Get:
		if !exists {
			return Result{Code: KeyNotFound}
		}
		return Result{Code: OK, Exists: true, Value: copyBytes(value)}
	case Delete:
		if exists {
			delete(m.values, c.Key)
			m.logicalBytes -= uint64(len(c.Key) + len(value))
		}
		return Result{Code: OK, Deleted: exists}
	case CAS:
		if c.Expected.Exists != exists || exists && !bytes.Equal(c.Expected.Value, value) {
			return Result{Code: CompareFailed}
		}
	}
	// The only remaining validated operations are Put and a matching CAS.
	return m.put(c.Key, c.Value, value, exists)
}

func (m *Machine) put(key string, value, old []byte, exists bool) Result {
	nextBytes := m.logicalBytes - uint64(len(old)) + uint64(len(value))
	if !exists {
		if len(m.values) >= MaxKeys {
			return Result{Code: ResourceLimit}
		}
		nextBytes += uint64(len(key))
	}
	if nextBytes > MaxLogicalBytes {
		return Result{Code: ResourceLimit}
	}

	// Compute all rejection conditions before mutating either field. Copy even
	// an empty value so a successful Get consistently returns an owned slice.
	stored := copyBytes(value)
	if m.values == nil {
		m.values = make(map[string][]byte)
	}
	m.values[key] = stored
	m.logicalBytes = nextBytes
	return Result{Code: OK, Applied: true}
}

func copyBytes(value []byte) []byte {
	result := make([]byte, len(value))
	copy(result, value)
	return result
}
