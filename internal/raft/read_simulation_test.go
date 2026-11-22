package raft

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

// A small explicit-event three-node run: no sockets or sleeps, bounded seeded
// message ordering, real core/WAL adapter/M02 work, permanent committed ghost.
func readScenario(t *testing.T, seed int64) []string {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	now := uint64(600)
	ss := map[string]*Service{}
	disks := map[string]*memoryStorage{}
	queue := []packet{}
	isolated := ""
	trace := []string{}
	ghost := map[uint64][]byte{}
	create := func(id string) {
		clock := newClock()
		if id != "n1" {
			clock.now.Store(600)
		}
		s, e := NewService(configuration(t, id), disks[id], Options{Commit: true, Reads: true, Clock: clock, Draw: func(min, max uint64) (uint64, error) { return min + uint64(id[1]-'1')*100, nil }})
		if e != nil {
			t.Fatal(e)
		}
		ss[id] = s
	}
	for _, id := range []string{"n1", "n2", "n3"} {
		disks[id] = &memoryStorage{s: initial()}
		create(id)
	}
	defer func() {
		for _, id := range []string{"n1", "n2", "n3"} {
			_ = ss[id].Shutdown(deadline(t))
		}
	}()
	apply := func(id string) {
		s := ss[id]
		for s.core.state.LastApplied < s.core.state.CommitIndex {
			index := s.core.state.LastApplied + 1
			e := s.core.entry(index)
			if raw, ok := ghost[index]; ok && !bytes.Equal(raw, e.Raw) {
				t.Fatal("committed ghost diverged", id, index)
			}
			ghost[index] = bytes.Clone(e.Raw)
			command, err := s.project(e)
			if err != nil {
				t.Fatal(err)
			}
			f, err := s.application.applier.TrySubmit(applier.Entry{Index: index, Command: command})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.core.Scheduled(index); err != nil {
				t.Fatal(err)
			}
			result, err := f.Wait(deadline(t))
			if err = s.core.Applied(1, index, err); err != nil {
				t.Fatal(err)
			}
			s.application.record(AppliedResult{index, e.Term, result.Result})
			// Independent list/string model reconstructs this slot from durable bytes,
			// without calling the production state machine to decide the read value.
			model := map[string]string{}
			for i := uint64(1); i <= index; i++ {
				o, _ := strictjson.Decode(s.core.entry(i).Raw, 48*1024)
				kind := o["kind"]
				if kind == "noop" {
					continue
				}
				c := o["command"].(map[string]any)
				key := c["key"].(string)
				switch kind {
				case "put":
					v, _ := strictjson.Base64(c["value_b64"], 16384)
					model[key] = string(v)
				case "delete":
					delete(model, key)
				case "read":
					if i == index {
						v, exists := model[key]
						r := result.Result
						if r.Exists != exists || string(r.Value) != v || (exists && r.Code != statemachine.OK) || (!exists && r.Code != statemachine.KeyNotFound) {
							t.Fatal("read oracle at wrong slot", id, index, r)
						}
					}
				}
			}
			trace = append(trace, fmt.Sprintf("apply %s %d %s %s", id, index, result.Result.Code, string(result.Result.Value)))
		}
	}
	drive := func(id string, e Effects, err error, req *peerprotocol.Message) {
		if err != nil {
			t.Fatal(id, err)
		}
		s := ss[id]
		for e.Persist != nil {
			p := e.Persist
			token, err := disks[id].Append(p.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			e, err = s.core.Durable(p.ID, token, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		apply(id)
		offer := func(p packet) {
			env := p.request.Envelope()
			if env.From == isolated || env.To == isolated {
				ss[env.From].core.Failed(p.request)
				return
			}
			queue = append(queue, p)
		}
		for _, m := range e.Send {
			if err := s.core.Sent(m); err != nil {
				t.Fatal(err)
			}
			offer(packet{request: m})
		}
		if e.Reply != nil {
			if req == nil {
				t.Fatal("reply correlation")
			}
			offer(packet{request: *req, response: *e.Reply})
		}
	}
	drain := func() {
		for step := 0; len(queue) > 0; step++ {
			if step > 256 {
				t.Fatal("message bound")
			}
			i := rng.Intn(len(queue))
			p := queue[i]
			queue = append(queue[:i], queue[i+1:]...)
			if len(p.response.Bytes()) == 0 {
				id := p.request.Envelope().To
				e, err := ss[id].core.Request(now, p.request)
				drive(id, e, err, &p.request)
			} else {
				id := p.response.Envelope().To
				e, err := ss[id].core.Response(now, p.request, p.response)
				drive(id, e, err, nil)
			}
		}
	}
	tick := func(duration uint64) {
		end := now + duration
		for now < end {
			now += 25
			for _, id := range []string{"n1", "n2", "n3"} {
				e, err := ss[id].core.Tick(now)
				drive(id, e, err, nil)
			}
			drain()
		}
	}
	e, err := ss["n1"].core.Tick(now)
	drive("n1", e, err, nil)
	drain()
	tick(100)
	if ss["n1"].core.state.Role != Leader {
		t.Fatal("no initial leader")
	}
	submit := func(id, kind string, command json.RawMessage) LocalAppend {
		s := ss[id]
		r := LocalAppend{s.core.state.LastIndex + 1, s.core.state.Term}
		e, err := s.core.AppendLocal(now, kind, command)
		drive(id, e, err, nil)
		drain()
		tick(100)
		return r
	}
	submit("n1", "put", writeCommand())
	prior := submit("n1", "read", json.RawMessage(`{"key":"x"}`))
	r, err := ss["n1"].WaitApplied(deadline(t), prior)
	if err != nil || string(r.Result.Value) != "x" {
		t.Fatal(r, err)
	}
	// Establish full isolation and discard all in-flight traffic BEFORE the new
	// old-leader invocation; never let a pre-partition quorum mask stale reads.
	isolated = "n1"
	for _, p := range queue {
		ss[p.request.Envelope().From].core.Failed(p.request)
	}
	queue = nil
	tick(1200)
	if ss["n2"].core.state.Role != Leader {
		t.Fatal("majority did not elect successor", ss["n2"].core.Status())
	}
	submit("n2", "put", json.RawMessage(`{"key":"x","value_b64":"eQ==","client_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","sequence":2}`))
	majority := submit("n2", "read", json.RawMessage(`{"key":"x"}`))
	got, err := ss["n2"].WaitApplied(deadline(t), majority)
	if err != nil || string(got.Result.Value) != "y" {
		t.Fatal("majority read", got, err)
	}
	old := submit("n1", "read", json.RawMessage(`{"key":"x"}`))
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ss["n1"].WaitApplied(canceled, old); err == nil {
		t.Fatal("isolated old leader returned cached value")
	}
	if ss["n1"].core.state.CommitIndex >= old.Index || ss["n1"].core.state.LastApplied >= old.Index {
		t.Fatal("speculative old-leader read applied")
	}
	fence := writeFence{old, make(chan error, 1)}
	ss["n1"].processFence(fence)
	if <-fence.reply == nil {
		t.Fatal("old leader authorized new read")
	}
	// A read committed before isolation may deliver its earlier captured result.
	fence = writeFence{prior, make(chan error, 1)}
	ss["n1"].processFence(fence)
	if err = <-fence.reply; err != nil {
		t.Fatal("past quorum improperly revoked", err)
	}
	isolated = ""
	tick(600)
	if ss["n1"].core.state.Role != Follower {
		t.Fatal("higher term not observed")
	}
	if _, err = ss["n1"].WaitApplied(deadline(t), old); err == nil {
		t.Fatal("superseded receipt completed")
	}
	// Fresh replay includes existing read entries but never changes their map.
	before := disks["n1"].State().CommitIndex
	if err = ss["n1"].Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	create("n1")
	if ss["n1"].core.state.LastApplied != before {
		t.Fatal("replay cursor")
	}
	got, err = ss["n1"].WaitApplied(deadline(t), majority)
	if err != nil || string(got.Result.Value) != "y" {
		t.Fatal("read replay result", got, err)
	}
	return trace
}
func TestT019SeededIsolationAndLoggedReadHistory(t *testing.T) {
	for _, seed := range []int64{42, 117, 203} {
		a := readScenario(t, seed)
		b := readScenario(t, seed)
		if !reflect.DeepEqual(a, b) {
			t.Fatal("non-reproducible read trace", seed)
		}
	}
}
