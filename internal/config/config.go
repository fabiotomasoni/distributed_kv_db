// Package config validates immutable three-member local configuration and derives
// the canonical shared configuration hash. It opens no listener or storage.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

const MaxBytes = 16 * 1024

var ErrConfig = errors.New("config: invalid static local configuration")

type Member struct {
	Client string `json:"client"`
	ID     string `json:"id"`
	Peer   string `json:"peer"`
}
type Config struct {
	cluster, node, dir, hash            string
	members                             [3]Member
	heartbeat, minElection, maxElection uint64
}

func (c Config) Valid() bool        { return c.hash != "" }
func (c Config) ClusterID() string  { return c.cluster }
func (c Config) NodeID() string     { return c.node }
func (c Config) DataDir() string    { return c.dir }
func (c Config) Hash() string       { return c.hash }
func (c Config) Members() [3]Member { return c.members }
func (c Config) Member(id string) (Member, bool) {
	for _, m := range c.members {
		if m.ID == id && c.Valid() {
			return m, true
		}
	}
	return Member{}, false
}
func (c Config) Local() Member                    { m, _ := c.Member(c.node); return m }
func (c Config) Timing() (uint64, uint64, uint64) { return c.heartbeat, c.minElection, c.maxElection }

// Address accepts canonical explicit loopback IPs and nonzero decimal ports.
// No DNS lookup, public bind, wildcard, zone, URI or implicit proxy is allowed.
func Address(s string) bool {
	h, p, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	a, err := netip.ParseAddr(h)
	if err != nil || !a.IsLoopback() || a.Zone() != "" || a.String() != h {
		return false
	}
	n, err := strconv.ParseUint(p, 10, 16)
	return err == nil && n > 0 && strconv.FormatUint(n, 10) == p && net.JoinHostPort(h, p) == s
}

func Parse(raw []byte) (Config, error) {
	bad := func() (Config, error) { return Config{}, ErrConfig }
	o, err := strictjson.Decode(raw, MaxBytes)
	if err != nil {
		return bad()
	}
	o, err = strictjson.Fields(o, "config_version cluster_id node_id client_addr peer_addr data_dir members max_logical_bytes max_keys max_sessions heartbeat_ms election_min_ms election_max_ms", "")
	if err != nil {
		return bad()
	}
	ver, err := strictjson.Integer(o["config_version"])
	if err != nil || ver != 1 || !strictjson.Hex(o["cluster_id"], 32) {
		return bad()
	}
	c := Config{}
	c.cluster = o["cluster_id"].(string)
	c.node, _ = o["node_id"].(string)
	c.dir, _ = o["data_dir"].(string)
	if c.dir == "" || c.dir == "." || filepath.Clean(c.dir) != c.dir || strings.ContainsRune(c.dir, 0) {
		return bad()
	}
	for _, p := range strings.Split(filepath.ToSlash(c.dir), "/") {
		if p == ".." {
			return bad()
		}
	}
	for key, want := range map[string]uint64{"max_logical_bytes": 4194304, "max_keys": 10000, "max_sessions": 1024} {
		v, err := strictjson.Integer(o[key])
		if err != nil || v != want {
			return bad()
		}
	}
	a, ok := o["members"].([]any)
	if !ok || len(a) != 3 {
		return bad()
	}
	members := make([]Member, 3)
	seen := map[string]bool{}
	addresses := map[string]bool{}
	for i, v := range a {
		m, err := strictjson.Fields(v, "id client peer", "")
		if err != nil {
			return bad()
		}
		id, _ := m["id"].(string)
		client, _ := m["client"].(string)
		peer, _ := m["peer"].(string)
		if !Address(client) || !Address(peer) {
			return bad()
		}
		endpoint := func(s string) string {
			h, p, _ := net.SplitHostPort(s)
			a, _ := netip.ParseAddr(h)
			return net.JoinHostPort(a.Unmap().String(), p)
		}
		ca, pa := endpoint(client), endpoint(peer)
		if (id != "n1" && id != "n2" && id != "n3") || seen[id] || ca == pa || addresses[ca] || addresses[pa] {
			return bad()
		}
		seen[id] = true
		addresses[ca] = true
		addresses[pa] = true
		members[i] = Member{ID: id, Client: client, Peer: peer}
	}
	sort.Slice(members, func(i, j int) bool { return members[i].ID < members[j].ID })
	copy(c.members[:], members)
	if !seen[c.node] {
		return bad()
	}
	local, _ := func() (Member, bool) {
		for _, m := range members {
			if m.ID == c.node {
				return m, true
			}
		}
		return Member{}, false
	}()
	if o["client_addr"] != local.Client || o["peer_addr"] != local.Peer {
		return bad()
	}
	c.heartbeat, err = strictjson.Integer(o["heartbeat_ms"])
	if err != nil {
		return bad()
	}
	c.minElection, err = strictjson.Integer(o["election_min_ms"])
	if err != nil {
		return bad()
	}
	c.maxElection, err = strictjson.Integer(o["election_max_ms"])
	if err != nil {
		return bad()
	}
	if c.heartbeat == 0 || c.minElection <= c.heartbeat || c.maxElection < c.minElection || c.maxElection > 2147483647 {
		return bad()
	}
	shared, _ := json.Marshal(map[string]any{"cluster_id": c.cluster, "members": members, "max_logical_bytes": 4194304, "max_keys": 10000, "max_sessions": 1024})
	h := sha256.Sum256(shared)
	c.hash = hex.EncodeToString(h[:])
	return c, nil
}

// SharedBytes is the exact hash preimage: sorted JSON keys, member IDs sorted,
// shared addresses/quotas included; local identity/binds/path/timing excluded.
func (c Config) SharedBytes() []byte {
	if !c.Valid() {
		return nil
	}
	b, _ := json.Marshal(map[string]any{"cluster_id": c.cluster, "members": c.members, "max_logical_bytes": 4194304, "max_keys": 10000, "max_sessions": 1024})
	return b
}
