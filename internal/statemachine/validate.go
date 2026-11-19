package statemachine

// valid checks the whole command before any state-dependent operation. In
// particular, an invalid replacement is not disguised as a failed comparison.
func (c Command) valid() bool {
	if !validKey(c.Key) {
		return false
	}
	switch c.Operation {
	case Put:
		return c.Expected == nil && len(c.Value) <= MaxValueBytes
	case Get, Delete:
		return c.Value == nil && c.Expected == nil
	case CAS:
		if c.Expected == nil || len(c.Value) > MaxValueBytes {
			return false
		}
		if !c.Expected.Exists {
			return c.Expected.Value == nil
		}
		return len(c.Expected.Value) <= MaxValueBytes
	default:
		return false
	}
}

func validKey(key string) bool {
	if len(key) == 0 || len(key) > MaxKeyBytes || !alphaNumeric(key[0]) {
		return false
	}
	for i := 1; i < len(key); i++ {
		b := key[i]
		if !alphaNumeric(b) && b != '.' && b != '_' && b != ':' && b != '/' && b != '-' {
			return false
		}
	}
	return true
}

func alphaNumeric(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
