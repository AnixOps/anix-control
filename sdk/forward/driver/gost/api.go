package gost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// gost's web API (Render's "api" block) is how the driver changes a
// running gost without a reload, and where it reads the services'
// statistics. It listens on Config.apiPath, a unix socket in the runtime
// directory, without authentication: the socket's permissions are its
// only key. Under the unit (UnitFile) gost creates it with UMask 0007 in
// RuntimeDirectory mode 0750, so the gost user and its group (the Agent)
// reach it and nobody else does.
//
// The driver uses:
//
//   - GET /config: the running configuration, each service with its
//     status (createTime, and with enableStats its connections and bytes);
//   - PUT /config/hops/<name>, /config/admissions/<name>,
//     /config/limiters/<name>, /config/climiters/<name>: replace one hot
//     object (gost resolves them by name at every connection, so no
//     service is re-created, no listener closes and no counter restarts);
//   - POST /config/<kind> and DELETE /config/<kind>/<name> for services,
//     chains and the hot objects (F4c): Apply adds, re-creates and
//     deletes the objects of the hops it changes one by one (sync.go), so
//     every other service, its listener, connections, UDP sessions, mux
//     carriers and statistics stay. A changed service is deleted and
//     created again, never replaced with PUT: gost's PUT closes the old
//     service before it builds the new one and keeps a closed service
//     registered when that fails.
//
// The API decodes bodies with encoding/json, so a duration is an integer
// of nanoseconds there (the configuration file takes "30s").

// apiTimeout bounds one API call.
const apiTimeout = 10 * time.Second

func newAPIClient(sock string) *http.Client {
	return &http.Client{
		Timeout: apiTimeout,
		Transport: &http.Transport{
			Proxy: nil,
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var dl net.Dialer
				return dl.DialContext(ctx, "unix", sock)
			},
			MaxIdleConnsPerHost: 2,
			IdleConnTimeout:     30 * time.Second,
		},
	}
}

// apiError is gost's error answer.
type apiError struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
}

// refusedError is gost's answer to a request it refused (any status but
// 200): gost checks a request before it changes anything, so a refused
// request changed nothing. A transport error, by contrast, leaves it
// unknown whether gost acted.
type refusedError struct {
	method, path, status, msg string
}

func (e *refusedError) Error() string {
	return fmt.Sprintf("gost driver: api %s %s: %s: %s", e.method, e.path, e.status, e.msg)
}

// refused reports whether err is gost refusing a request.
func refused(err error) bool {
	var r *refusedError
	return errors.As(err, &r)
}

