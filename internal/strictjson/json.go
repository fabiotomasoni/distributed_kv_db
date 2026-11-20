// Package strictjson validates bounded wire/configuration JSON, including facts
// encoding/json deliberately accepts: duplicate names and unpaired surrogates.
// It does not impose persistent-byte canonicalization; callers validate schemas.
package strictjson

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

var ErrInvalid = errors.New("strictjson: invalid JSON, schema or bound")

func Decode(raw []byte, limit int) (map[string]any, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) || !surrogates(raw) {
		return nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	v, err := read(d, 0)
	if err != nil {
		return nil, ErrInvalid
	}
	o, ok := v.(map[string]any)
	if !ok {
		return nil, ErrInvalid
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, ErrInvalid
	}
	return o, nil
}

func read(d *json.Decoder, depth int) (any, error) {
	if depth > 16 {
		return nil, ErrInvalid
	}
	t, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch t {
	case json.Delim('{'):
		if depth >= 16 {
			return nil, ErrInvalid
		}
		o := map[string]any{}
		for d.More() {
			t, err := d.Token()
			if err != nil {
				return nil, err
			}
			key, ok := t.(string)
			if !ok {
				return nil, ErrInvalid
			}
			if _, exists := o[key]; exists {
				return nil, ErrInvalid
			}
			v, err := read(d, depth+1)
			if err != nil {
				return nil, err
			}
			o[key] = v
		}
		end, err := d.Token()
		if err != nil || end != json.Delim('}') {
			return nil, ErrInvalid
		}
		return o, nil
	case json.Delim('['):
		if depth >= 16 {
			return nil, ErrInvalid
		}
		a := []any{}
		for d.More() {
			v, err := read(d, depth+1)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		end, err := d.Token()
		if err != nil || end != json.Delim(']') {
			return nil, ErrInvalid
		}
		return a, nil
	default:
		if _, bad := t.(json.Delim); bad {
			return nil, ErrInvalid
		}
		return t, nil
	}
}

func surrogates(raw []byte) bool {
	quoted := false
	for i := 0; i < len(raw); i++ {
		if raw[i] == '"' {
			quoted = !quoted
			continue
		}
		if !quoted || raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		n, err := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if n >= 0xdc00 && n <= 0xdfff {
			return false
		}
		if n >= 0xd800 && n <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			low, err := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if err != nil || low < 0xdc00 || low > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}

func Fields(v any, required, optional string) (map[string]any, error) {
	o, ok := v.(map[string]any)
	if !ok {
		return nil, ErrInvalid
	}
	names := strings.Fields(required + " " + optional)
	for k := range o {
		found := false
		for _, n := range names {
			if k == n {
				found = true
				break
			}
		}
		if !found {
			return nil, ErrInvalid
		}
	}
	for _, k := range strings.Fields(required) {
		if _, ok := o[k]; !ok {
			return nil, ErrInvalid
		}
	}
	return o, nil
}
func Uint(v any) (uint64, error) {
	s, ok := v.(string)
	if !ok || s == "" || len(s) > 20 || len(s) > 1 && s[0] == '0' {
		return 0, ErrInvalid
	}
	for _, b := range []byte(s) {
		if b < '0' || b > '9' {
			return 0, ErrInvalid
		}
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, ErrInvalid
	}
	return n, nil
}
func Integer(v any) (uint64, error) {
	n, ok := v.(json.Number)
	if !ok {
		return 0, ErrInvalid
	}
	return Uint(string(n))
}
func Hex(v any, n int) bool {
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
func Base64(v any, limit int) ([]byte, error) {
	s, ok := v.(string)
	if !ok || len(s) > base64.StdEncoding.EncodedLen(limit) {
		return nil, ErrInvalid
	}
	b, err := base64.StdEncoding.Strict().DecodeString(s)
	if err != nil || len(b) > limit || base64.StdEncoding.EncodeToString(b) != s {
		return nil, ErrInvalid
	}
	return b, nil
}
