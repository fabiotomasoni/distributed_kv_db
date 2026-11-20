package wal

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/fabiotomasoni/distributed_kv_db/internal/storageformat"
)

type fixtureData struct {
	WAL []struct {
		Hex string `json:"frame_hex"`
	}
	Commands []struct {
		Raw string `json:"entry_json"`
	}
	Snapshot struct {
		Hex string `json:"image_hex"`
	}
	Identity string `json:"identity_json"`
	Current  string `json:"current_json"`
	Recovery []struct {
		ID        string
		Hex       string `json:"wal_hex"`
		Expect    string
		Namespace []string
	} `json:"recovery_cases"`
}

func fixtures(t *testing.T) fixtureData {
	t.Helper()
	b, err := os.ReadFile("../../docs/fixtures/canonical-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f fixtureData
	if err = json.Unmarshal(b, &f); err != nil {
		t.Fatal(err)
	}
	return f
}
func rawHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func payload(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func hard(term uint64, vote any) map[string]any {
	return map[string]any{"current_term": fmt.Sprint(term), "voted_for": vote}
}
func command(t *testing.T, index, term uint64) map[string]any {
	t.Helper()
	var e map[string]any
	if err := json.Unmarshal([]byte(fixtures(t).Commands[0].Raw), &e); err != nil {
		t.Fatal(err)
	}
	e["index"] = fmt.Sprint(index)
	e["term"] = fmt.Sprint(term)
	return e
}
func appendTx(t *testing.T, term, index uint64, commit bool) []byte {
	e := command(t, index, term)
	tx := map[string]any{"hard_state": hard(term, "n1"), "replace_suffix": map[string]any{"from_index": fmt.Sprint(index), "entries": []any{e}}}
	if commit {
		tx["commit_index"] = fmt.Sprint(index)
	}
	return payload(t, tx)
}

func TestCanonicalRecoveryFixtures(t *testing.T) {
	for _, c := range fixtures(t).Recovery {
		if len(c.Namespace) > 0 {
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			r, err := Replay(bytes.NewReader(rawHex(t, c.Hex)), 0, 0)
			switch {
			case strings.HasPrefix(c.Expect, "fatal:"):
				if err == nil || r.State.Sequence != 0 {
					t.Fatal("accepted invalid persistent image", err)
				}
			case strings.HasPrefix(c.Expect, "truncate-and-sync:"):
				want, _ := strconv.ParseInt(strings.TrimPrefix(c.Expect, "truncate-and-sync:"), 10, 64)
				if err != nil || r.ValidBytes != want || r.TailBytes == 0 {
					t.Fatal(r, err)
				}
			default:
				want := uint64(1)
				if c.ID == "bootstrap" {
					want = 0
				}
				if err != nil || r.State.CommitIndex != want || r.State.LastIndex() != want {
					t.Fatal(r, err)
				}
			}
		})
	}
}

func TestEveryFinalFrameCutAndNoMagicSearch(t *testing.T) {
	f := fixtures(t)
	initial := rawHex(t, f.WAL[0].Hex)
	last := rawHex(t, f.WAL[1].Hex)
	for n := 0; n < len(last); n++ {
		b := append(bytes.Clone(initial), last[:n]...)
		r, err := Replay(bytes.NewReader(b), 0, 0)
		if err != nil || r.State.Sequence != 1 || r.ValidBytes != int64(len(initial)) || r.TailBytes != int64(n) {
			t.Fatalf("cut %d: %+v %v", n, r, err)
		}
	}
	for _, tail := range [][]byte{[]byte("garbage"), {'Q', 'W', 'A', 'L', 0, 2}, append([]byte("bad header"), last...)} {
		if _, err := Replay(bytes.NewReader(append(bytes.Clone(initial), tail...)), 0, 0); err == nil {
			t.Fatal("bad header treated as repairable tail")
		}
	}
	if _, err := Replay(bytes.NewReader(initial[:10]), 0, 0); err == nil {
		t.Fatal("incomplete initial frame accepted")
	}
}

func TestStateTransitionsCommitAndSuffix(t *testing.T) {
	initial, _ := storageformat.EncodeFrame(1, []byte(initialTransaction))
	a, _ := storageformat.EncodeFrame(2, appendTx(t, 1, 1, true))
	b, _ := storageformat.EncodeFrame(3, appendTx(t, 2, 2, false))
	r, err := Replay(bytes.NewReader(bytes.Join([][]byte{initial, a, b}, nil)), 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.State.CommitIndex != 1 || r.State.LastIndex() != 2 || len(r.State.CommittedEntries()) != 1 {
		t.Fatal("uncommitted suffix became applied/committed", r)
	}
	for _, tx := range []any{
		map[string]any{"hard_state": hard(1, "n1")},
		map[string]any{"hard_state": hard(2, "n2")},
		map[string]any{"hard_state": hard(2, nil)},
		map[string]any{"commit_index": "0"}, map[string]any{"commit_index": "3"},
		map[string]any{"replace_suffix": map[string]any{"from_index": "1", "entries": []any{}}},
		map[string]any{"replace_suffix": map[string]any{"from_index": "4", "entries": []any{}}},
	} {
		frame, err := storageformat.EncodeFrame(4, payload(t, tx))
		if err != nil {
			t.Fatal(err)
		}
		all := bytes.Join([][]byte{initial, a, b, frame}, nil)
		if state, err := Replay(bytes.NewReader(all), 0, 0); err == nil || state.State.Sequence != 0 {
			t.Fatal("non-atomic/invalid replay", tx, err)
		}
	}
	// A higher term can clear the vote and replace only the uncommitted suffix.
	replace := payload(t, map[string]any{"hard_state": hard(3, nil), "replace_suffix": map[string]any{"from_index": "2", "entries": []any{command(t, 2, 3)}}, "commit_index": "2"})
	frame, err := storageformat.EncodeFrame(4, replace)
	if err != nil {
		t.Fatal(err)
	}
	r, err = Replay(bytes.NewReader(bytes.Join([][]byte{initial, a, b, frame}, nil)), 0, 0)
	if err != nil || r.State.CommitIndex != 2 || r.State.HardState.Term != 3 || r.State.Entries[1].Term != 3 || r.State.HardState.Vote != "" {
		t.Fatal(r, err)
	}
}

type brokenReader struct{}

func (brokenReader) Read([]byte) (int, error) { return 0, errors.New("injected read error") }
func TestReadErrorsAndInitialTransaction(t *testing.T) {
	if _, err := Replay(brokenReader{}, 0, 0); err == nil {
		t.Fatal("read error swallowed")
	}
	f := fixtures(t)
	valid := rawHex(t, f.WAL[0].Hex)
	next := rawHex(t, f.WAL[1].Hex)
	if state, err := Replay(io.MultiReader(bytes.NewReader(append(valid, next[:24]...)), brokenReader{}), 0, 0); err == nil || state.State.Sequence != 0 {
		t.Fatal("mid-record I/O failure treated as torn tail")
	}
	for _, tx := range []any{map[string]any{"commit_index": "0"}, map[string]any{"hard_state": hard(0, nil)}, map[string]any{"hard_state": hard(0, nil), "commit_index": "1"}} {
		b, err := storageformat.EncodeFrame(1, payload(t, tx))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Replay(bytes.NewReader(b), 0, 0); err == nil {
			t.Fatal("invalid initialization accepted")
		}
	}
}

func TestIncompleteTailWithWrongSequenceFailsClosed(t *testing.T) {
	initial := rawHex(t, fixtures(t).WAL[0].Hex)
	s, root := newStore(t)
	closeOK(t, s)
	path := filepath.Join(root, "gen-1/wal")
	for _, seq := range []uint64{1, 3} {
		tail, err := storageformat.EncodeFrame(seq, appendTx(t, 1, 1, false))
		if err != nil {
			t.Fatal(err)
		}
		for _, cut := range []int{20, 25} {
			bad := append(bytes.Clone(initial), tail[:cut]...)
			if _, err = Replay(bytes.NewReader(bad), 0, 0); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("incomplete physical sequence %d classified as repairable: %v", seq, err)
			}
			if err = os.WriteFile(path, bad, 0600); err != nil {
				t.Fatal(err)
			}
			if r, err := Open(root, testIdentity); r != nil || !errors.Is(err, ErrCorrupt) {
				t.Fatal("corrupt tail opened", err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, bad) {
				t.Fatal("contradictory tail destructively truncated", err)
			}
		}
	}
}