// call sends one API request with body as JSON (nil for none) and decodes
// the answer into out (nil to discard it).
func (d *Driver) call(ctx context.Context, method, path string, body, out any) error {
	if d.apiFault != nil && method != http.MethodGet {
		if err := d.apiFault(method, path); err != nil {
			return &refusedError{method: method, path: path, status: "injected", msg: err.Error()}
		}
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("gost driver: api %s %s: %w", method, path, err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://gost"+path, rd)
	if err != nil {
		return fmt.Errorf("gost driver: api %s %s: %w", method, path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := d.api.Do(req)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}
		return fmt.Errorf("gost driver: api %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("gost driver: api %s %s: %w", method, path, err)
	}
	if resp.StatusCode != http.StatusOK {
		var e apiError
		_ = json.Unmarshal(data, &e)
		return &refusedError{method: method, path: path, status: resp.Status, msg: clip(e.Msg)}
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("gost driver: api %s %s: %w", method, path, err)
		}
	}
	return nil
}

// put replaces the hot object name of kind (hops, admissions, limiters,
// climiters).
func (d *Driver) put(ctx context.Context, kind, name string, body any) error {
	return d.call(ctx, http.MethodPut, "/config/"+kind+"/"+url.PathEscape(name), body, nil)
}

// liveConfig is what GET /config answers, reduced to what the driver reads.
type liveConfig struct {
	Services   []liveService   `json:"services"`
	Chains     []liveNamed     `json:"chains"`
	Hops       []liveHop       `json:"hops"`
	Admissions []liveAdmission `json:"admissions"`
	Limiters   []liveNamed     `json:"limiters"`
	CLimiters  []liveNamed     `json:"climiters"`
}

// liveNamed is a running object of which the driver reads the name only.
type liveNamed struct {
	Name string `json:"name"`
}

// names answers the running objects of every kind sync changes, as
// "kind/name".
func (c *liveConfig) names() map[string]bool {
	out := map[string]bool{}
	add := func(kind, name string) { out[kind+"/"+name] = true }
	for _, o := range c.Services {
		add(kindServices, o.Name)
	}
	for _, o := range c.Chains {
		add(kindChains, o.Name)
	}
	for _, o := range c.Hops {
		add(kindHops, o.Name)
	}
	for _, o := range c.Admissions {
		add(kindAdmissions, o.Name)
	}
	for _, o := range c.Limiters {
		add(kindLimiters, o.Name)
	}
	for _, o := range c.CLimiters {
		add(kindCLimiters, o.Name)
	}
	return out
}

type liveService struct {
	Name   string `json:"name"`
	Status *struct {
		// CreateTime is when gost created the service object (unix
		// seconds): its statistics started from zero then.
		CreateTime int64      `json:"createTime"`
		Stats      *liveStats `json:"stats"`
	} `json:"status"`
}

// liveStats are a service's statistics: InputBytes are read from its
// clients (up), OutputBytes written to them (down).
type liveStats struct {
	TotalConns   uint64 `json:"totalConns"`
	CurrentConns uint64 `json:"currentConns"`
	InputBytes   uint64 `json:"inputBytes"`
	OutputBytes  uint64 `json:"outputBytes"`
}

type liveHop struct {
	Name     string         `json:"name"`
	Metadata map[string]any `json:"metadata"`
}

type liveAdmission struct {
	Name      string   `json:"name"`
	Whitelist bool     `json:"whitelist"`
	Matchers  []string `json:"matchers"`
}

// readLive reads the running configuration. gost answers its API once
// it serves (the API starts before the metrics path Apply waits for);
// right after a start by systemd it may not yet, so it retries briefly.
func (d *Driver) readLive(ctx context.Context) (*liveConfig, error) {
	deadline := time.Now().Add(2 * time.Second)
	for {
		var c liveConfig
		err := d.call(ctx, http.MethodGet, "/config", nil, &c)
		if err == nil {
			return &c, nil
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func (c *liveConfig) service(name string) *liveService {
	for i := range c.Services {
		if c.Services[i].Name == name {
			return &c.Services[i]
		}
	}
	return nil
}

func (c *liveConfig) hop(name string) *liveHop {
	for i := range c.Hops {
		if c.Hops[i].Name == name {
			return &c.Hops[i]
		}
	}
	return nil
}

func (c *liveConfig) admission(name string) *liveAdmission {
	for i := range c.Admissions {
		if c.Admissions[i].Name == name {
			return &c.Admissions[i]
		}
	}
	return nil
}

// The API's spelling of a hop: the selector's failTimeout in nanoseconds.
type (
	apiHop struct {
		Name     string            `json:"name"`
		Selector apiSelector       `json:"selector"`
		Nodes    []node            `json:"nodes"`
		Metadata map[string]string `json:"metadata,omitempty"`
	}
	apiSelector struct {
		Strategy    string `json:"strategy"`
		MaxFails    uint32 `json:"maxFails"`
		FailTimeout int64  `json:"failTimeout"`
	}
)

func toAPIHop(h hopConfig) (apiHop, error) {
	out := apiHop{Name: h.Name, Nodes: h.Nodes, Metadata: h.Metadata}
	if h.Selector != nil {
		ft, err := time.ParseDuration(h.Selector.FailTimeout)
		if err != nil {
			return apiHop{}, fmt.Errorf("%w: hop %s: failTimeout %q", driver.ErrInvalidArtifact, h.Name, h.Selector.FailTimeout)
		}
		out.Selector = apiSelector{Strategy: h.Selector.Strategy, MaxFails: h.Selector.MaxFails, FailTimeout: int64(ft)}
	}
	return out, nil
}

// decodeConfig reads a configuration the driver rendered.
func decodeConfig(content []byte) (*gostConfig, error) {
	var c gostConfig
	if err := json.Unmarshal(content, &c); err != nil {
		return nil, fmt.Errorf("%w: %v", driver.ErrInvalidArtifact, err)
	}
	return &c, nil
}

func (c *gostConfig) hop(name string) *hopConfig {
	for i := range c.Hops {
		if c.Hops[i].Name == name {
			return &c.Hops[i]
		}
	}
	return nil
}

func (c *gostConfig) admission(name string) *admission {
	for i := range c.Admissions {
		if c.Admissions[i].Name == name {
			return &c.Admissions[i]
		}
	}
	return nil
}

// errNotRunning is the error of calls that need a running gost.
var errNotRunning = errors.New("gost driver: gost is not running")
