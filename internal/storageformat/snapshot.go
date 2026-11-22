package storageformat

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"sort"
	"strconv"
)

const MaxSnapshot = 8 * 1024 * 1024
const MaxSnapshotSessions = 1024

type SnapshotKV struct {
	Key   string
	Value []byte
}
type SnapshotResult struct {
	Code             string
	Index            uint64
	Applied, Deleted bool
}
type SnapshotSession struct {
	ClientID            string
	Sequence            uint64
	CommandSHA256, Kind string
	Result              SnapshotResult
}

// Snapshot is a detached logical image. Counts are derived, never trusted as
// allocation hints. Application profile is bound by ClusterID/ConfigHash (ADR-010).
type Snapshot struct {
	ClusterID, ConfigHash     string
	Index, Term, LogicalBytes uint64
	KV                        []SnapshotKV
	Sessions                  []SnapshotSession
}

func snapshotResult(s SnapshotSession, boundary uint64) (map[string]any, bool) {
	r := s.Result
	if !hexString(s.ClientID, 32) || !hexString(s.CommandSHA256, 64) || s.Sequence < 1 || s.Sequence > 9007199254740991 || r.Index == 0 || r.Index > boundary {
		return nil, false
	}
	o := map[string]any{"code": r.Code, "index": strconv.FormatUint(r.Index, 10)}
	switch s.Kind {
	case "put", "cas", "delete":
	default:
		return nil, false
	}
	switch r.Code {
	case "OK":
		if s.Kind == "delete" {
			if r.Applied {
				return nil, false
			}
			o["deleted"] = r.Deleted
		} else {
			if !r.Applied || r.Deleted {
				return nil, false
			}
			o["applied"] = true
		}
	case "COMPARE_FAILED":
		if s.Kind != "cas" || r.Applied || r.Deleted {
			return nil, false
		}
		o["applied"] = false
	case "RESOURCE_LIMIT":
		if s.Kind == "delete" || r.Applied || r.Deleted {
			return nil, false
		}
		o["applied"] = false
	default:
		return nil, false
	}
	return o, true
}

// EncodeSnapshot sorts private row copies; it never mutates caller ordering.
// Callers retain exclusive ownership of the input while encoding.
func EncodeSnapshot(s Snapshot) ([]byte, error) {
	if !hexString(s.ClusterID, 32) || !hexString(s.ConfigHash, 64) || (s.Index == 0) != (s.Term == 0) || len(s.KV) > 10000 || len(s.Sessions) > MaxSnapshotSessions || s.LogicalBytes > 4194304 {
		return nil, ErrMalformed
	}
	if s.Index == 0 && (len(s.KV) != 0 || len(s.Sessions) != 0 || s.LogicalBytes != 0) {
		return nil, ErrMalformed
	}
	kv := append([]SnapshotKV(nil), s.KV...)
	sessions := append([]SnapshotSession(nil), s.Sessions...)
	sort.Slice(kv, func(i, j int) bool { return kv[i].Key < kv[j].Key })
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].ClientID < sessions[j].ClientID })
	rows := make([]any, 0, len(kv))
	var total uint64
	for i, v := range kv {
		if !validKey(v.Key) || len(v.Value) > 16384 || i > 0 && kv[i-1].Key == v.Key {
			return nil, ErrMalformed
		}
		total += uint64(len(v.Key) + len(v.Value))
		if total > 4194304 {
			return nil, ErrMalformed
		}
		rows = append(rows, map[string]any{"key": v.Key, "value_b64": base64.StdEncoding.EncodeToString(v.Value)})
	}
	if total != s.LogicalBytes {
		return nil, ErrMalformed
	}
	records := make([]any, 0, len(sessions))
	for i, v := range sessions {
		r, ok := snapshotResult(v, s.Index)
		if !ok || i > 0 && sessions[i-1].ClientID == v.ClientID {
			return nil, ErrMalformed
		}
		rb, _ := json.Marshal(r)
		if len(rb) > 512 {
			return nil, ErrMalformed
		}
		records = append(records, map[string]any{"client_id": v.ClientID, "sequence": v.Sequence, "command_sha256": v.CommandSHA256, "kind": v.Kind, "result": r})
	}
	payload, err := json.Marshal(map[string]any{"format_version": 1, "cluster_id": s.ClusterID, "config_hash": s.ConfigHash, "last_included_index": strconv.FormatUint(s.Index, 10), "last_included_term": strconv.FormatUint(s.Term, 10), "logical_bytes": strconv.FormatUint(total, 10), "key_count": strconv.Itoa(len(kv)), "session_count": strconv.Itoa(len(sessions)), "kv": rows, "sessions": records})
	if err != nil || len(payload) > MaxSnapshot-44 {
		return nil, ErrMalformed
	}
	image := make([]byte, 12, 44+len(payload))
	copy(image, "QSN1")
	binary.BigEndian.PutUint64(image[4:], uint64(len(payload)))
	image = append(image, payload...)
	sum := sha256.Sum256(image)
	return append(image, sum[:]...), nil
}

