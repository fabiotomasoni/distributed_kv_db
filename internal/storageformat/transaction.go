package storageformat

import (
	"bytes"
	"encoding/json"
)

// HardState uses an empty Vote for the format's null vote. These types are
// decoded views, not an alternative persistence format.
type HardState struct {
	Term uint64
	Vote string
}
type LogEntry struct {
	Index, Term uint64
	Raw         []byte
}
type Suffix struct {
	From    uint64
	Entries []LogEntry
}
type Transaction struct {
	HardState *HardState
	Replace   *Suffix
	Commit    *uint64
}

// DecodeTransaction reuses the canonical schema validator. Returned entry bytes
// are owned copies. State-dependent validation belongs to the WAL owner.
func DecodeTransaction(raw []byte) (Transaction, error) {
	if err := transaction(raw); err != nil {
		return Transaction{}, err
	}
	o, err := object(raw, MaxPayload)
	if err != nil {
		return Transaction{}, err
	}
	var tx Transaction
	if v, ok := o["hard_state"]; ok {
		h := v.(map[string]any)
		term, _ := uintString(h["current_term"])
		vote, _ := h["voted_for"].(string)
		tx.HardState = &HardState{term, vote}
	}
	if v, ok := o["commit_index"]; ok {
		n, _ := uintString(v)
		tx.Commit = &n
	}
	if v, ok := o["replace_suffix"]; ok {
		r := v.(map[string]any)
		from, _ := uintString(r["from_index"])
		s := &Suffix{From: from}
		for _, v := range r["entries"].([]any) {
			e := v.(map[string]any)
			i, _ := uintString(e["index"])
			t, _ := uintString(e["term"])
			b, _ := json.Marshal(e)
			s.Entries = append(s.Entries, LogEntry{i, t, bytes.Clone(b)})
		}
		tx.Replace = s
	}
	return tx, nil
}
