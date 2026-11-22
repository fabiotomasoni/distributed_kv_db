// Package committed projects approved entries into M02 work, with logged reads
// behind an explicit M10 capability. It owns no scheduling, consensus or sessions.
package committed

import (
	"encoding/base64"
	"errors"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

var ErrKind = errors.New("committed: unsupported application kind")

const noop statemachine.Operation = "raft/noop"

func Project(e storageformat.LogEntry) (statemachine.Command, error) { return project(e, false) }

// ProjectWithReads adds the approved read(key) -> M01 Get projection. The M02
// worker captures its exact-slot result; this function itself reads no map.
func ProjectWithReads(e storageformat.LogEntry) (statemachine.Command, error) {
	return project(e, true)
}
func project(e storageformat.LogEntry, reads bool) (statemachine.Command, error) {
	if err := storageformat.ValidateEntry(e.Raw); err != nil {
		return statemachine.Command{}, err
	}
	v, err := strictjson.Decode(e.Raw, storageformat.MaxEntry)
	if err != nil {
		return statemachine.Command{}, err
	}
	kind, _ := v["kind"].(string)
	if kind == "noop" {
		return statemachine.Command{Operation: noop}, nil
	}
	if kind == "read" {
		if !reads {
			return statemachine.Command{}, ErrKind
		}
		return statemachine.Command{Operation: statemachine.Get, Key: v["command"].(map[string]any)["key"].(string)}, nil
	}
	m := v["command"].(map[string]any)
	key := m["key"].(string)
	c := statemachine.Command{Operation: statemachine.Operation(kind), Key: key}
	switch kind {
	case "put":
		c.Value, _ = base64.StdEncoding.Strict().DecodeString(m["value_b64"].(string))
	case "delete":
	case "cas":
		expected := m["expected"].(map[string]any)
		c.Expected = &statemachine.Expectation{Exists: expected["exists"].(bool)}
		if c.Expected.Exists {
			c.Expected.Value, _ = base64.StdEncoding.Strict().DecodeString(expected["value_b64"].(string))
		}
		c.Value, _ = base64.StdEncoding.Strict().DecodeString(m["new_value_b64"].(string))
	default:
		return statemachine.Command{}, ErrKind
	}
	return c, nil
}

// Wrap intercepts the private noop inside the M02 worker and advances the global
// application slot without sending an unknown operation into M01.
func Wrap(machine applier.Machine) applier.Machine { return bridge{machine} }

type bridge struct{ machine applier.Machine }

func (b bridge) Apply(c statemachine.Command) statemachine.Result {
	if c.Operation == noop {
		return statemachine.Result{Code: statemachine.OK}
	}
	return b.machine.Apply(c)
}
