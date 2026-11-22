package statemachine

import (
	"bytes"
	"fmt"
	"testing"
)

func TestStateImageOwnershipAndValidation(t *testing.T) {
	m := New()
	m.Apply(Command{Operation: Put, Key: "b", Value: []byte{0, 255}})
	m.Apply(Command{Operation: Put, Key: "a", Value: []byte{}})
	s := m.CopyState()
	if len(s.Records) != 2 || s.Records[0].Key != "a" || s.LogicalBytes != 4 {
		t.Fatal(s)
	}
	restored, e := FromState(s)
	if e != nil {
		t.Fatal(e)
	}
	s.Records[1].Value[0] = 42
	s.Records[0].Key = "changed"
	if !bytes.Equal(m.Apply(Command{Operation: Get, Key: "b"}).Value, []byte{0, 255}) || !bytes.Equal(restored.Apply(Command{Operation: Get, Key: "b"}).Value, []byte{0, 255}) {
		t.Fatal("state alias")
	}
	for _, s := range []StateImage{{Records: []Record{{"x", nil}}, LogicalBytes: 0}, {Records: []Record{{"x", nil}, {"x", nil}}, LogicalBytes: 2}, {Records: []Record{{"!", nil}}, LogicalBytes: 1}, {Records: []Record{{"x", make([]byte, 16385)}}, LogicalBytes: 16386}, {Records: make([]Record, 10001)}, {LogicalBytes: 4194305}} {
		if v, e := FromState(s); e == nil || v != nil {
			t.Fatal("invalid restore")
		}
	}
	rows := make([]Record, 257)
	shared := make([]byte, 16384)
	for i := range rows {
		rows[i] = Record{fmt.Sprintf("k%04d", i), shared}
	}
	if _, e = FromState(StateImage{Records: rows, LogicalBytes: 4194304}); e == nil {
		t.Fatal("aggregate quota")
	}
}
