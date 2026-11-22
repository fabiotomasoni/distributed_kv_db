package storageformat

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestSessionIdentityV2GoldenAndLegacyRejection(t *testing.T) {
	raw, e := os.ReadFile("../../docs/fixtures/configuration-v2-sessions.json")
	if e != nil {
		t.Fatal(e)
	}
	var f struct {
		Identity string `json:"identity_json"`
	}
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	if e = ValidateSessionIdentity([]byte(f.Identity)); e != nil {
		t.Fatal(e)
	}
	if e = ValidateManifest("IDENTITY", []byte(f.Identity)); e == nil {
		t.Fatal("v1 validator accepted v2")
	}
	var data map[string]any
	_ = json.Unmarshal([]byte(f.Identity), &data)
	delete(data, "checksum")
	b, _ := json.Marshal(data)
	sealed, e := SealSessionIdentity(b)
	if e != nil || !bytes.Equal(sealed, []byte(f.Identity)) {
		t.Fatal("golden", e)
	}
	data["extra"] = true
	b, _ = json.Marshal(data)
	if _, e = SealSessionIdentity(b); e == nil {
		t.Fatal("unknown field")
	}
}
