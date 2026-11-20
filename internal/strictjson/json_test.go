package strictjson

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStrictDecode(t *testing.T) {
	for _, raw := range []string{`{"a":1,"\u0061":2}`, `{"x":{"a":0,"a":1}}`, `{"a":"\ud800"}`, `{"a":"\udc00"}`, `{"a":"\ud800\u0041"}`, `{"a":1} {}`, `[]`, `{"a":"` + string([]byte{255}) + `"}`, `{"a":` + strings.Repeat(`[`, 17) + `0` + strings.Repeat(`]`, 17) + `}`, `{"a":`} {
		if _, err := Decode([]byte(raw), 4096); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	for _, raw := range []string{`{"a":"\ud83d\ude00"}`, ` {"a":"\\uD800","b":[null,true,1]} `, `{"a":"é"}`} {
		if _, err := Decode([]byte(raw), 4096); err != nil {
			t.Fatalf("valid %q: %v", raw, err)
		}
	}
	if _, err := Decode([]byte(`{}`), 1); err == nil {
		t.Fatal("size")
	}
}
func TestScalarAndSchemaBoundaries(t *testing.T) {
	for _, v := range []any{"", "01", "-1", "+1", "1.0", "1e1", "18446744073709551616", json.Number("1"), nil} {
		if _, err := Uint(v); err == nil {
			t.Fatalf("uint accepted %v", v)
		}
	}
	if n, err := Uint("18446744073709551615"); err != nil || n != ^uint64(0) {
		t.Fatal(n, err)
	}
	for _, v := range []any{json.Number("1e1"), json.Number("1.0"), "1"} {
		if _, err := Integer(v); err == nil {
			t.Fatal(v)
		}
	}
	for _, s := range []string{"AA==\n", "AB==", "AA", "!!!!"} {
		if _, err := Base64(s, 1); err == nil {
			t.Fatal(s)
		}
	}
	if _, err := Base64("AA==", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := Fields(map[string]any{"A": 1}, "a", ""); err == nil {
		t.Fatal("case")
	}
	if _, err := Fields(map[string]any{}, "a", ""); err == nil {
		t.Fatal("missing")
	}
	if Hex(strings.Repeat("A", 32), 32) || Hex(nil, 32) || !Hex(strings.Repeat("a", 32), 32) {
		t.Fatal("hex")
	}
}
