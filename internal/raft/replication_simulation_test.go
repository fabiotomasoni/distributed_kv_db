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

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type replicationEvidence struct {
	Seed        int64  `json:"seed"`
	TraceSHA    string `json:"trace_sha256"`
	Events      int    `json:"events"`
	LastIndex   uint64 `json:"last_index"`
	CommitIndex uint64 `json:"commit_index"`
	Restarted   bool   `json:"restarted_after_suffix_sync"`
}

func replicationScenario(t *testing.T, seed int64) replicationEvidence {
	t.Helper()
	rng := rand.New(rand.NewSource(seed))
	cores := map[string]*Core{}
	disks := map[string]wal.State{}
	trace := []string{}
	now := uint64(600)
	crashed := false
	faults := true
	queue := []packet{}
	for i, id := range []string{"n1", "n2", "n3"} {
		terms := [][]uint64{{1, 2, 2}, {1, 4, 4, 4}, {1}}[i]
		s := logState(terms, 1)
		s.Sequence = 2
		disks[id] = s
		start := uint64(0)
		if id != "n1" {
			start = 600
		}
		c, err := New(configuration(t, id), s, "gen-1", fmt.Sprintf("%032x", i+1), minimum, start)
		if err != nil {
			t.Fatal(err)
		}
		if err = c.EnableReplication(2); err != nil {
			t.Fatal(err)
		}
		cores[id] = c
	}
	// n3 is a legitimate up-to-date voter; n2's divergent uncommitted suffix
	// cannot prevent the other two members forming the election majority.
	e, err := cores["n1"].Tick(600)
	if err != nil {
		t.Fatal(err)
	}
	e = complete(t, cores["n1"], e)
	// The self-vote completion above is independently covered by M05. Seed its
	// resulting persistent state here before driving only M06 effects.
	ds := disks["n1"]
	ds.HardState = storageformat.HardState{Term: 6, Vote: "n1"}
	ds.Sequence = cores["n1"].sequence
	disks["n1"] = ds
	var drive func(string, Effects, error, *peerprotocol.Message)
	enqueue := func(p packet) {
		if faults && rng.Intn(6) == 0 {
			cores[p.request.Envelope().From].Failed(p.request)
			trace = append(trace, "drop "+p.request.Envelope().RPCID)
			return
		}
		p.at = now
		if faults {
			p.at += uint64(rng.Intn(4)) * 25
		}
		queue = append(queue, p)
		if faults && rng.Intn(5) == 0 {
			queue = append(queue, p)
		}
	}
	drive = func(id string, e Effects, err error, req *peerprotocol.Message) {
		if err != nil {
			t.Fatal(id, err)
		}
		c := cores[id]
		for e.Persist != nil {
			p := e.Persist
			tx, err := storageformat.DecodeTransaction(p.Bytes)
			if err != nil {
				t.Fatal(err)
			}
			s := disks[id]
			if p.ID != s.Sequence+1 {
				t.Fatal("physical sequence drift")
			}
			s.Sequence = p.ID
			if tx.HardState != nil {
				if tx.HardState.Term < s.HardState.Term {
					t.Fatal("term regression")
				}
				s.HardState = *tx.HardState
			}
			if tx.Commit != nil {
				t.Fatal("M06 wrote a commit")
			}
			if tx.Replace != nil {
				if tx.Replace.From <= s.CommitIndex {
					t.Fatal("committed replacement")
				}
				if id == "n1" && tx.Replace.From != s.LastIndex()+1 {
					t.Fatal("leader not append-only")
				}
				cut := int(tx.Replace.From - s.BaseIndex - 1)
				s.Entries = append(s.Entries[:cut:cut], tx.Replace.Entries...)
			}
			disks[id] = s
			trace = append(trace, fmt.Sprintf("%d sync %s seq=%d last=%d", now, id, s.Sequence, s.LastIndex()))
			if id == "n2" && !crashed && tx.Replace != nil {
				crashed = true
				c.Stop()
				recovered, err := New(c.cfg, s, "gen-1", "cccccccccccccccccccccccccccccccc", minimum, now)
				if err != nil {
					t.Fatal(err)
				}
				if err = recovered.EnableReplication(2); err != nil {
					t.Fatal(err)
				}
				cores[id] = recovered
				if req != nil {
					cores[req.Envelope().From].Failed(*req)
				}
				trace = append(trace, "crash-restart n2 before ack")
				return
			}
			e, err = c.Durable(p.ID, wal.Token{Generation: "gen-1", Sequence: p.ID}, nil)
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, m := range e.Send {
			if err = c.Sent(m); err != nil {
				t.Fatal(err)
			}
			enqueue(packet{request: m})
		}
		if e.Reply != nil {
			if req == nil {
				t.Fatal("missing request")
			}
			enqueue(packet{request: *req, response: *e.Reply})
		}
	}
	var vote peerprotocol.Message
	for _, m := range e.Send {
		if m.Envelope().To == "n3" {
			vote = m
		}
	}
	ve, err := cores["n3"].Request(600, vote)
	if err != nil {
		t.Fatal(err)
	}
	if ve.Persist == nil {
		t.Fatal("vote not persistent")
	}
	tx, _ := storageformat.DecodeTransaction(ve.Persist.Bytes)
	ds = disks["n3"]
	ds.HardState = *tx.HardState
	ds.Sequence = ve.Persist.ID
	disks["n3"] = ds
	ve = complete(t, cores["n3"], ve)
	e, err = cores["n1"].Response(601, vote, *ve.Reply)
	drive("n1", e, err, nil)
	for i := 0; i < 2; i++ {
		e, err = cores["n1"].AppendLocal(602+uint64(i), "read", json.RawMessage(`{"key":"k"}`))
		drive("n1", e, err, nil)
	}
	oracle := func() {
		for _, a := range []string{"n1", "n2", "n3"} {
			s := disks[a]
			if s.CommitIndex != 1 || !bytes.Equal(s.Entries[0].Raw, logEntry(1, 1).Raw) {
				t.Fatal("known commit changed")
			}
			for _, b := range []string{"n1", "n2", "n3"} {
				other := disks[b]
				for i := 0; i < len(s.Entries) && i < len(other.Entries); i++ {
					if s.Entries[i].Term == other.Entries[i].Term {
						for k := 0; k <= i; k++ {
							if !bytes.Equal(s.Entries[k].Raw, other.Entries[k].Raw) {
								t.Fatal("independent Log Matching oracle", a, b, i, k)
							}
						}
					}
				}
			}
		}
	}
	for now = 625; now <= 2600; now += 25 {
		if now >= 1000 {
			faults = false
		}
		for _, id := range []string{"n1", "n2", "n3"} {
			e, err = cores[id].Tick(now)
			drive(id, e, err, nil)
		}
		for steps := 0; steps < 128; steps++ {
			var ready []int
			for i, p := range queue {
				if p.at <= now {
					ready = append(ready, i)
				}
			}
			if len(ready) == 0 {
				break
			}
			i := ready[rng.Intn(len(ready))]
			p := queue[i]
			queue = append(queue[:i], queue[i+1:]...)
			if len(p.response.Bytes()) == 0 {
				id := p.request.Envelope().To
				e, err = cores[id].Request(now, p.request)
				drive(id, e, err, &p.request)
			} else {
				id := p.response.Envelope().To
				e, err = cores[id].Response(now, p.request, p.response)
				drive(id, e, err, nil)
			}
			oracle()
		}
		if len(queue) > 256 {
			t.Fatal("packet bound")
		}
		s := cores["n1"].Status()
		trace = append(trace, fmt.Sprintf("%d progress %v", now, s.Progress))
	}
	if !crashed {
		t.Fatal("no crash exercised")
	}
	last := disks["n1"].LastIndex()
	for _, id := range []string{"n2", "n3"} {
		if disks[id].LastIndex() != last || cores["n1"].progress[id].MatchIndex != last {
			t.Fatal("no convergence", seed, id, cores["n1"].progress[id])
		}
	}
	raw, _ := json.Marshal(trace)
	hash := sha256.Sum256(raw)
	return replicationEvidence{seed, hex.EncodeToString(hash[:]), len(trace), last, 1, crashed}
}
func TestReplicationFaultTracesReplayAndLogMatching(t *testing.T) {
	var actual []replicationEvidence
	for seed := int64(0); seed < 5; seed++ {
		a, b := replicationScenario(t, seed), replicationScenario(t, seed)
		if a != b {
			t.Fatal("non-reproducible replication", seed)
		}
		actual = append(actual, a)
	}
	raw, _ := json.MarshalIndent(actual, "", "  ")
	if path := os.Getenv("QUORUMKV_RECORD_REPLICATION"); path != "" {
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	expectedRaw, err := os.ReadFile("../../docs/fixtures/replication-v1-results.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected []replicationEvidence
	if err = json.Unmarshal(expectedRaw, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, actual) {
		t.Fatal("replication trace fixture drift")
	}
}
