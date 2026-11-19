package storageformat

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

type fixtures struct {
	WAL []struct {
		ID       string
		Payload  string `json:"payload_json"`
		Sequence string
		Hex      string `json:"frame_hex"`
	}
	Commands []struct {
		ID     string
		Entry  string `json:"entry_json"`
		Digest string `json:"command_sha256"`
	}
	Current  string `json:"current_json"`
	Identity string `json:"identity_json"`
}

func vectors(t *testing.T) fixtures {
	t.Helper()
	b, err := os.ReadFile("../../docs/fixtures/canonical-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtures
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestGoldenFrames(t *testing.T) {
	for i, v := range vectors(t).WAL {
		t.Run(v.ID, func(t *testing.T) {
			want, err := hex.DecodeString(v.Hex)
			if err != nil {
				t.Fatal(err)
			}
			got, err := EncodeFrame(uint64(i+1), []byte(v.Payload))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("golden encode: %v", err)
			}
			f, n, err := DecodeFrame(append(bytes.Clone(got), got...))
			if err != nil || n != len(got) || f.Sequence != uint64(i+1) || string(f.Payload) != v.Payload {
				t.Fatalf("decode: %+v %d %v", f, n, err)
			}
			again, err := EncodeFrame(f.Sequence, f.Payload)
			if err != nil || !bytes.Equal(again, want) {
				t.Fatal("round trip differs")
			}
			got[24] ^= 1
			if string(f.Payload) != v.Payload {
				t.Fatal("decode aliases caller bytes")
			}
		})
	}
}

func TestGoldenEntriesAndManifests(t *testing.T) {
	f := vectors(t)
	for _, v := range f.Commands {
		t.Run(v.ID, func(t *testing.T) {
			if err := ValidateEntry([]byte(v.Entry)); err != nil {
				t.Fatal(err)
			}
			h, err := CommandDigest([]byte(v.Entry))
			if err != nil || hex.EncodeToString(h[:]) != v.Digest {
				t.Fatalf("digest differs: %v", err)
			}
		})
	}
	for kind, text := range map[string]string{"IDENTITY": f.Identity, "CURRENT": f.Current} {
		t.Run(kind, func(t *testing.T) {
			if err := ValidateManifest(kind, []byte(text)); err != nil {
				t.Fatal(err)
			}
			var o map[string]any
			if err := json.Unmarshal([]byte(text), &o); err != nil {
				t.Fatal(err)
			}
			delete(o, "checksum")
			data, _ := json.Marshal(o)
			got, err := SealManifest(kind, data)
			if err != nil || string(got) != text {
				t.Fatalf("manifest bytes: %v", err)
			}
			var sealed map[string]any
			json.Unmarshal(got, &sealed)
			sealed["checksum"] = "0000000000000000000000000000000000000000000000000000000000000000"
			bad, _ := json.Marshal(sealed)
			if err = ValidateManifest(kind, bad); !errors.Is(err, ErrChecksum) {
				t.Fatal(err)
			}
		})
	}
}

func TestFrameBoundariesAndCorruption(t *testing.T) {
	want, _ := hex.DecodeString(vectors(t).WAL[0].Hex)
	for n := 0; n < len(want); n++ {
		if _, used, err := DecodeFrame(want[:n]); !errors.Is(err, ErrIncomplete) || used != 0 {
			t.Fatalf("cut %d: %v", n, err)
		}
	}
	for _, c := range []struct {
		name   string
		mutate func([]byte)
		err    error
	}{
		{"magic", func(b []byte) { b[0] = 'X' }, ErrMalformed},
		{"version", func(b []byte) { b[5] = 2 }, ErrVersion},
		{"kind", func(b []byte) { b[7] = 2 }, ErrVersion},
		{"zero-length", func(b []byte) { binary.BigEndian.PutUint32(b[8:], 0) }, ErrMalformed},
		{"oversize", func(b []byte) { binary.BigEndian.PutUint32(b[8:], MaxPayload+1) }, ErrMalformed},
		{"sequence-zero", func(b []byte) { binary.BigEndian.PutUint64(b[12:], 0) }, ErrMalformed},
		{"crc", func(b []byte) { b[20] ^= 1 }, ErrChecksum},
		{"payload", func(b []byte) { b[len(b)-1] ^= 1 }, ErrChecksum},
	} {
		t.Run(c.name, func(t *testing.T) {
			b := bytes.Clone(want)
			c.mutate(b)
			if _, n, err := DecodeFrame(b); n != 0 || !errors.Is(err, c.err) {
				t.Fatal(err)
			}
		})
	}
	if _, err := EncodeFrame(0, []byte(`{"commit_index":"0"}`)); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
}

func TestStrictTransactions(t *testing.T) {
	for _, raw := range []string{
		`{}`, ` {"commit_index":"0"}`, `{"commit_index":"0"}\n`,
		`{"commit_index":"0","commit_index":"0"}`, `{"commit_index":"0","extra":1}`,
		`{"":1,"commit_index":"0"}`, `{"hard_state replace_suffix":1}`,
		`{"commit_index":null}`, `{"commit_index":0}`, `{"commit_index":"01"}`, `{"commit_index":"18446744073709551616"}`,
		`{"hard_state":{"current_term":"0","voted_for":"n1"}}`,
		`{"hard_state":{"current_term":"1","voted_for":"n4"}}`,
		`{"hard_state":{"current_term":"1"}}`,
		`{"replace_suffix":{"entries":null,"from_index":"1"}}`,
		`{"replace_suffix":{"entries":[],"from_index":"0"}}`,
		`{"commit_index":"0"}{"commit_index":"0"}`,
	} {
		if _, err := EncodeFrame(1, []byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if _, err := EncodeFrame(1, []byte(`{"hard_state":{"current_term":"1","voted_for":null},"replace_suffix":{"entries":[],"from_index":"1"}}`)); err != nil {
		t.Fatal(err)
	}
	// A valid checksum must not bypass schema validation in the decoder.
	p := []byte(`{"extra":1}`)
	b := make([]byte, 24+len(p))
	copy(b, "QWAL")
	binary.BigEndian.PutUint16(b[4:], 1)
	binary.BigEndian.PutUint16(b[6:], 1)
	binary.BigEndian.PutUint32(b[8:], uint32(len(p)))
	binary.BigEndian.PutUint64(b[12:], 1)
	copy(b[24:], p)
	binary.BigEndian.PutUint32(b[20:], checksum(b[:20], p))
	if _, _, err := DecodeFrame(b); !errors.Is(err, ErrMalformed) {
		t.Fatal(err)
	}
}

func TestEntryRejections(t *testing.T) {
	for _, raw := range []string{
		`{"command":null,"index":"1","kind":"noop","term":"1"}`,
		`{"index":"0","kind":"noop","term":"1"}`,
		`{"index":"1","kind":"noop","term":"0"}`,
		`{"index":"1","kind":"get","term":"1"}`,
		`{"command":{"key":"k","value_b64":""},"index":"1","kind":"read","term":"1"}`,
		`{"command":{"key":"../bad"},"index":"1","kind":"read","term":"1"}`,
		`{"command":{"key":"\u006b"},"index":"1","kind":"read","term":"1"}`,
	} {
		if err := ValidateEntry([]byte(raw)); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	base := vectors(t).Commands[0].Entry
	for _, change := range []func(map[string]any){
		func(c map[string]any) { c["sequence"] = 0 }, func(c map[string]any) { c["sequence"] = 1.5 },
		func(c map[string]any) { c["sequence"] = 9007199254740992 }, func(c map[string]any) { c["value_b64"] = nil },
		func(c map[string]any) { c["value_b64"] = "eB==" }, func(c map[string]any) { c["client_id"] = "BAD" },
	} {
		var o map[string]any
		json.Unmarshal([]byte(base), &o)
		change(o["command"].(map[string]any))
		b, _ := json.Marshal(o)
		if err := ValidateEntry(b); err == nil {
			t.Fatal(string(b))
		}
	}
}

func TestSizeAndNestedTransactionConstraints(t *testing.T) {
	var e map[string]any
	json.Unmarshal([]byte(vectors(t).Commands[3].Entry), &e)
	e["index"], e["term"] = "18446744073709551615", "18446744073709551615"
	c := e["command"].(map[string]any)
	c["key"] = strings.Repeat("k", 256)
	c["sequence"] = json.Number("9007199254740991")
	v := base64.StdEncoding.EncodeToString(make([]byte, 16384))
	c["new_value_b64"] = v
	c["expected"].(map[string]any)["value_b64"] = v
	b, _ := json.Marshal(e)
	if len(b) != 44184 || ValidateEntry(b) != nil {
		t.Fatal("maximum CAS cannot be represented", len(b))
	}
	c["new_value_b64"] = base64.StdEncoding.EncodeToString(make([]byte, 16385))
	b, _ = json.Marshal(e)
	if ValidateEntry(b) == nil {
		t.Fatal("oversized value accepted")
	}
	for _, raw := range []string{
		`{"replace_suffix":{"entries":[{"index":"2","kind":"noop","term":"1"}],"from_index":"1"}}`,
		`{"hard_state":{"current_term":"1","voted_for":null},"replace_suffix":{"entries":[{"index":"1","kind":"noop","term":"2"}],"from_index":"1"}}`,
		`{"replace_suffix":{"entries":[{"index":"1","kind":"noop","term":"2"},{"index":"2","kind":"noop","term":"1"}],"from_index":"1"}}`,
		`{"commit_index":` + strings.Repeat("[", 17) + `"0"` + strings.Repeat("]", 17) + `}`,
	} {
		if _, err := EncodeFrame(1, []byte(raw)); err == nil {
			t.Fatal("invalid transaction accepted", raw)
		}
	}
	entries := make([]any, 17)
	for i := range entries {
		entries[i] = map[string]any{"index": fmt.Sprint(i + 1), "term": "1", "kind": "noop"}
	}
	b, _ = json.Marshal(map[string]any{"replace_suffix": map[string]any{"entries": entries, "from_index": "1"}})
	if _, err := EncodeFrame(1, b); err == nil {
		t.Fatal("batch above 16 accepted")
	}
}

func TestManifestSchemaRejections(t *testing.T) {
	for _, change := range []func(map[string]any){
		func(o map[string]any) { o["generation"] = "../gen-1" }, func(o map[string]any) { o["generation"] = "gen-01" },
		func(o map[string]any) { o["base_term"] = "1" }, func(o map[string]any) { o["format_version"] = 2 },
		func(o map[string]any) { o["snapshot_sha256"] = nil }, func(o map[string]any) { o["extra"] = true },
	} {
		var o map[string]any
		json.Unmarshal([]byte(vectors(t).Current), &o)
		change(o)
		b, _ := json.Marshal(o)
		if ValidateManifest("CURRENT", b) == nil {
			t.Fatal(string(b))
		}
	}
	if ValidateManifest("OTHER", []byte(vectors(t).Current)) == nil {
		t.Fatal("unknown manifest accepted")
	}
}
