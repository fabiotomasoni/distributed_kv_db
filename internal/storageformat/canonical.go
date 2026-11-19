package storageformat

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

var (
	ErrMalformed  = errors.New("storageformat: malformed schema or bounds")
	ErrCanonical  = errors.New("storageformat: noncanonical JSON")
	ErrVersion    = errors.New("storageformat: unsupported version or kind")
	ErrChecksum   = errors.New("storageformat: checksum mismatch")
	ErrIncomplete = errors.New("storageformat: incomplete frame")
)

const (
	MaxPayload  = 384 * 1024
	MaxEntry    = 48 * 1024
	MaxManifest = 4096
)

// object validates canonical bytes before any schema interpretation. Comparing
// re-encoding also rejects duplicate names, alternate escapes and trailing data.
func object(raw []byte, limit int) (map[string]any, error) {
	if len(raw) == 0 || len(raw) > limit {
		return nil, ErrMalformed
	}
	depth, quoted, escaped := 0, false, false
	for _, b := range raw {
		if b >= 128 {
			return nil, ErrCanonical
		}
		if quoted {
			if escaped {
				escaped = false
			} else if b == '\\' {
				escaped = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		switch b {
		case '"':
			quoted = true
		case '{', '[':
			depth++
			if depth > 16 {
				return nil, ErrMalformed
			}
		case '}', ']':
			depth--
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		return nil, ErrMalformed
	}
	o, ok := v.(map[string]any)
	if !ok {
		return nil, ErrMalformed
	}
	b, err := json.Marshal(o)
	if err != nil || !bytes.Equal(raw, b) {
		return nil, ErrCanonical
	}
	return o, nil
}

func fields(v any, required, optional string) (map[string]any, bool) {
	o, ok := v.(map[string]any)
	if !ok {
		return nil, false
	}
	allowed := strings.Fields(required + " " + optional)
	for key := range o {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	for _, key := range strings.Fields(required) {
		if _, ok := o[key]; !ok {
			return nil, false
		}
	}
	return o, true
}

func uintString(v any) (uint64, bool) {
	s, ok := v.(string)
	if !ok || s == "" || len(s) > 20 || len(s) > 1 && s[0] == '0' {
		return 0, false
	}
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

func hexString(v any, n int) bool {
	s, ok := v.(string)
	if !ok || len(s) != n {
		return false
	}
	for _, b := range []byte(s) {
		if !(b >= '0' && b <= '9' || b >= 'a' && b <= 'f') {
			return false
		}
	}
	return true
}

func value(v any) ([]byte, bool) {
	s, ok := v.(string)
	if !ok || len(s) > 21848 {
		return nil, false
	}
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	return b, err == nil && len(b) <= 16384 && base64.StdEncoding.EncodeToString(b) == s
}

func validKey(v any) bool {
	s, ok := v.(string)
	if !ok || len(s) < 1 || len(s) > 256 {
		return false
	}
	for i, b := range []byte(s) {
		alnum := b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9'
		if !alnum && (i == 0 || !strings.ContainsRune("._:/-", rune(b))) {
			return false
		}
	}
	return true
}

func member(v any) bool { return v == "n1" || v == "n2" || v == "n3" }
