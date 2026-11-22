package clientapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"time"

	"github.com/fabiotomasoni/distributed_kv_db/internal/config"
	"github.com/fabiotomasoni/distributed_kv_db/internal/raft"
	"github.com/fabiotomasoni/distributed_kv_db/internal/strictjson"
)

var ErrResponse = errors.New("clientapi: invalid response or unconfigured hint")

// Client is a bounded single-attempt test client. It neither follows hints nor
// retries an uncertain mutation; client_id/sequence do not yet supply dedup.
type Client struct {
	cfg   config.Config
	http  *http.Client
	pool  *http.Transport
	slots chan struct{}
}

func NewClient(c config.Config) (*Client, error) {
	if !c.Valid() {
		return nil, config.ErrConfig
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 100 * time.Millisecond}).DialContext, DisableCompression: true, ForceAttemptHTTP2: false, MaxConnsPerHost: 2, MaxIdleConns: 3, MaxIdleConnsPerHost: 1, IdleConnTimeout: time.Second, ResponseHeaderTimeout: 5250 * time.Millisecond}
	return &Client{c, &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, tr, make(chan struct{}, 4)}, nil
}
func (c *Client) Close() { c.pool.CloseIdleConnections() }
func (c *Client) Write(ctx context.Context, member, kind string, raw []byte) (Response, int, error) {
	if kind == "cas" && !c.cfg.Sessions() {
		return Response{Outcome: raft.NotAdmitted}, 0, raft.ErrUnsupported
	}
	if kind != "put" && kind != "delete" && kind != "cas" {
		return Response{Outcome: raft.NotAdmitted}, 0, raft.ErrInput
	}
	return c.call(ctx, member, kind, raw)
}

