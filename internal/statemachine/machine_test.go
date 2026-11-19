package statemachine

import (
	"bytes"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// These views are test-only observations, not a snapshot or persistence format.
type entryView struct {
	Key   string
	Value string
}

type stateView struct {
	Entries      []entryView
	LogicalBytes uint64
}

func view(m *Machine) stateView {
	entries := make([]entryView, 0, len(m.values))
	for key, value := range m.values {
		entries = append(entries, entryView{key, string(value)})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	return stateView{entries, m.logicalBytes}
}

func assertInvariants(t *testing.T, m *Machine) {
	t.Helper()
	var total uint64
	for key, value := range m.values {
		if len(key) == 0 || len(key) > MaxKeyBytes || len(value) > MaxValueBytes {
			t.Fatalf("invalid stored entry: key bytes=%d, value bytes=%d", len(key), len(value))
		}
		total += uint64(len(key) + len(value))
	}
	if total != m.logicalBytes || total > MaxLogicalBytes || len(m.values) > MaxKeys {
		t.Fatalf("quota invariant: measured=%d, recorded=%d, keys=%d", total, m.logicalBytes, len(m.values))
	}
}

func assertResult(t *testing.T, got, want Result) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("result = %#v, want %#v", got, want)
	}
}

func TestT009OperationSequence(t *testing.T) {
	m := New()
	cases := []struct {
		name    string
		command Command
		want    Result
	}{
		{"missing", Command{Operation: Get, Key: "config/theme"}, Result{Code: KeyNotFound}},
		{"insert", Command{Operation: Put, Key: "config/theme", Value: []byte("dark")}, Result{Code: OK, Applied: true}},
		{"read", Command{Operation: Get, Key: "config/theme"}, Result{Code: OK, Exists: true, Value: []byte("dark")}},
		{"update", Command{Operation: Put, Key: "config/theme", Value: []byte("light")}, Result{Code: OK, Applied: true}},
		{"read_update", Command{Operation: Get, Key: "config/theme"}, Result{Code: OK, Exists: true, Value: []byte("light")}},
		{"delete", Command{Operation: Delete, Key: "config/theme"}, Result{Code: OK, Deleted: true}},
		{"missing_after_delete", Command{Operation: Get, Key: "config/theme"}, Result{Code: KeyNotFound}},
		{"delete_absent", Command{Operation: Delete, Key: "config/theme"}, Result{Code: OK}},
		{"empty_nil", Command{Operation: Put, Key: "empty"}, Result{Code: OK, Applied: true}},
		{"read_empty", Command{Operation: Get, Key: "empty"}, Result{Code: OK, Exists: true, Value: []byte{}}},
		{"empty_slice", Command{Operation: Put, Key: "empty", Value: []byte{}}, Result{Code: OK, Applied: true}},
		{"binary_value", Command{Operation: Put, Key: "bytes", Value: []byte{0, 255, 128}}, Result{Code: OK, Applied: true}},
		{"read_binary", Command{Operation: Get, Key: "bytes"}, Result{Code: OK, Exists: true, Value: []byte{0, 255, 128}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertResult(t, m.Apply(tc.command), tc.want)
			assertInvariants(t, m)
		})
	}
}

func TestT009CASLocalSemantics(t *testing.T) {
	cases := []struct {
		name     string
		initial  []byte
		present  bool
		expected Expectation
		newValue []byte
		want     Code
	}{
		{"match", []byte("old"), true, Expectation{true, []byte("old")}, []byte("new"), OK},
		{"mismatch", []byte("old"), true, Expectation{true, []byte("other")}, []byte("new"), CompareFailed},
		{"create_absent", nil, false, Expectation{}, []byte("new"), OK},
		{"expected_present_but_absent", nil, false, Expectation{true, []byte{}}, []byte("new"), CompareFailed},
		{"expected_absent_but_present_empty", []byte{}, true, Expectation{}, []byte("new"), CompareFailed},
		{"present_empty_matches", []byte{}, true, Expectation{true, nil}, []byte("new"), OK},
		{"create_empty", nil, false, Expectation{}, nil, OK},
		{"replace_with_empty", []byte("old"), true, Expectation{true, []byte("old")}, nil, OK},
		{"rewrite_same", []byte("same"), true, Expectation{true, []byte("same")}, []byte("same"), OK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New()
			if tc.present {
				assertResult(t, m.Apply(Command{Operation: Put, Key: "k", Value: tc.initial}), Result{Code: OK, Applied: true})
			}
			before := view(m)
			result := m.Apply(Command{Operation: CAS, Key: "k", Value: tc.newValue, Expected: &tc.expected})
			assertResult(t, result, Result{Code: tc.want, Applied: tc.want == OK})
			if tc.want == CompareFailed {
				if !reflect.DeepEqual(view(m), before) {
					t.Fatal("failed CAS changed state")
				}
			} else {
				got := m.Apply(Command{Operation: Get, Key: "k"})
				if got.Code != OK || !got.Exists || !bytes.Equal(got.Value, tc.newValue) {
					t.Fatalf("CAS result not stored: %#v", got)
				}
			}
			assertInvariants(t, m)
		})
	}
}