// DecodeSnapshot checks framing/digest before parsing. Streaming rows are
// capped by their validated counts; an attacker cannot build an unbounded JSON
// object graph by placing millions of tiny records inside an 8 MiB envelope.
// Returned strings/values/records do not alias the source byte buffer.
func DecodeSnapshot(image []byte) (Snapshot, error) {
	bad := func(e error) (Snapshot, error) { return Snapshot{}, e }
	if len(image) < 4 {
		return bad(ErrIncomplete)
	}
	if string(image[:4]) != "QSN1" {
		return bad(ErrVersion)
	}
	if len(image) < 12 {
		return bad(ErrIncomplete)
	}
	size := binary.BigEndian.Uint64(image[4:12])
	if size > MaxSnapshot-44 || size == 0 || len(image) > MaxSnapshot {
		return bad(ErrMalformed)
	}
	want := int(size) + 44
	if len(image) < want {
		return bad(ErrIncomplete)
	}
	if len(image) != want {
		return bad(ErrMalformed)
	}
	hash := sha256.Sum256(image[:want-32])
	if !bytes.Equal(hash[:], image[want-32:]) {
		return bad(ErrChecksum)
	}
	raw := image[12 : want-32]
	depth := 0
	quoted := false
	for _, c := range raw {
		if c <= 32 || c >= 127 || c == '\\' {
			return bad(ErrCanonical)
		}
		if c == '"' {
			quoted = !quoted
		}
		if !quoted {
			switch c {
			case '{', '[':
				depth++
				if depth > 16 {
					return bad(ErrMalformed)
				}
			case '}', ']':
				depth--
				if depth < 0 {
					return bad(ErrMalformed)
				}
			}
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	t, err := d.Token()
	if err != nil || t != json.Delim('{') {
		return bad(ErrMalformed)
	}
	var s Snapshot
	var keyCount, sessionCount uint64
	names := []string{"cluster_id", "config_hash", "format_version", "key_count", "kv", "last_included_index", "last_included_term", "logical_bytes", "session_count", "sessions"}
	for _, name := range names {
		key, e := d.Token()
		if e != nil || key != name {
			return bad(ErrCanonical)
		}
		if name == "kv" || name == "sessions" {
			t, e := d.Token()
			if e != nil || t != json.Delim('[') {
				return bad(ErrMalformed)
			}
			count := keyCount
			if name == "sessions" {
				count = sessionCount
			}
			for d.More() {
				length := len(s.KV)
				if name == "sessions" {
					length = len(s.Sessions)
				}
				if uint64(length) >= count {
					return bad(ErrMalformed)
				}
				var row json.RawMessage
				if d.Decode(&row) != nil {
					return bad(ErrMalformed)
				}
				limit := MaxEntry
				if name == "sessions" {
					limit = MaxManifest
				}
				o, e := object(row, limit)
				if e != nil {
					return bad(e)
				}
				if name == "kv" {
					o, ok := fields(o, "key value_b64", "")
					if !ok || !validKey(o["key"]) {
						return bad(ErrMalformed)
					}
					v, ok := value(o["value_b64"])
					if !ok {
						return bad(ErrMalformed)
					}
					k := o["key"].(string)
					if len(s.KV) > 0 && s.KV[len(s.KV)-1].Key >= k {
						return bad(ErrCanonical)
					}
					s.KV = append(s.KV, SnapshotKV{k, v})
				} else {
					o, ok := fields(o, "client_id sequence command_sha256 kind result", "")
					if !ok || !hexString(o["client_id"], 32) || !hexString(o["command_sha256"], 64) {
						return bad(ErrMalformed)
					}
					seq, ok := o["sequence"].(json.Number)
					if !ok {
						return bad(ErrMalformed)
					}
					n, e := strconv.ParseUint(string(seq), 10, 64)
					if e != nil || strconv.FormatUint(n, 10) != string(seq) {
						return bad(ErrMalformed)
					}
					kind, ok := o["kind"].(string)
					if !ok {
						return bad(ErrMalformed)
					}
					r, ok := fields(o["result"], "code index", "applied deleted")
					if !ok {
						return bad(ErrMalformed)
					}
					ri, ok := uintString(r["index"])
					if !ok {
						return bad(ErrMalformed)
					}
					code, ok := r["code"].(string)
					if !ok {
						return bad(ErrMalformed)
					}
					applied, _ := r["applied"].(bool)
					deleted, _ := r["deleted"].(bool)
					ss := SnapshotSession{o["client_id"].(string), n, o["command_sha256"].(string), kind, SnapshotResult{code, ri, applied, deleted}}
					canonical, ok := snapshotResult(ss, ^uint64(0))
					if !ok {
						return bad(ErrMalformed)
					}
					a, _ := json.Marshal(canonical)
					b, _ := json.Marshal(r)
					if len(b) > 512 || !bytes.Equal(a, b) {
						return bad(ErrMalformed)
					}
					if len(s.Sessions) > 0 && s.Sessions[len(s.Sessions)-1].ClientID >= ss.ClientID {
						return bad(ErrCanonical)
					}
					s.Sessions = append(s.Sessions, ss)
				}
			}
			if t, e = d.Token(); e != nil || t != json.Delim(']') {
				return bad(ErrMalformed)
			}
			length := len(s.KV)
			if name == "sessions" {
				length = len(s.Sessions)
			}
			if uint64(length) != count {
				return bad(ErrMalformed)
			}
			continue
		}
		v, e := d.Token()
		if e != nil {
			return bad(ErrMalformed)
		}
		switch name {
		case "cluster_id":
			if !hexString(v, 32) {
				return bad(ErrMalformed)
			}
			s.ClusterID = v.(string)
		case "config_hash":
			if !hexString(v, 64) {
				return bad(ErrMalformed)
			}
			s.ConfigHash = v.(string)
		case "format_version":
			if v != json.Number("1") {
				return bad(ErrVersion)
			}
		default:
			n, ok := uintString(v)
			if !ok {
				return bad(ErrMalformed)
			}
			switch name {
			case "key_count":
				if n > 10000 {
					return bad(ErrMalformed)
				}
				keyCount = n
			case "session_count":
				if n > MaxSnapshotSessions {
					return bad(ErrMalformed)
				}
				sessionCount = n
			case "last_included_index":
				s.Index = n
			case "last_included_term":
				s.Term = n
			case "logical_bytes":
				if n > 4194304 {
					return bad(ErrMalformed)
				}
				s.LogicalBytes = n
			}
		}
	}
	if t, e := d.Token(); e != nil || t != json.Delim('}') {
		return bad(ErrCanonical)
	}
	if _, e := d.Token(); e != io.EOF {
		return bad(ErrCanonical)
	}
	canonical, e := EncodeSnapshot(s)
	if e != nil {
		return bad(e)
	}
	if !bytes.Equal(canonical, image) {
		return bad(ErrCanonical)
	}
	return s, nil
}
func ValidateSnapshot(image []byte) error { _, e := DecodeSnapshot(image); return e }
