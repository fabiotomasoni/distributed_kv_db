package config

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestSessionConfigurationV2CanonicalHashAndBounds(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/configuration-v2-sessions.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Configuration map[string]any
		Shared        string `json:"shared_preimage"`
		Hash          string `json:"config_hash"`
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	encode := func(m map[string]any) []byte { b, _ := json.Marshal(m); return b }
	c, e := Parse(encode(f.Configuration))
	if e != nil || !c.Sessions() || c.Hash() != f.Hash || string(c.SharedBytes()) != f.Shared {
		t.Fatal(c.Hash(), e)
	}
	for i := 1; i <= 3; i++ {
		b, e := os.ReadFile(fmt.Sprintf("../../examples/m11/n%d.json", i))
		if e != nil {
			t.Fatal(e)
		}
		example, e := Parse(b)
		if e != nil || !example.Sessions() || example.Hash() != f.Hash {
			t.Fatal("M11 example mismatch", i, e)
		}
	}
	legacy := map[string]any{}
	for k, v := range f.Configuration {
		legacy[k] = v
	}
	legacy["config_version"] = 1
	delete(legacy, "application_version")
	old, e := Parse(encode(legacy))
	if e != nil || old.Sessions() || old.Hash() == c.Hash() {
		t.Fatal("profiles share hash", e)
	}
	for _, v := range []any{nil, 1, 3, "2", 2.5} {
		m := map[string]any{}
		for k, x := range f.Configuration {
			m[k] = x
		}
		if v == nil {
			delete(m, "application_version")
		} else {
			m["application_version"] = v
		}
		if _, e = Parse(encode(m)); e == nil {
			t.Fatal("accepted app version", v)
		}
	}
	legacy["application_version"] = 2
	if _, e = Parse(encode(legacy)); e == nil {
		t.Fatal("v1 silently accepted session profile")
	}
}