func TestT009CASOrderedCompetition(t *testing.T) {
	// This is sequential competition, not an M02 concurrency test.
	for _, order := range [][]string{{"11", "10"}, {"10", "11"}} {
		m := New()
		m.Apply(Command{Operation: Put, Key: "stock/pen", Value: []byte("12")})
		for i, replacement := range order {
			got := m.Apply(Command{Operation: CAS, Key: "stock/pen", Value: []byte(replacement), Expected: &Expectation{true, []byte("12")}})
			want := Result{Code: CompareFailed}
			if i == 0 {
				want = Result{Code: OK, Applied: true}
			}
			assertResult(t, got, want)
		}
		assertResult(t, m.Apply(Command{Operation: Get, Key: "stock/pen"}), Result{Code: OK, Exists: true, Value: []byte(order[0])})
	}

	m := New()
	m.Apply(Command{Operation: Put, Key: "k", Value: []byte("same")})
	for i := 0; i < 2; i++ {
		assertResult(t, m.Apply(Command{Operation: CAS, Key: "k", Value: []byte("same"), Expected: &Expectation{true, []byte("same")}}), Result{Code: OK, Applied: true})
	}
}

func TestT009InvalidCommandsAreAtomic(t *testing.T) {
	oversize := make([]byte, MaxValueBytes+1)
	cases := []struct {
		name string
		cmd  Command
	}{
		{"zero_command", Command{}},
		{"unknown_operation", Command{Operation: "append", Key: "k"}},
		{"empty_key", Command{Operation: Put, Value: []byte("v")}},
		{"key_too_long", Command{Operation: Put, Key: strings.Repeat("a", MaxKeyBytes+1)}},
		{"bad_first_character", Command{Operation: Put, Key: "/k"}},
		{"space", Command{Operation: Put, Key: "bad key"}},
		{"non_ascii", Command{Operation: Put, Key: "caf\u00e9"}},
		{"invalid_utf8", Command{Operation: Put, Key: "k\xff"}},
		{"nul", Command{Operation: Put, Key: "k\x00"}},
		{"put_too_large", Command{Operation: Put, Key: "k", Value: oversize}},
		{"get_value", Command{Operation: Get, Key: "k", Value: []byte{}}},
		{"delete_value", Command{Operation: Delete, Key: "k", Value: []byte("v")}},
		{"get_expected", Command{Operation: Get, Key: "k", Expected: &Expectation{}}},
		{"delete_expected", Command{Operation: Delete, Key: "k", Expected: &Expectation{}}},
		{"put_expected", Command{Operation: Put, Key: "k", Expected: &Expectation{}}},
		{"cas_no_expected", Command{Operation: CAS, Key: "k", Value: []byte("v")}},
		{"cas_absent_with_value", Command{Operation: CAS, Key: "k", Expected: &Expectation{false, []byte{}}}},
		{"cas_expected_too_large", Command{Operation: CAS, Key: "k", Expected: &Expectation{true, oversize}}},
		{"cas_new_too_large_before_mismatch", Command{Operation: CAS, Key: "k", Value: oversize, Expected: &Expectation{true, []byte("different")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := New()
			m.Apply(Command{Operation: Put, Key: "k", Value: []byte("keep")})
			before := view(m)
			assertResult(t, m.Apply(tc.cmd), Result{Code: InvalidArgument})
			if !reflect.DeepEqual(view(m), before) {
				t.Fatal("invalid command changed map or accounting")
			}
			assertInvariants(t, m)
		})
	}
}

func TestT009KeyGrammarAndCase(t *testing.T) {
	m := New()
	for _, key := range []string{"A", "a", "0", "Z9._:/-", strings.Repeat("x", MaxKeyBytes)} {
		assertResult(t, m.Apply(Command{Operation: Put, Key: key, Value: []byte(key)}), Result{Code: OK, Applied: true})
		assertResult(t, m.Apply(Command{Operation: Get, Key: key}), Result{Code: OK, Exists: true, Value: []byte(key)})
	}
	assertResult(t, m.Apply(Command{Operation: Get, Key: "A"}), Result{Code: OK, Exists: true, Value: []byte("A")})
	assertResult(t, m.Apply(Command{Operation: Get, Key: "a"}), Result{Code: OK, Exists: true, Value: []byte("a")})
	assertInvariants(t, m)
}

func TestT009BuffersNeverAliasState(t *testing.T) {
	m := New()
	input := []byte("old")
	m.Apply(Command{Operation: Put, Key: "k", Value: input})
	input[0] = 'X'
	first := m.Apply(Command{Operation: Get, Key: "k"})
	assertResult(t, first, Result{Code: OK, Exists: true, Value: []byte("old")})
	first.Value[0] = 'Y'
	assertResult(t, m.Apply(Command{Operation: Get, Key: "k"}), Result{Code: OK, Exists: true, Value: []byte("old")})

	expected, replacement := []byte("old"), []byte("new")
	m.Apply(Command{Operation: CAS, Key: "k", Expected: &Expectation{true, expected}, Value: replacement})
	expected[0], replacement[0] = 'X', 'X'
	assertResult(t, m.Apply(Command{Operation: Get, Key: "k"}), Result{Code: OK, Exists: true, Value: []byte("new")})

	// A retained read result must not change after later writes or deletion.
	retained := m.Apply(Command{Operation: Get, Key: "k"})
	m.Apply(Command{Operation: Put, Key: "k", Value: []byte("other")})
	m.Apply(Command{Operation: Delete, Key: "k"})
	assertResult(t, retained, Result{Code: OK, Exists: true, Value: []byte("new")})
	assertInvariants(t, m)
}

func TestT009ValuesAreWholeOpaqueDocuments(t *testing.T) {
	m := New()
	m.Apply(Command{Operation: Put, Key: "resource/7", Value: []byte("{\"a\":1,\"b\":2}")})
	m.Apply(Command{Operation: Put, Key: "resource/7", Value: []byte("{\"a\":3}")})
	assertResult(t, m.Apply(Command{Operation: Get, Key: "resource/7"}), Result{Code: OK, Exists: true, Value: []byte("{\"a\":3}")})
}