// Get sends one logged-read attempt. It does not retry or sample status locally.
func (c *Client) Get(ctx context.Context, member string, raw []byte) (Response, int, error) {
	return c.call(ctx, member, "get", raw)
}
func (c *Client) call(ctx context.Context, member, kind string, raw []byte) (Response, int, error) {
	no := Response{Outcome: raft.NotAdmitted}
	if ctx == nil {
		return no, 0, raft.ErrInput
	}
	m, ok := c.cfg.Member(member)
	if !ok {
		return no, 0, config.ErrConfig
	}
	_, timeout, code := Decode(kind, raw)
	if code != "" {
		return no, 0, raft.ErrInput
	}
	if err := ctx.Err(); err != nil {
		return no, 0, err
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	default:
		return no, 0, raft.ErrFull
	}
	ctx, cancel := context.WithTimeout(ctx, timeout+250*time.Millisecond)
	defer cancel()
	// Deliberately omit GetBody: no replay of a POST by the transport.
	req, err := http.NewRequestWithContext(ctx, "POST", "http://"+m.Client+"/v1/kv/"+kind, io.NopCloser(bytes.NewReader(raw)))
	if err != nil {
		return no, 0, err
	}
	req.ContentLength = int64(len(raw))
	req.Header.Set("Content-Type", "application/json")
	if c.cfg.Sessions() && kind != "get" {
		req.Header.Set(ApplicationHeader, "2")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return Response{Outcome: raft.Unknown}, 0, err
	}
	defer res.Body.Close()
	if c.cfg.Sessions() && kind != "get" {
		versions := res.Header.Values(ApplicationHeader)
		if len(versions) != 1 || versions[0] != "2" {
			return Response{Outcome: raft.Unknown}, res.StatusCode, ErrResponse
		}
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, MaxBody+1))
	if err != nil {
		return Response{Outcome: raft.Unknown}, res.StatusCode, err
	}
	media, _, e := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if e != nil || media != "application/json" || res.Header.Get("Content-Encoding") != "" {
		return Response{Outcome: raft.Unknown}, res.StatusCode, ErrResponse
	}
	out, err := c.decodeResponse(kind, res.StatusCode, body)
	if err != nil {
		return Response{Outcome: raft.Unknown}, res.StatusCode, err
	}
	return out, res.StatusCode, nil
}
func (c *Client) decodeResponse(kind string, status int, raw []byte) (Response, error) {
	bad := func() (Response, error) { return Response{}, ErrResponse }
	o, err := strictjson.Decode(raw, MaxBody)
	if err != nil {
		return bad()
	}
	if _, err = strictjson.Fields(o, "code request_id outcome", "index applied deleted leader_id leader_url exists value_b64"); err != nil {
		return bad()
	}
	var out Response
	if json.Unmarshal(raw, &out) != nil || out.RequestID == "" || len(out.RequestID) > 128 {
		return bad()
	}
	_, hasExists := o["exists"]
	_, hasValue := o["value_b64"]
	if kind != "get" && (hasExists || hasValue) {
		return bad()
	}
	_, hasApplied := o["applied"]
	_, hasDeleted := o["deleted"]
	_, hasLeader := o["leader_id"]
	_, hasURL := o["leader_url"]
	if hasLeader || hasURL {
		id, ok := o["leader_id"].(string)
		url, urlOK := o["leader_url"].(string)
		if !ok || !urlOK || id == "" || url == "" {
			return bad()
		}
	}
	if out.LeaderID != "" || out.LeaderURL != "" {
		m, ok := c.cfg.Member(out.LeaderID)
		if !ok || out.LeaderURL != "http://"+m.Client || out.Code != "NOT_LEADER" {
			return bad()
		}
	}
	switch out.Outcome {
	case raft.Completed:
		index, err := strictjson.Uint(out.Index)
		if err != nil || index == 0 || out.LeaderID != "" {
			return bad()
		}
		if kind == "get" {
			if hasApplied || hasDeleted || out.Exists == nil {
				return bad()
			}
			if status == 404 && out.Code == "KEY_NOT_FOUND" && !*out.Exists && !hasValue {
				return out, nil
			}
			if status != 200 || out.Code != "OK" || !*out.Exists || out.ValueB64 == nil {
				return bad()
			}
			if _, err := strictjson.Base64(*out.ValueB64, 16384); err != nil {
				return bad()
			}
			return out, nil
		}
		if status == 409 && (out.Code == "SESSION_LIMIT" || out.Code == "REQUEST_CONFLICT" || out.Code == "STALE_SEQUENCE" || out.Code == "SEQUENCE_GAP") {
			if hasApplied || hasDeleted {
				return bad()
			}
			return out, nil
		}
		if status == 409 && out.Code == "COMPARE_FAILED" {
			if kind != "cas" || out.Applied == nil || *out.Applied || hasDeleted {
				return bad()
			}
			return out, nil
		}
		if out.Code == "RESOURCE_LIMIT" && status == 409 {
			if hasDeleted || (hasApplied && (out.Applied == nil || *out.Applied)) {
				return bad()
			}
			return out, nil
		}
		if status != 200 || out.Code != "OK" {
			return bad()
		}
		if kind == "put" || kind == "cas" {
			if out.Applied == nil || !*out.Applied || hasDeleted {
				return bad()
			}
		} else if out.Deleted == nil || hasApplied {
			return bad()
		}
	case raft.Unknown, raft.NotAdmitted:
		if hasExists || hasValue {
			return bad()
		}
		if _, ok := o["index"]; ok {
			return bad()
		}
		if _, ok := o["applied"]; ok {
			return bad()
		}
		if _, ok := o["deleted"]; ok {
			return bad()
		}
		expected := map[string]int{"INVALID_ARGUMENT": 400, "INVALID_SEQUENCE": 400, "REQUEST_TOO_LARGE": 413, "NOT_LEADER": 503, "NO_LEADER": 503, "OVERLOADED": 429, "TIMEOUT": 504, "UNAVAILABLE": 503, "STORAGE_UNAVAILABLE": 507}
		if expected[out.Code] != status {
			return bad()
		}
		if out.Outcome == raft.Unknown {
			if out.Code != "TIMEOUT" && out.Code != "UNAVAILABLE" && out.Code != "STORAGE_UNAVAILABLE" {
				return bad()
			}
		} else if out.Code == "TIMEOUT" {
			return bad()
		}
		if out.Code == "NOT_LEADER" && out.LeaderID == "" {
			return bad()
		}
	default:
		return bad()
	}
	return out, nil
}

// CAS sends one mutation attempt. Callers keep client_id, sequence and payload
// unchanged for a manual retry, and advance sequence only for a new operation.
func (c *Client) CAS(ctx context.Context, member string, raw []byte) (Response, int, error) {
	return c.Write(ctx, member, "cas", raw)
}
