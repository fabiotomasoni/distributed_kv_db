package storageformat

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
)

// EmptySnapshot encodes only the approved empty bootstrap checkpoint, not a
// general snapshot capture/installation codec (M13).
func EmptySnapshot(clusterID, configHash string) ([]byte, error) {
	if !hexString(clusterID, 32) || !hexString(configHash, 64) {
		return nil, ErrMalformed
	}
	p, _ := json.Marshal(map[string]any{"format_version": 1, "cluster_id": clusterID, "config_hash": configHash, "last_included_index": "0", "last_included_term": "0", "logical_bytes": "0", "key_count": "0", "session_count": "0", "kv": []any{}, "sessions": []any{}})
	b := make([]byte, 12, len(p)+44)
	copy(b, "QSN1")
	binary.BigEndian.PutUint64(b[4:], uint64(len(p)))
	b = append(b, p...)
	h := sha256.Sum256(b)
	return append(b, h[:]...), nil
}
