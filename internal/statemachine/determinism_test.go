package statemachine

import (
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"testing"
)

// The oracle uses a linear list and immutable strings, not the production map,
// byte-accounting helper or validator. Its inputs are valid and far below quota;
// explicit boundary and rejection tests cover validation and resource limits.
type referenceModel []entryView

func (r *referenceModel) apply(c Command) Result {
	index := -1
	for i, item := range *r {
		if item.Key == c.Key {
			index = i
			break
		}
	}
	exists := index >= 0
	switch c.Operation {
	case Get:
		if !exists {
			return Result{Code: KeyNotFound}
		}
		return Result{Code: OK, Exists: true, Value: []byte((*r)[index].Value)}
	case Delete:
		if exists {
			*r = append((*r)[:index], (*r)[index+1:]...)
		}
		return Result{Code: OK, Deleted: exists}
	case CAS:
		if exists != c.Expected.Exists || exists && (*r)[index].Value != string(c.Expected.Value) {
			return Result{Code: CompareFailed}
		}
	}
	if exists {
		(*r)[index].Value = string(c.Value)
	} else {
		*r = append(*r, entryView{c.Key, string(c.Value)})
	}
	return Result{Code: OK, Applied: true}
}

func (r referenceModel) view() stateView {
	entries := append(make([]entryView, 0, len(r)), r...)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Key < entries[j].Key })
	var total uint64
	for _, item := range entries {
		total += uint64(len(item.Key) + len(item.Value))
	}
	return stateView{entries, total}
}

func TestT008DeterministicReplayAgainstModel(t *testing.T) {
	const seed = 19
	rng := rand.New(rand.NewSource(seed))
	operations := []Operation{Put, Get, Delete, CAS}
	values := [][]byte{nil, {}, []byte("v0"), []byte("v1"), {0, 255, 128}}
	machines := []*Machine{New(), New(), {}} // Include the usable zero value.
	var model referenceModel
	for step := 0; step < 128; step++ {
		c := Command{Operation: operations[rng.Intn(len(operations))], Key: fmt.Sprintf("k%d", rng.Intn(8))}
		if c.Operation == Put || c.Operation == CAS {
			c.Value = values[rng.Intn(len(values))]
		}
		if c.Operation == CAS {
			c.Expected = &Expectation{Exists: rng.Intn(2) == 1}
			if c.Expected.Exists {
				c.Expected.Value = values[rng.Intn(len(values))]
			}
		}
		want := model.apply(c)
		for i, machine := range machines {
			got := machine.Apply(c)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("seed=%d step=%d instance=%d command=%#v: got %#v want %#v", seed, step, i, c, got, want)
			}
			if !reflect.DeepEqual(view(machine), model.view()) {
				t.Fatalf("seed=%d step=%d instance=%d: state differs from independent model", seed, step, i)
			}
			assertInvariants(t, machine)
		}
	}
}

func TestT008RepeatedSequenceAndIndependence(t *testing.T) {
	commands := []Command{
		{Operation: Put, Key: "a", Value: []byte("first")},
		{Operation: Put, Key: "a", Value: []byte("first")},
		{Operation: CAS, Key: "a", Expected: &Expectation{true, []byte("first")}, Value: []byte("second")},
		{Operation: CAS, Key: "a", Expected: &Expectation{true, []byte("first")}, Value: []byte("third")},
		{Operation: Get, Key: "a"},
		{Operation: Delete, Key: "a"},
		{Operation: Delete, Key: "a"},
		{Operation: Get, Key: "a"},
	}
	var original []Result
	for run := 0; run < 3; run++ {
		m := New()
		var results []Result
		for _, c := range commands {
			results = append(results, m.Apply(c))
			assertInvariants(t, m)
		}
		if run == 0 {
			original = results
		} else if !reflect.DeepEqual(results, original) {
			t.Fatalf("replay %d differs", run)
		}
		if len(m.values) != 0 || m.logicalBytes != 0 {
			t.Fatal("sequence must end in the empty state")
		}
	}

	a, b := New(), New()
	a.Apply(Command{Operation: Put, Key: "only-a", Value: []byte("a")})
	assertResult(t, b.Apply(Command{Operation: Get, Key: "only-a"}), Result{Code: KeyNotFound})
	b.Apply(Command{Operation: Put, Key: "only-a", Value: []byte("b")})
	b.Apply(Command{Operation: Delete, Key: "only-a"})
	assertResult(t, a.Apply(Command{Operation: Get, Key: "only-a"}), Result{Code: OK, Exists: true, Value: []byte("a")})
	assertInvariants(t, a)
	assertInvariants(t, b)
}
