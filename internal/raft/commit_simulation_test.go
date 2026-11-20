package raft

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/committed"
	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

type commitEvidence struct {
	Seed               int64  `json:"seed"`
	TraceSHA           string `json:"trace_sha256"`
	StateSHA           string `json:"state_sha256"`
	Events             int    `json:"events"`
	Committed, Applied uint64
	Restarted          bool
}

func commitScenario(t *testing.T, seed int64) commitEvidence {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	services := map[string]*Service{}
	disks := map[string]*memoryStorage{}
	machines := map[string]*statemachine.Machine{}
	queue := []packet{}
	now := uint64(600)
	trace := []string{}
	ghost := map[uint64][]byte{}
	results := map[uint64]statemachine.Code{}
	isolated := ""
	faults := false
	create := func(id string, offset uint64) {
		clock := newClock()
		clock.now.Store(offset)
		machine := statemachine.New()
		s, err := NewService(configuration(t, id), disks[id], Options{Commit: true, Clock: clock, Draw: func(min, max uint64) (uint64, error) { return min + uint64(id[1]-'1')*100, nil }, MachineFactory: func() applier.Machine { return machine }})
		if err != nil {
			t.Fatal(err)
		}
		services[id] = s
		machines[id] = machine
	}
	for _, id := range []string{"n1", "n2", "n3"} {
		disks[id] = &memoryStorage{s: initial()}
		offset := uint64(0)
		if id != "n1" {
			offset = 600
		}
		create(id, offset)
	}
	register := func(id string) {
		state := disks[id].State()
		for index := uint64(1); index <= state.CommitIndex; index++ {
			raw := state.Entries[index-1].Raw
			if before, ok := ghost[index]; ok && !bytes.Equal(before, raw) {
				t.Fatal("independent committed ghost changed", id, index)
			}
			ghost[index] = bytes.Clone(raw)
		}
	}
	apply := func(id string) {
		s := services[id]
		for s.core.state.ScheduledApplyIndex < s.core.state.CommitIndex {
			index := s.core.state.ScheduledApplyIndex + 1
			e := s.core.entry(index)
			if !bytes.Equal(ghost[index], e.Raw) {
				t.Fatal("applied without ghost commit", id, index)
			}
			c, err := committed.Project(e)
			if err != nil {
				t.Fatal(err)
			}
			f, err := s.application.applier.TrySubmit(applier.Entry{Index: index, Command: c})
			if err != nil {
				t.Fatal(err)
			}
			if err = s.core.Scheduled(index); err != nil {
				t.Fatal(err)
			}
			outcome, err := f.Wait(deadline(t))
			if err = s.core.Applied(1, index, err); err != nil {
				t.Fatal(err)
			}
			if old, ok := results[index]; ok && old != outcome.Result.Code {
				t.Fatal("State Machine Safety oracle", id, index)
			}
			results[index] = outcome.Result.Code
			s.application.record(AppliedResult{index, e.Term, outcome.Result})
			trace = append(trace, fmt.Sprintf("%d apply %s %d %s", now, id, index, outcome.Result.Code))
		}
	}
	enqueue := func(p packet) {
		env := p.request.Envelope()
		if env.From == isolated || env.To == isolated || faults && rng.Intn(8) == 0 {
			services[env.From].core.Failed(p.request)
			return
		}
		p.at = now
		if faults {
			p.at += uint64(rng.Intn(4)) * 25
		}
		queue = append(queue, p)
		if faults && rng.Intn(7) == 0 {
			queue = append(queue, p)
		}
	}
	drive := func(id string, e Effects, err error, req *peerprotocol.Message) {
		if err != nil {
			t.Fatal(id, err)
		}
		s := services[id]
		for e.Persist != nil {
			p := e.Persist
			tx, err := storageformat.DecodeTransaction(p.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			before := disks[id].State()
			if tx.Replace != nil && tx.Replace.From <= before.CommitIndex {
				t.Fatal("overwrite committed")
			}
			token, err := disks[id].Append(p.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			register(id)
			if tx.Commit != nil {
				trace = append(trace, fmt.Sprintf("%d commit-sync %s %d", now, id, *tx.Commit))
			}
			e, err = s.core.Durable(p.ID, token, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		apply(id)
		for _, m := range e.Send {
			if err = s.core.Sent(m); err != nil {
				t.Fatal(err)
			}
			enqueue(packet{request: m})
		}
		if e.Reply != nil {
			if req == nil {
				t.Fatal("uncorrelated ack")
			}
			enqueue(packet{request: *req, response: *e.Reply})
		}
	}
	deliver := func() {
		for step := 0; step < 128; step++ {
			var due []int
			for i, p := range queue {
				if p.at <= now {
					due = append(due, i)
				}
			}
			if len(due) == 0 {
				return
			}
			i := due[rng.Intn(len(due))]
			p := queue[i]
			queue = append(queue[:i], queue[i+1:]...)
			if len(p.response.Bytes()) == 0 {
				id := p.request.Envelope().To
				e, err := services[id].core.Request(now, p.request)
				drive(id, e, err, &p.request)
			} else {
				id := p.response.Envelope().To
				e, err := services[id].core.Response(now, p.request, p.response)
				drive(id, e, err, nil)
			}
		}
		t.Fatal("delivery bound")
	}
	advance := func(duration uint64) {
		end := now + duration
		for now += 25; now <= end; now += 25 {
			for _, id := range []string{"n1", "n2", "n3"} {
				e, err := services[id].core.Tick(now)
				drive(id, e, err, nil)
			}
			deliver()
			if len(queue) > 256 {
				t.Fatal("queue bound")
			}
		}
		now = end
	}
	e, err := services["n1"].core.Tick(now)
	drive("n1", e, err, nil)
	deliver()
	advance(300)
	cmd := func(index uint64, kind, key, value string, expected map[string]any) json.RawMessage {
		entry := mutation(index, 1, kind, key, value, expected)
		var v map[string]json.RawMessage
		_ = json.Unmarshal(entry.Raw, &v)
		return v["command"]
	}
	faults = true
	e, err = services["n1"].core.AppendLocal(now, "put", cmd(2, "put", "kept", "A", nil))
	drive("n1", e, err, nil)
	advance(400)
	faults = false
	advance(400)
	for _, id := range []string{"n1", "n2", "n3"} {
		if services[id].core.state.CommitIndex != 2 || services[id].core.state.LastApplied != 2 {
			t.Fatal("initial prefix did not commit", seed, id)
		}
	}
	isolated = "n1"
	queue = nil
	e, err = services["n1"].core.AppendLocal(now, "put", cmd(3, "put", "lost", "B", nil))
	drive("n1", e, err, nil)
	advance(1500)
	if services["n1"].core.state.CommitIndex != 2 || services["n1"].core.state.LastApplied != 2 {
		t.Fatal("isolated leader committed")
	}
	leader := ""
	for _, id := range []string{"n2", "n3"} {
		if services[id].core.state.Role == Leader {
			leader = id
		}
	}
	if leader == "" {
		t.Fatal("majority did not elect")
	}
	if services[leader].core.state.CommitIndex < 3 {
		t.Fatal("new-term noop not committed")
	}
	// Crash old leader before its uncommitted proposal can apply; a fresh model
	// reconstructs the durable prefix, then reconciles after healing.
	if err = services["n1"].Shutdown(deadline(t)); err != nil {
		t.Fatal(err)
	}
	create("n1", now)
	apply("n1")
	isolated = ""
	advance(300)
	expected := map[string]any{"exists": true, "value_b64": "QQ=="}
	e, err = services[leader].core.AppendLocal(now, "cas", cmd(4, "cas", "kept", "C", expected))
	drive(leader, e, err, nil)
	faults = true
	advance(400)
	faults = false
	advance(300)
	e, err = services[leader].core.AppendLocal(now, "cas", cmd(5, "cas", "kept", "wrong", expected))
	drive(leader, e, err, nil)
	advance(300)
	var digest string
	last := services[leader].core.state.CommitIndex
	for _, id := range []string{"n1", "n2", "n3"} {
		s := services[id]
		if s.core.state.CommitIndex != last || s.core.state.LastApplied != last || s.core.state.ScheduledApplyIndex != last {
			t.Fatal("final apply lag", seed, id)
		}
		if err = s.Shutdown(deadline(t)); err != nil {
			t.Fatal(err)
		}
		kept := machines[id].Apply(statemachine.Command{Operation: statemachine.Get, Key: "kept"})
		lost := machines[id].Apply(statemachine.Command{Operation: statemachine.Get, Key: "lost"})
		if string(kept.Value) != "C" || lost.Code != statemachine.KeyNotFound {
			t.Fatal("replica state diverged")
		}
		raw, _ := json.Marshal([]statemachine.Result{kept, lost})
		hash := sha256.Sum256(raw)
		h := hex.EncodeToString(hash[:])
		if digest != "" && digest != h {
			t.Fatal("state digest mismatch")
		}
		digest = h
	}
	if results[5] != statemachine.CompareFailed {
		t.Fatal("negative CAS was not a completed application")
	}
	raw, _ := json.Marshal(trace)
	hash := sha256.Sum256(raw)
	return commitEvidence{seed, hex.EncodeToString(hash[:]), digest, len(trace), last, last, true}
}
func TestCommitPartitionReplayGhostAndThreeStateMachines(t *testing.T) {
	var actual []commitEvidence
	for seed := int64(0); seed < 3; seed++ {
		a, b := commitScenario(t, seed), commitScenario(t, seed)
		if a != b {
			t.Fatal("commit trace not reproducible", seed)
		}
		actual = append(actual, a)
	}
	raw, _ := json.MarshalIndent(actual, "", "  ")
	if path := os.Getenv("QUORUMKV_RECORD_COMMIT"); path != "" {
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	expectedRaw, err := os.ReadFile("../../docs/fixtures/commit-v1-results.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected []commitEvidence
	if err = json.Unmarshal(expectedRaw, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("commit trace drift")
	}
}
