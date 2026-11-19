package storageformat

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"strconv"
)

func entry(v any) bool {
	o, ok := fields(v, "index term kind", "command")
	if !ok {
		return false
	}
	i, ok := uintString(o["index"])
	if !ok || i == 0 {
		return false
	}
	t, ok := uintString(o["term"])
	if !ok || t == 0 {
		return false
	}
	k, ok := o["kind"].(string)
	if !ok {
		return false
	}
	if k == "noop" {
		_, exists := o["command"]
		return !exists
	}
	var required string
	switch k {
	case "read":
		required = "key"
	case "put":
		required = "key value_b64 client_id sequence"
	case "delete":
		required = "key client_id sequence"
	case "cas":
		required = "key expected new_value_b64 client_id sequence"
	default:
		return false
	}
	c, ok := fields(o["command"], required, "")
	if !ok || !validKey(c["key"]) {
		return false
	}
	if k != "read" {
		n, ok := c["sequence"].(json.Number)
		if !ok {
			return false
		}
		u, err := strconv.ParseUint(string(n), 10, 64)
		if err != nil || u < 1 || u > 9007199254740991 || strconv.FormatUint(u, 10) != string(n) || !hexString(c["client_id"], 32) {
			return false
		}
	}
	if k == "put" {
		_, ok := value(c["value_b64"])
		return ok
	}
	if k == "cas" {
		e, ok := fields(c["expected"], "exists", "value_b64")
		if !ok {
			return false
		}
		exists, ok := e["exists"].(bool)
		if !ok {
			return false
		}
		if exists {
			if _, ok := value(e["value_b64"]); !ok {
				return false
			}
		} else if _, has := e["value_b64"]; has {
			return false
		}
		_, ok = value(c["new_value_b64"])
		return ok
	}
	return true
}

// ValidateEntry checks one canonical log entry, not its relation to a log.
func ValidateEntry(raw []byte) error {
	o, err := object(raw, MaxEntry)
	if err != nil {
		return err
	}
	if !entry(o) {
		return ErrMalformed
	}
	return nil
}

// CommandDigest excludes index/term and client identity/sequence as specified.
func CommandDigest(raw []byte) ([32]byte, error) {
	o, err := object(raw, MaxEntry)
	if err != nil {
		return [32]byte{}, err
	}
	if !entry(o) {
		return [32]byte{}, ErrMalformed
	}
	k := o["kind"].(string)
	opcode := map[string]byte{"noop": 0, "put": 1, "delete": 2, "cas": 3, "read": 4}[k]
	b := append([]byte("QCMD\x01"), opcode)
	prefix := func(v []byte) { b = binary.BigEndian.AppendUint32(b, uint32(len(v))); b = append(b, v...) }
	if k != "noop" {
		c := o["command"].(map[string]any)
		prefix([]byte(c["key"].(string)))
		if k == "put" {
			v, _ := value(c["value_b64"])
			prefix(v)
		}
		if k == "cas" {
			e := c["expected"].(map[string]any)
			if e["exists"].(bool) {
				b = append(b, 1)
				v, _ := value(e["value_b64"])
				prefix(v)
			} else {
				b = append(b, 0)
			}
			v, _ := value(c["new_value_b64"])
			prefix(v)
		}
	}
	return sha256.Sum256(b), nil
}
