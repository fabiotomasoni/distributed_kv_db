package storageformat

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

// SealManifest builds a canonical IDENTITY or CURRENT from canonical data fields
// without checksum. Unknown fields and nulls are rejected, never omitted silently.
func SealManifest(kind string, data []byte) ([]byte, error) { return sealManifest(kind, data, 1) }
func SealSessionIdentity(data []byte) ([]byte, error)       { return sealManifest("IDENTITY", data, 2) }
func sealManifest(kind string, data []byte, version int) ([]byte, error) {
	o, err := object(data, MaxManifest)
	if err != nil {
		return nil, err
	}
	if !manifestVersion(kind, o, false, version) {
		return nil, ErrMalformed
	}
	h := sha256.Sum256(data)
	o["checksum"] = hex.EncodeToString(h[:])
	b, err := json.Marshal(o)
	if err != nil || len(b) > MaxManifest {
		return nil, ErrMalformed
	}
	return b, nil
}

func manifest(kind string, o map[string]any, sealed bool) bool {
	return manifestVersion(kind, o, sealed, 1)
}
func manifestVersion(kind string, o map[string]any, sealed bool, version int) bool {
	required := "format_version cluster_id node_id config_hash"
	if kind == "CURRENT" {
		required = "format_version generation snapshot_sha256 base_index base_term"
	} else if kind != "IDENTITY" {
		return false
	}
	if sealed {
		required += " checksum"
	}
	if _, ok := fields(o, required, ""); !ok {
		return false
	}
	if (version != 1 && (version != 2 || kind != "IDENTITY")) || o["format_version"] != json.Number(fmt.Sprint(version)) {
		return false
	}
	if sealed && !hexString(o["checksum"], 64) {
		return false
	}
	if kind == "IDENTITY" {
		return hexString(o["cluster_id"], 32) && hexString(o["config_hash"], 64) && member(o["node_id"])
	}
	i, ok := uintString(o["base_index"])
	if !ok {
		return false
	}
	t, ok := uintString(o["base_term"])
	if !ok || (i == 0) != (t == 0) {
		return false
	}
	g, ok := o["generation"].(string)
	if !ok || !strings.HasPrefix(g, "gen-") {
		return false
	}
	n, ok := uintString(strings.TrimPrefix(g, "gen-"))
	return ok && n > 0 && hexString(o["snapshot_sha256"], 64)
}

// ValidateManifest checks bytes/schema/checksum, not referenced generation files
// or equality with configuration. Those are future storage-owner obligations.
func ValidateManifest(kind string, raw []byte) error { return validateManifest(kind, raw, 1) }
func ValidateSessionIdentity(raw []byte) error       { return validateManifest("IDENTITY", raw, 2) }
func validateManifest(kind string, raw []byte, version int) error {
	o, err := object(raw, MaxManifest)
	if err != nil {
		return err
	}
	if !manifestVersion(kind, o, true, version) {
		return ErrMalformed
	}
	digest := o["checksum"].(string)
	delete(o, "checksum")
	b, _ := json.Marshal(o)
	h := sha256.Sum256(b)
	if digest != hex.EncodeToString(h[:]) {
		return ErrChecksum
	}
	return nil
}
