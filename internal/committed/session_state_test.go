package committed

import (
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"strings"
	"testing"
)

func TestSessionStateCopyRestoreAndInvalidUnions(t *testing.T) {
	id := strings.Repeat("a", 32)
	s := NewSessions(statemachine.New())
	entry := sessionEntry(1, id, 1, "put", "x", "A", nil)
	c, e := ProjectWithSessions(entry)
	if e != nil {
		t.Fatal(e)
	}
	s.Apply(c)
	state := s.CopySessions()
	fresh, e := SessionsFromState(statemachine.New(), state)
	if e != nil {
		t.Fatal(e)
	}
	state[0].Result.Code = "BROKEN"
	state[0].Digest[0] ^= 1
	state[0].ClientID = strings.Repeat("b", 32)
	r, index, e := DecodeSessionResult(fresh.Apply(c), 2)
	if e != nil || r.Code != statemachine.OK || index != 1 || s.CopySessions()[0].Result.Code != statemachine.OK {
		t.Fatal("session alias", r, index, e)
	}
	valid := s.CopySessions()[0]
	for _, mutate := range []func(*SessionState){func(v *SessionState) { v.ClientID = "bad" }, func(v *SessionState) { v.Sequence = 0 }, func(v *SessionState) { v.Sequence = 9007199254740992 }, func(v *SessionState) { v.Index = 0 }, func(v *SessionState) { v.Kind = "read" }, func(v *SessionState) { v.Result.Value = []byte{} }, func(v *SessionState) { v.Result.Exists = true }, func(v *SessionState) { v.Result.Code = statemachine.CompareFailed }, func(v *SessionState) { v.Result.Applied = false }, func(v *SessionState) { v.Result.Deleted = true }, func(v *SessionState) {
		v.Kind = "delete"
		v.Result.Code = statemachine.ResourceLimit
		v.Result.Applied = false
	}} {
		invalid := valid
		mutate(&invalid)
		if restored, e := SessionsFromState(statemachine.New(), []SessionState{invalid}); e == nil || restored != nil {
			t.Fatal("invalid union accepted", invalid)
		}
	}
	if _, e = SessionsFromState(nil, nil); e == nil {
		t.Fatal("nil machine")
	}
	if _, e = SessionsFromState(statemachine.New(), []SessionState{valid, valid}); e == nil {
		t.Fatal("duplicate session")
	}
	if _, e = SessionsFromState(statemachine.New(), make([]SessionState, 1025)); e == nil {
		t.Fatal("capacity")
	}
}
