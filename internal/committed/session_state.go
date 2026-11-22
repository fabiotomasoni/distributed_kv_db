package committed

import (
	"errors"
	"github.com/fabiotomasoni/distributed_kv_db/internal/applier"
	"github.com/fabiotomasoni/distributed_kv_db/internal/statemachine"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
	"sort"
)

type SessionState struct {
	ClientID        string
	Sequence, Index uint64
	Digest          [32]byte
	Kind            string
	Result          statemachine.Result
}

var ErrSessionState = errors.New("committed: invalid isolated session state")

// CopySessions requires the same exclusive owner as Apply. No TTL/eviction or
// second mutable session cache is introduced. Results contain no value buffers.
func (s *Sessions) CopySessions() []SessionState {
	out := make([]SessionState, 0, len(s.clients))
	for id, v := range s.clients {
		out = append(out, SessionState{id, v.sequence, v.index, v.digest, v.kind, v.result})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ClientID < out[j].ClientID })
	return out
}

// SessionsFromState constructs a fresh wrapper, preserving original result index.
func SessionsFromState(machine applier.Machine, states []SessionState) (*Sessions, error) {
	if machine == nil || len(states) > MaxSessions {
		return nil, ErrSessionState
	}
	for i, v := range states {
		if !strictjson.Hex(v.ClientID, 32) || v.Sequence == 0 || v.Sequence > 9007199254740991 || v.Index == 0 || i > 0 && states[i-1].ClientID >= v.ClientID {
			return nil, ErrSessionState
		}
		r := v.Result
		if r.Value != nil || r.Exists {
			return nil, ErrSessionState
		}
		switch v.Kind {
		case "put", "cas", "delete":
		default:
			return nil, ErrSessionState
		}
		switch r.Code {
		case statemachine.OK:
			if v.Kind == "delete" {
				if r.Applied {
					return nil, ErrSessionState
				}
			} else if !r.Applied || r.Deleted {
				return nil, ErrSessionState
			}
		case statemachine.CompareFailed:
			if v.Kind != "cas" || r.Applied || r.Deleted {
				return nil, ErrSessionState
			}
		case statemachine.ResourceLimit:
			if v.Kind == "delete" || r.Applied || r.Deleted {
				return nil, ErrSessionState
			}
		default:
			return nil, ErrSessionState
		}
	}
	s := NewSessions(machine)
	for _, v := range states {
		s.clients[v.ClientID] = session{v.Sequence, v.Index, v.Digest, v.Kind, v.Result}
	}
	return s, nil
}
