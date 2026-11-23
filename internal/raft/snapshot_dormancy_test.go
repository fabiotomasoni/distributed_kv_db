package raft

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
)

func TestOrdinaryCoreSnapshotRequestStillUnavailable(t *testing.T) {
	x := engine(t, "n2", initial())
	if e := x.EnableReplication(16); e != nil {
		t.Fatal(e)
	}
	req := wire(t, x.cfg, peerprotocol.Snapshot, "n1", 5, map[string]any{"leader_id": "n1", "snapshot_id": strings.Repeat("a", 64), "sha256": strings.Repeat("a", 64), "last_included_index": "4", "last_included_term": "2", "offset": "0", "data_b64": base64.StdEncoding.EncodeToString(make([]byte, 44)), "done": true, "total_bytes": "44"})
	out, e := x.Request(1, req)
	if e != nil {
		t.Fatal(e)
	}
	if out.Persist == nil {
		t.Fatal("higher term not synchronized")
	}
	out = complete(t, x, out)
	if !errors.Is(out.Rejected, ErrUnsupported) || out.Reply != nil || x.Status().LastIndex != 0 || x.baseIndex != 0 || x.Status().Term != 5 {
		t.Fatal("ordinary runtime install capability changed", out, x.Status())
	}
}
