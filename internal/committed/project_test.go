package committed

import (
	"encoding/json"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

func TestNoopAndReadBoundary(t *testing.T) {
	raw := []byte(`{"index":"1","kind":"noop","term":"1"}`)
	c, err := Project(storageformat.LogEntry{Index: 1, Term: 1, Raw: raw})
	if err != nil {
		t.Fatal(err)
	}
	m := statemachine.New()
	if r := Wrap(m).Apply(c); r.Code != statemachine.OK || r.Applied {
		t.Fatal(r)
	}
	raw = []byte(`{"command":{"key":"x"},"index":"2","kind":"read","term":"1"}`)
	if _, err = Project(storageformat.LogEntry{Index: 2, Term: 1, Raw: raw}); err != ErrKind {
		t.Fatal("M10 read path enabled", err)
	}
	if _, err = Project(storageformat.LogEntry{Raw: json.RawMessage(`{}`)}); err == nil {
		t.Fatal("invalid projection")
	}
}
