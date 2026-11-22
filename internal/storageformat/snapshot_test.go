package storageformat

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

type snapshotVector struct {
	Payload string `json:"payload_json"`
	Hex     string `json:"image_hex"`
	SHA     string `json:"file_sha256"`
	Trailer string `json:"trailer_sha256"`
}

func vectorFile(t *testing.T, path string) map[string]snapshotVector {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var all map[string]json.RawMessage
	if e = json.Unmarshal(raw, &all); e != nil {
		t.Fatal(e)
	}
	out := map[string]snapshotVector{}
	for k, v := range all {
		var s snapshotVector
		if json.Unmarshal(v, &s) == nil && s.Hex != "" {
			out[k] = s
		}
	}
	return out
}
func vectorBytes(t *testing.T, v snapshotVector) []byte {
	t.Helper()
	b, e := hex.DecodeString(v.Hex)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func imagePayload(raw []byte) []byte {
	b := make([]byte, 12)
	copy(b, "QSN1")
	binary.BigEndian.PutUint64(b[4:], uint64(len(raw)))
	b = append(b, raw...)
	sum := sha256.Sum256(b)
	return append(b, sum[:]...)
}
func TestSnapshotApprovedV1AndV2ByteVectors(t *testing.T) {
	for _, path := range []string{"../../docs/fixtures/canonical-v1.json", "../../docs/fixtures/snapshot-v2.json"} {
		count := 0
		for name, v := range vectorFile(t, path) {
			count++
			t.Run(path+name, func(t *testing.T) {
				b := vectorBytes(t, v)
				sum := sha256.Sum256(b)
				if hex.EncodeToString(sum[:]) != v.SHA || string(b[12:len(b)-32]) != v.Payload {
					t.Fatal("independent vector mismatch")
				}
				if v.Trailer != "" && hex.EncodeToString(b[len(b)-32:]) != v.Trailer {
					t.Fatal("trailer vector mismatch")
				}
				s, e := DecodeSnapshot(b)
				if e != nil {
					t.Fatal(e)
				}
				encoded, e := EncodeSnapshot(s)
				if e != nil || !bytes.Equal(b, encoded) {
					t.Fatal("byte round trip", e)
				}
				if s.Index == 0 {
					empty, e := EmptySnapshot(s.ClusterID, s.ConfigHash)
					if e != nil || !bytes.Equal(empty, b) {
						t.Fatal("bootstrap changed", e)
					}
				}
				for i, j := 0, len(s.KV)-1; i < j; i, j = i+1, j-1 {
					s.KV[i], s.KV[j] = s.KV[j], s.KV[i]
				}
				for i, j := 0, len(s.Sessions)-1; i < j; i, j = i+1, j-1 {
					s.Sessions[i], s.Sessions[j] = s.Sessions[j], s.Sessions[i]
				}
				before := s
				before.KV = append([]SnapshotKV(nil), s.KV...)
				before.Sessions = append([]SnapshotSession(nil), s.Sessions...)
				encoded, e = EncodeSnapshot(s)
				if e != nil || !bytes.Equal(encoded, b) || !reflect.DeepEqual(before, s) {
					t.Fatal("ordering/mutation", e)
				}
				decoded, e := DecodeSnapshot(b)
				if e != nil {
					t.Fatal(e)
				}
				if len(decoded.KV) > 0 {
					value := bytes.Clone(decoded.KV[0].Value)
					for i := range b {
						b[i] = 0
					}
					if !bytes.Equal(value, decoded.KV[0].Value) {
						t.Fatal("source buffer alias")
					}
				}
			})
		}
		if count != 2 {
			t.Fatal("missing vectors", path, count)
		}
	}
}
func TestSnapshotRejectsCorruptionCanonicalAndSemanticErrors(t *testing.T) {
	v := vectorFile(t, "../../docs/fixtures/snapshot-v2.json")["stateful"]
	good := vectorBytes(t, v)
	reject := func(raw []byte) {
		t.Helper()
		s, e := DecodeSnapshot(raw)
		if e == nil || s.Index != 0 || s.KV != nil || s.Sessions != nil {
			t.Fatal("invalid image returned partial/success", e)
		}
	}
	for _, n := range []int{0, 1, 3, 4, 11, 12, 20, len(good) / 2, len(good) - 33, len(good) - 1} {
		reject(good[:n])
	}
	wrong := bytes.Clone(good)
	wrong[3] = '2'
	reject(wrong)
	wrong = bytes.Clone(good)
	wrong[len(wrong)-1] ^= 1
	reject(wrong)
	reject(append(bytes.Clone(good), 0))
	wrong = bytes.Clone(good)
	binary.BigEndian.PutUint64(wrong[4:], ^uint64(0))
	reject(wrong)
	wrong = bytes.Clone(good)
	binary.BigEndian.PutUint64(wrong[4:], 0)
	reject(wrong)
	mutations := []func(map[string]any){
		func(o map[string]any) { o["format_version"] = 2 }, func(o map[string]any) { o["last_included_term"] = "0" }, func(o map[string]any) { o["last_included_index"] = "0" }, func(o map[string]any) { o["last_included_index"] = "01" }, func(o map[string]any) { o["key_count"] = "10001" }, func(o map[string]any) { o["key_count"] = "18446744073709551616" }, func(o map[string]any) { o["session_count"] = "1025" }, func(o map[string]any) { o["logical_bytes"] = "4194305" }, func(o map[string]any) { o["logical_bytes"] = "0" }, func(o map[string]any) { o["unknown"] = true },
		func(o map[string]any) { o["kv"].([]any)[0].(map[string]any)["value_b64"] = "AB==" }, func(o map[string]any) { o["kv"].([]any)[0].(map[string]any)["value_b64"] = nil }, func(o map[string]any) { o["kv"].([]any)[1] = o["kv"].([]any)[0] }, func(o map[string]any) { k := o["kv"].([]any); k[0], k[1] = k[1], k[0] },
		func(o map[string]any) { o["sessions"].([]any)[1] = o["sessions"].([]any)[0] }, func(o map[string]any) { k := o["sessions"].([]any); k[0], k[1] = k[1], k[0] },
		func(o map[string]any) { o["sessions"].([]any)[0].(map[string]any)["sequence"] = 0 }, func(o map[string]any) { o["sessions"].([]any)[0].(map[string]any)["sequence"] = 9007199254740992 }, func(o map[string]any) { o["sessions"].([]any)[0].(map[string]any)["sequence"] = 1.5 },
		func(o map[string]any) { o["sessions"].([]any)[0].(map[string]any)["kind"] = "read" }, func(o map[string]any) {
			o["sessions"].([]any)[0].(map[string]any)["command_sha256"] = strings.Repeat("A", 64)
		},
		func(o map[string]any) {
			o["sessions"].([]any)[0].(map[string]any)["result"].(map[string]any)["index"] = "10"
		}, func(o map[string]any) {
			o["sessions"].([]any)[0].(map[string]any)["result"].(map[string]any)["index"] = "0"
		}, func(o map[string]any) {
			o["sessions"].([]any)[0].(map[string]any)["result"].(map[string]any)["applied"] = false
		},
		func(o map[string]any) {
			o["sessions"].([]any)[0].(map[string]any)["result"].(map[string]any)["value_b64"] = ""
		}, func(o map[string]any) {
			o["sessions"].([]any)[1].(map[string]any)["result"].(map[string]any)["code"] = "RESOURCE_LIMIT"
		}, func(o map[string]any) {
			o["sessions"].([]any)[0].(map[string]any)["result"].(map[string]any)["code"] = "STALE_SEQUENCE"
		},
	}
	for i, mutate := range mutations {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			var o map[string]any
			_ = json.Unmarshal([]byte(v.Payload), &o)
			mutate(o)
			raw, _ := json.Marshal(o)
			reject(imagePayload(raw))
		})
	}
	for _, raw := range []string{" " + v.Payload, strings.Replace(v.Payload, `"cluster_id":`, `"cluster_id":null,"cluster_id":`, 1), strings.Replace(v.Payload, `"cluster_id"`, `"cluste\u0072_id"`, 1), strings.Replace(v.Payload, `"format_version":1`, `"format_version":1e0`, 1)} {
		reject(imagePayload([]byte(raw)))
	}
	// Counts are checked before allocating/decoding excessive rows.
	raw := strings.Replace(v.Payload, `"key_count":"3"`, `"key_count":"0"`, 1)
	reject(imagePayload([]byte(raw)))
}
func TestSnapshotLogicalAndSessionMaximumBounds(t *testing.T) {
	s := Snapshot{ClusterID: strings.Repeat("1", 32), ConfigHash: strings.Repeat("a", 64), Index: ^uint64(0), Term: ^uint64(0)}
	remaining := 4194304
	for i := 0; remaining > 0; i++ {
		key := fmt.Sprintf("k%04d", i)
		n := remaining - len(key)
		if n > 16384 {
			n = 16384
		}
		if n < 0 {
			t.Fatal("fixture accounting")
		}
		s.KV = append(s.KV, SnapshotKV{key, make([]byte, n)})
		remaining -= len(key) + n
	}
	s.LogicalBytes = 4194304
	for i := 0; i < 1024; i++ {
		s.Sessions = append(s.Sessions, SnapshotSession{ClientID: fmt.Sprintf("%032x", i), Sequence: 9007199254740991, CommandSHA256: strings.Repeat("a", 64), Kind: "cas", Result: SnapshotResult{Code: "COMPARE_FAILED", Index: ^uint64(0)}})
	}
	b, e := EncodeSnapshot(s)
	if e != nil || len(b) > 6782978 {
		t.Fatal(len(b), e)
	}
	decoded, e := DecodeSnapshot(b)
	if e != nil || decoded.LogicalBytes != 4194304 || len(decoded.Sessions) != 1024 || decoded.Index != ^uint64(0) {
		t.Fatal(e)
	}
	s.Sessions = append(s.Sessions, s.Sessions[0])
	if _, e = EncodeSnapshot(s); e == nil {
		t.Fatal("session cap")
	}
	s.Sessions = s.Sessions[:1024]
	s.LogicalBytes++
	if _, e = EncodeSnapshot(s); e == nil {
		t.Fatal("logical cap")
	}
	s.LogicalBytes = 4194304
	s.KV[0].Value = make([]byte, 16385)
	if _, e = EncodeSnapshot(s); e == nil {
		t.Fatal("value cap")
	}
	s.KV = make([]SnapshotKV, 10001)
	if _, e = EncodeSnapshot(s); e == nil {
		t.Fatal("key count")
	}
}

func TestSnapshotMaximumKeyCountAndKeyLength(t *testing.T) {
	s := Snapshot{ClusterID: strings.Repeat("1", 32), ConfigHash: strings.Repeat("a", 64), Index: 1, Term: 1}
	for i := 0; i < 10000; i++ {
		key := fmt.Sprintf("k%04d", i)
		s.KV = append(s.KV, SnapshotKV{Key: key})
		s.LogicalBytes += uint64(len(key))
	}
	raw, e := EncodeSnapshot(s)
	if e != nil {
		t.Fatal(e)
	}
	restored, e := DecodeSnapshot(raw)
	if e != nil || len(restored.KV) != 10000 {
		t.Fatal(e)
	}
	s.KV = []SnapshotKV{{Key: strings.Repeat("k", 256), Value: make([]byte, 16384)}}
	s.LogicalBytes = 256 + 16384
	raw, e = EncodeSnapshot(s)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = DecodeSnapshot(raw); e != nil {
		t.Fatal(e)
	}
	s.KV[0].Key += "k"
	s.LogicalBytes++
	if _, e = EncodeSnapshot(s); e == nil {
		t.Fatal("key length cap")
	}
}
