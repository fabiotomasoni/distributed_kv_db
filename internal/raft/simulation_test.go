package raft

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/peerprotocol"
	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
	"github.com/fabiotomasoni/distributed_kv_db/internal/wal"
)

type simulatedNode struct {
	core  *Core
	disk  wal.State
	draw  Draw
	boots int
}
type packet struct {
	request, response peerprotocol.Message
	at                uint64
}
type simulation struct {
	t                          *testing.T
	seed                       int64
	rng                        *rand.Rand
	nodes                      map[string]*simulatedNode
	now                        uint64
	queue                      []packet
	trace                      []string
	votes                      map[string]string
	leaders                    map[uint64]string
	faults, crashVote, crashed bool
	heartbeats                 int
	history                    []string
	lastState                  map[string]string
	isolated                   string
}
type electionEvidence struct {
	Seed           int64    `json:"seed"`
	TraceSHA       string   `json:"trace_sha256"`
	Events         int      `json:"events"`
	LeaderTerms    int      `json:"leader_terms"`
	CrashAfterVote bool     `json:"crash_after_vote"`
	History        []string `json:"vote_term_leader_trace"`
}

func newSimulation(t *testing.T, seed int64, faults, crash bool) *simulation {
	t.Helper()
	s := &simulation{t: t, seed: seed, rng: rand.New(rand.NewSource(seed)), nodes: map[string]*simulatedNode{}, votes: map[string]string{}, leaders: map[uint64]string{}, faults: faults, crashVote: crash, lastState: map[string]string{}}
	for i, id := range []string{"n1", "n2", "n3"} {
		r := rand.New(rand.NewSource(seed*37 + int64(i) + 1))
		draw := func(min, max uint64) (uint64, error) { return min + uint64(r.Int63n(int64(max-min+1))), nil }
		c, err := New(configuration(t, id), initial(), "gen-1", fmt.Sprintf("%032x", i+1), draw, 0)
		if err != nil {
			t.Fatal(err)
		}
		s.nodes[id] = &simulatedNode{c, initial(), draw, 0}
	}
	return s
}
func (s *simulation) record(format string, args ...any) {
	s.trace = append(s.trace, fmt.Sprintf("%d "+format, append([]any{s.now}, args...)...))
	if strings.HasPrefix(format, "sync ") || strings.HasPrefix(format, "noop ") || strings.HasPrefix(format, "crash-") || strings.HasPrefix(format, "restart-") {
		s.history = append(s.history, s.trace[len(s.trace)-1])
	}
}
func (s *simulation) enqueue(p packet) {
	if s.isolated != "" && (p.request.Envelope().From == s.isolated || p.request.Envelope().To == s.isolated) {
		return
	}
	if s.faults {
		if s.rng.Intn(10) == 0 {
			s.record("drop %s", p.request.Envelope().RPCID)
			return
		}
		p.at = s.now + uint64(s.rng.Intn(4))*25
	} else {
		p.at = s.now
	}
	s.queue = append(s.queue, p)
	if s.faults && s.rng.Intn(7) == 0 {
		s.queue = append(s.queue, p)
		s.record("duplicate %s", p.request.Envelope().RPCID)
	}
	if len(s.queue) > 256 {
		s.t.Fatal("simulator packet bound exceeded")
	}
}
func (s *simulation) effect(id string, e Effects, err error, req *peerprotocol.Message) {
	s.t.Helper()
	if err != nil {
		s.t.Fatal(id, err)
	}
	n := s.nodes[id]
	for e.Persist != nil {
		p := e.Persist
		tx, err := storageformat.DecodeTransaction(p.Bytes)
		if err != nil {
			s.t.Fatal(err)
		}
		if p.ID != n.disk.Sequence+1 {
			s.t.Fatal("disk sequence")
		}
		n.disk.Sequence = p.ID
		if tx.HardState != nil {
			h := *tx.HardState
			if h.Term < n.disk.HardState.Term {
				s.t.Fatal("term regression")
			}
			n.disk.HardState = h
			if h.Vote != "" {
				key := fmt.Sprintf("%d/%s", h.Term, id)
				if v := s.votes[key]; v != "" && v != h.Vote {
					s.t.Fatal("independent oracle: two votes", key, v, h.Vote)
				}
				s.votes[key] = h.Vote
			}
			s.record("sync %s term=%d vote=%s", id, h.Term, h.Vote)
		}
		if tx.Replace != nil {
			if tx.Replace.From != n.disk.LastIndex()+1 || len(tx.Replace.Entries) != 1 {
				s.t.Fatal("M05 wrote more than a local noop")
			}
			n.disk.Entries = append(n.disk.Entries, tx.Replace.Entries...)
			s.record("noop %s index=%d", id, n.disk.LastIndex())
		}
		if tx.Commit != nil {
			s.t.Fatal("M05 changed commit")
		}
		if s.crashVote && !s.crashed && tx.HardState != nil && tx.HardState.Vote != "" && tx.HardState.Vote != id {
			s.crashed = true
			s.record("crash-after-sync %s", id)
			n.core.Stop()
			n.boots++
			c, err := New(n.core.cfg, n.disk, "gen-1", fmt.Sprintf("%032x", 100+n.boots), n.draw, s.now)
			if err != nil {
				s.t.Fatal(err)
			}
			n.core = c
			if c.Status().ElectionDeadline < s.now+600 || c.Status().ElectionDeadline > s.now+1000 {
				s.t.Fatal("restart reused a time-zero deadline")
			}
			other := "n1"
			if other == id || other == n.disk.HardState.Vote {
				other = "n2"
			}
			if other == id || other == n.disk.HardState.Vote {
				other = "n3"
			}
			probe := voteRequest(s.t, c.cfg, other, n.disk.HardState.Term, n.disk.LastIndex(), c.Status().LastTerm)
			rejected, err := c.Request(s.now, probe)
			if err != nil || rejected.Persist != nil || fields(s.t, rejected.Reply)["vote_granted"] != false {
				s.t.Fatal("second vote after crash")
			}
			s.record("restart-denied %s candidate=%s", id, other)
			return
		}
		e, err = n.core.Durable(p.ID, wal.Token{Generation: "gen-1", Sequence: p.ID}, nil)
		if err != nil {
			s.t.Fatal(err)
		}
	}
	status := n.core.Status()
	s.record("state %s %s term=%d vote=%s", id, status.Role, status.Term, status.Vote)
	stateKey := fmt.Sprintf("%s/%d/%s", status.Role, status.Term, status.Vote)
	if s.lastState[id] != stateKey {
		s.history = append(s.history, s.trace[len(s.trace)-1])
		s.lastState[id] = stateKey
	}
	if status.Role == Leader {
		if previous := s.leaders[status.Term]; previous != "" && previous != id {
			s.t.Fatal("independent Election Safety oracle", status.Term, previous, id)
		}
		count := 0
		for _, voter := range []string{"n1", "n2", "n3"} {
			if s.votes[fmt.Sprintf("%d/%s", status.Term, voter)] == id {
				count++
			}
		}
		if count < 2 {
			s.t.Fatal("leader without persistent majority")
		}
		s.leaders[status.Term] = id
	}
	for _, m := range e.Send {
		if m.Kind() == peerprotocol.Append {
			s.heartbeats++
		}
		s.record("send %s %s to=%s", id, m.Kind(), m.Envelope().To)
		s.enqueue(packet{request: m})
	}
	if e.Reply != nil {
		if req == nil {
			s.t.Fatal("uncorrelated reply")
		}
		body := fields(s.t, e.Reply)
		if body["vote_granted"] == true && s.votes[fmt.Sprintf("%d/%s", e.Reply.Envelope().Term, id)] != req.Envelope().From {
			s.t.Fatal("positive vote before sync")
		}
		s.record("reply %s %s", id, e.Reply.Kind())
		s.enqueue(packet{request: *req, response: *e.Reply})
	}
}
func (s *simulation) tick(now uint64) {
	s.now = now
	for _, id := range []string{"n1", "n2", "n3"} {
		e, err := s.nodes[id].core.Tick(now)
		s.effect(id, e, err, nil)
	}
}
func (s *simulation) deliver() {
	for steps := 0; steps < 256; steps++ {
		var due []int
		for i, p := range s.queue {
			if p.at <= s.now {
				due = append(due, i)
			}
		}
		if len(due) == 0 {
			return
		}
		i := due[0]
		if s.faults {
			i = due[s.rng.Intn(len(due))]
		}
		p := s.queue[i]
		s.queue = append(s.queue[:i], s.queue[i+1:]...)
		if len(p.response.Bytes()) == 0 {
			id := p.request.Envelope().To
			e, err := s.nodes[id].core.Request(s.now, p.request)
			s.effect(id, e, err, &p.request)
		} else {
			id := p.response.Envelope().To
			e, err := s.nodes[id].core.Response(s.now, p.request, p.response)
			s.effect(id, e, err, nil)
		}
	}
	s.t.Fatal("simulator delivery bound exceeded")
}
func runElectionScenario(t *testing.T, seed int64) electionEvidence {
	t.Helper()
	s := newSimulation(t, seed, true, true)
	// All three persist self-votes before any request can be delivered: a forced
	// split vote, independent of scheduling or randomness.
	s.tick(1000)
	for _, id := range []string{"n1", "n2", "n3"} {
		if s.nodes[id].core.Status().Role != Candidate {
			t.Fatal("split setup")
		}
	}
	s.deliver()
	if len(s.leaders) != 0 {
		t.Fatal("split vote elected a leader")
	}
	for now := uint64(1025); now <= 5000; now += 25 {
		s.tick(now)
		s.deliver()
	}
	if !s.crashed || len(s.leaders) == 0 {
		t.Fatal("scenario did not exercise crash/election", seed)
	}
	raw, _ := json.Marshal(s.trace)
	hash := sha256.Sum256(raw)
	return electionEvidence{seed, hex.EncodeToString(hash[:]), len(s.trace), len(s.leaders), s.crashed, s.history}
}
func TestElectionSimulatorTwentySeedsAndReplay(t *testing.T) {
	actual := make([]electionEvidence, 20)
	for seed := range actual {
		a := runElectionScenario(t, int64(seed))
		b := runElectionScenario(t, int64(seed))
		if !reflect.DeepEqual(a, b) {
			t.Fatal("non-reproducible trace", seed, a, b)
		}
		actual[seed] = a
	}
	raw, _ := json.MarshalIndent(actual, "", "  ")
	// Explicit evidence recording is used only when regenerating this fixture.
	// Ordinary tests require all twenty checked trace fingerprints to match.
	if path := os.Getenv("QUORUMKV_RECORD_ELECTIONS"); path != "" {
		if err := os.WriteFile(path, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	expectedRaw, err := os.ReadFile("../../docs/fixtures/election-v1-results.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected []electionEvidence
	if err = json.Unmarshal(expectedRaw, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatal("election fixture trace drift; review before explicit regeneration")
	}
}
func TestHundredFaultFreeHeartbeatsKeepOneLeader(t *testing.T) {
	s := newSimulation(t, 17, false, false)
	leader := ""
	var term uint64
	for now := uint64(25); now <= 3000; now += 25 {
		s.tick(now)
		s.deliver()
		for _, id := range []string{"n1", "n2", "n3"} {
			st := s.nodes[id].core.Status()
			if st.Role == Leader {
				leader = id
				term = st.Term
			}
		}
		if leader != "" {
			break
		}
	}
	if leader == "" {
		t.Fatal("no initial leader")
	}
	before := s.heartbeats
	for round := 0; round < 100; round++ {
		s.tick(s.now + 100)
		s.deliver()
		for _, id := range []string{"n1", "n2", "n3"} {
			st := s.nodes[id].core.Status()
			if st.Term != term || id == leader && st.Role != Leader || id != leader && st.Role != Follower {
				t.Fatal("spurious election", round, id, st)
			}
		}
	}
	if s.heartbeats-before != 200 || len(s.leaders) != 1 {
		t.Fatal("wrong heartbeat/election count", s.heartbeats-before, len(s.leaders))
	}
}

func TestPartitionMajorityElectsAndOldLeaderStepsDown(t *testing.T) {
	s := newSimulation(t, 17, false, false)
	old := ""
	var oldTerm uint64
	for now := uint64(25); now <= 3000; now += 25 {
		s.tick(now)
		s.deliver()
		for _, id := range []string{"n1", "n2", "n3"} {
			st := s.nodes[id].core.Status()
			if st.Role == Leader {
				old = id
				oldTerm = st.Term
			}
		}
		if old != "" {
			break
		}
	}
	if old == "" {
		t.Fatal("no leader")
	}
	s.isolated = old
	s.queue = nil
	end := s.now + 2000
	for now := s.now + 25; now <= end; now += 25 {
		s.tick(now)
		s.deliver()
	}
	newLeader := ""
	for _, id := range []string{"n1", "n2", "n3"} {
		st := s.nodes[id].core.Status()
		if id != old && st.Role == Leader && st.Term > oldTerm {
			newLeader = id
		}
	}
	if newLeader == "" || s.nodes[old].core.Status().Role != Leader {
		t.Fatal("partition did not leave distinct-term local beliefs")
	}
	s.isolated = ""
	end = s.now + 200
	for now := s.now + 25; now <= end; now += 25 {
		s.tick(now)
		s.deliver()
	}
	if s.nodes[old].core.Status().Role != Follower || s.nodes[old].core.Status().Term <= oldTerm {
		t.Fatal("old leader ignored higher term")
	}
}
