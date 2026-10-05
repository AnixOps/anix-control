package relayctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// Error codes of the control API.
const (
	CodeInvalid  = "invalid"
	CodeConflict = "conflict"
	CodeNotFound = "not_found"
	CodeInternal = "internal"
)

// Error is a refusal by the relay.
type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return "anixops-relay: " + e.Code + ": " + e.Message }

// Status is what the relay runs.
type Status struct {
	// Instance is a random id made at start: the relay's identity for
	// counter epochs.
	Instance string `json:"instance"`
	// Digest is the digest of the configuration the relay runs ("" before
	// the first one) and Applies counts the configurations it applied.
	Digest  string      `json:"digest,omitempty"`
	Applies uint64      `json:"applies"`
	Hops    []HopStatus `json:"hops,omitempty"`
}

// HopStatus says whether a hop's listeners are bound.
type HopStatus struct {
	Route     string `json:"route"`
	Hop       uint32 `json:"hop"`
	Listening bool   `json:"listening"`
}

// Observation is what Observe reads.
type Observation struct {
	Instance string   `json:"instance"`
	Digest   string   `json:"digest,omitempty"`
	Hops     []HopObs `json:"hops,omitempty"`
	// Retired are the final counters of the epochs that ended since the last
	// Observe that drained them (a hop whose listener changed or that was
	// removed).
	Retired []HopObs `json:"retired,omitempty"`
}

// HopObs is one hop's counters, rotation and upstream health.
type HopObs struct {
	Route string `json:"route"`
	Hop   uint32 `json:"hop"`
	Epoch string `json:"epoch"`
	// Up is client to target, Down the way back, in payload bytes; packets
	// count UDP datagrams.
	UpBytes     uint64 `json:"up_bytes"`
	DownBytes   uint64 `json:"down_bytes"`
	UpPackets   uint64 `json:"up_packets"`
	DownPackets uint64 `json:"down_packets"`
	Active      uint32 `json:"active"`
	Total       uint64 `json:"total"`
	// Rotation is the upstreams in rotation now.
	Rotation []RotationEntry `json:"rotation,omitempty"`
	Health   []UpstreamState `json:"health,omitempty"`
}

// RotationEntry is one upstream of a hop in rotation.
type RotationEntry struct {
	Address string `json:"address"`
	Port    uint32 `json:"port"`
	Weight  uint32 `json:"weight"`
}

// UpstreamState is the relay's own view of one upstream: consecutive
// failures and, when the breaker is open, until when.
type UpstreamState struct {
	Address             string `json:"address"`
	Port                uint32 `json:"port"`
	ConsecutiveFailures uint32 `json:"consecutive_failures"`
	CircuitOpenUntilMs  int64  `json:"circuit_open_until_ms,omitempty"`
	// ActiveStreams is the connections and associations in flight toward it.
	ActiveStreams uint32 `json:"active_streams"`
}

// Rotation is the request that puts a hop's upstreams in rotation; a zero
// weight keeps the rendered one.
type Rotation struct {
	Route  string          `json:"route"`
	Hop    uint32          `json:"hop"`
	Active []RotationEntry `json:"active"`
}

// ApplyReply says whether a configuration changed what the relay runs.
type ApplyReply struct {
	Changed bool   `json:"changed"`
	Digest  string `json:"digest"`
}

// Client talks to a relay over its control socket.
type Client struct {
	// Socket is the path of the relay's control socket.
	Socket string
	// Timeout bounds each call; 5 seconds when zero.
	Timeout time.Duration

	http *http.Client
}

// NewClient returns a client for the socket.
func NewClient(socket string) *Client {
	return &Client{Socket: socket, http: &http.Client{
		Transport: &http.Transport{
			Proxy:             nil,
			DisableKeepAlives: true,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socket)
			},
		},
	}}
}

func (c *Client) call(ctx context.Context, method, path string, in, out any) error {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var body io.Reader
	if raw, ok := in.([]byte); ok {
		body = bytes.NewReader(raw)
	} else if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://relay"+path, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil && !errors.Is(cerr, context.DeadlineExceeded) {
			return cerr
		}
		return fmt.Errorf("relayctl: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("relayctl: %s %s: %w", method, path, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e Error
		if json.Unmarshal(raw, &e) != nil || e.Code == "" {
			e = Error{Code: CodeInternal, Message: fmt.Sprintf("HTTP %d", resp.StatusCode)}
		}
		return &e
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("relayctl: %s %s: %w", method, path, err)
		}
	}
	return nil
}

// Status asks what the relay runs; an error means it does not answer.
func (c *Client) Status(ctx context.Context) (*Status, error) {
	var s Status
	if err := c.call(ctx, http.MethodGet, "/v1/status", nil, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Apply sends a configuration document (the encoded bytes).
func (c *Client) Apply(ctx context.Context, content []byte) (*ApplyReply, error) {
	var r ApplyReply
	if err := c.call(ctx, http.MethodPut, "/v1/config", content, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// Observe reads the relay's counters; drain also takes the retired list.
func (c *Client) Observe(ctx context.Context, drain bool) (*Observation, error) {
	path := "/v1/observe"
	if drain {
		path += "?drain=1"
	}
	var o Observation
	if err := c.call(ctx, http.MethodGet, path, nil, &o); err != nil {
		return nil, err
	}
	return &o, nil
}

// SetRotation puts upstreams of one hop in rotation.
func (c *Client) SetRotation(ctx context.Context, r Rotation) error {
	return c.call(ctx, http.MethodPut, "/v1/rotation", r, nil)
}

// ReloadCredentials makes the relay read the link files again.
func (c *Client) ReloadCredentials(ctx context.Context) error {
	return c.call(ctx, http.MethodPost, "/v1/credentials", nil, nil)
}
