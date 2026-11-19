package statemachine

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"
)

func TestT009ValueBoundaries(t *testing.T) {
	for _, size := range []int{0, 1, MaxValueBytes} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := New()
			value := bytes.Repeat([]byte{0xff}, size)
			assertResult(t, m.Apply(Command{Operation: Put, Key: "k", Value: value}), Result{Code: OK, Applied: true})
			assertResult(t, m.Apply(Command{Operation: Get, Key: "k"}), Result{Code: OK, Exists: true, Value: value})
			replacement := bytes.Repeat([]byte{0x80}, size)
			assertResult(t, m.Apply(Command{Operation: CAS, Key: "k", Expected: &Expectation{true, value}, Value: replacement}), Result{Code: OK, Applied: true})
			assertResult(t, m.Apply(Command{Operation: Get, Key: "k"}), Result{Code: OK, Exists: true, Value: replacement})
			assertInvariants(t, m)
		})
	}
}

func TestT009KeyQuota(t *testing.T) {
	m := New()
	// Build a consistent boundary fixture, not 10,000 client operations. It is
	// private test state and not an import, snapshot or persistence interface.
	m.values = make(map[string][]byte, MaxKeys)
	for i := 0; i < MaxKeys-1; i++ {
		key := fmt.Sprintf("k%05d", i)
		m.values[key] = []byte{}
		m.logicalBytes += uint64(len(key))
	}
	assertInvariants(t, m)
	assertResult(t, m.Apply(Command{Operation: Put, Key: "last"}), Result{Code: OK, Applied: true})
	before := view(m)
	for _, c := range []Command{
		{Operation: Put, Key: "overflow"},
		{Operation: CAS, Key: "overflow", Expected: &Expectation{}},
	} {
		assertResult(t, m.Apply(c), Result{Code: ResourceLimit})
		if !reflect.DeepEqual(view(m), before) {
			t.Fatal("key quota rejection changed state")
		}
	}
	// Replacing an existing key at the key-count limit must still work.
	assertResult(t, m.Apply(Command{Operation: Put, Key: "last", Value: []byte("x")}), Result{Code: OK, Applied: true})
	assertResult(t, m.Apply(Command{Operation: CAS, Key: "last", Expected: &Expectation{true, []byte("x")}}), Result{Code: OK, Applied: true})
	assertResult(t, m.Apply(Command{Operation: Delete, Key: "last"}), Result{Code: OK, Deleted: true})
	assertResult(t, m.Apply(Command{Operation: Put, Key: "replacement"}), Result{Code: OK, Applied: true})
	assertInvariants(t, m)
}

func TestT009LogicalByteQuota(t *testing.T) {
	m := New()
	large := bytes.Repeat([]byte{0xa5}, MaxValueBytes)
	// Fewer than 300 transitions reach the actual 4 MiB boundary. Account for
	// keys independently instead of assuming 256 maximum values fit.
	used := 0
	for i := 0; i < 255; i++ {
		key := fmt.Sprintf("k%03d", i)
		assertResult(t, m.Apply(Command{Operation: Put, Key: key, Value: large}), Result{Code: OK, Applied: true})
		used += len(key) + len(large)
	}
	last := bytes.Repeat([]byte{0x5a}, MaxLogicalBytes-used-len("last"))
	if len(last)+1 > MaxValueBytes {
		t.Fatal("bad quota fixture")
	}
	assertResult(t, m.Apply(Command{Operation: Put, Key: "last", Value: last}), Result{Code: OK, Applied: true})
	if m.logicalBytes != MaxLogicalBytes {
		t.Fatalf("fixture uses %d bytes, want exact quota", m.logicalBytes)
	}
	before := view(m)
	over := append(append([]byte{}, last...), 1)
	for _, c := range []Command{
		{Operation: Put, Key: "last", Value: over},
		{Operation: CAS, Key: "last", Expected: &Expectation{true, last}, Value: over},
		{Operation: Put, Key: "new"},
		{Operation: CAS, Key: "new", Expected: &Expectation{}},
	} {
		assertResult(t, m.Apply(c), Result{Code: ResourceLimit})
		if !reflect.DeepEqual(view(m), before) {
			t.Fatal("byte quota rejection changed state")
		}
	}
	// A mismatch stays a comparison failure even when there is no spare space.
	assertResult(t, m.Apply(Command{Operation: CAS, Key: "last", Expected: &Expectation{true, []byte("wrong")}, Value: over}), Result{Code: CompareFailed})
	assertResult(t, m.Apply(Command{Operation: Get, Key: "last"}), Result{Code: OK, Exists: true, Value: last})
	assertResult(t, m.Apply(Command{Operation: CAS, Key: "last", Expected: &Expectation{true, last}}), Result{Code: OK, Applied: true})
	assertResult(t, m.Apply(Command{Operation: Put, Key: "new", Value: []byte("v")}), Result{Code: OK, Applied: true})
	assertResult(t, m.Apply(Command{Operation: Delete, Key: "k000"}), Result{Code: OK, Deleted: true})
	assertResult(t, m.Apply(Command{Operation: Put, Key: "reused", Value: large}), Result{Code: OK, Applied: true})
	assertInvariants(t, m)
}
