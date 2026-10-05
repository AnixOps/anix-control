package relayctl

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Format names the configuration document's version.
const Format = "anixops.relay/v1"

// Ingress and egress security names.
const (
	SecurityRaw     = "raw"
	SecurityAnixOps = "anixops"
)

// Carrier names (sdk/forward/relay's CarrierChoice).
const (
	CarrierAuto   = "auto"
	CarrierTLSTCP = "tls_tcp"
	CarrierQUIC   = "quic"
	CarrierPlain  = "plain"
)

// Balance strategy names.
const (
	BalanceRoundRobin = "round_robin"
	BalanceRandom     = "random"
	BalanceIPHash     = "ip_hash"
	BalanceLeastConn  = "least_conn"
	BalanceFailover   = "failover"
)

// Config is the relay's configuration document.
type Config struct {
	// Format is Format; Owner is the driver's ownership mark and Note says
	// the file is generated.
	Format string `json:"format"`
	Owner  string `json:"owner"`
	Note   string `json:"note"`
	// Link names the node's link certificate, its key and the link CA
	// bundle; without it the relay carries RAW and PLAIN links only.
	Link *LinkFiles `json:"link,omitempty"`
	// StatelessResetKeyFile holds the 32 bytes a QUIC listener answers the
	// packets of connections it forgot with (L3); the driver writes it.
	StatelessResetKeyFile string `json:"stateless_reset_key_file,omitempty"`
	Hops                  []Hop  `json:"hops"`
}

// LinkFiles are the node's link credentials as paths the relay reads.
type LinkFiles struct {
	Cert string `json:"cert"`
	Key  string `json:"key"`
	CA   string `json:"ca"`
}

// Hop is one hop of one route on this node.
type Hop struct {
	Route string `json:"route"`
	Hop   uint32 `json:"hop"`
	// Role is "entry", "relay" or "exit".
	Role   string `json:"role"`
	Listen Listen `json:"listen"`
	// Ingress is what the listener terminates.
	Ingress Ingress `json:"ingress"`
	// Sources are the admitted source prefixes (required on relay and exit
	// hops, so a relay is never open); empty on an entry admits everybody.
	Sources []string `json:"sources,omitempty"`
	// Peers are the identities allowed to dial an encrypted ingress.
	Peers     []string   `json:"peers,omitempty"`
	Upstreams []Upstream `json:"upstreams"`
	Balance   string     `json:"balance"`
	Breaker   Breaker    `json:"breaker"`
	Limits    Limits     `json:"limits"`
	// Paused admits nobody new; the hop, its listener and counters stay.
	Paused bool `json:"paused,omitempty"`
}

// Listen is the hop's socket: an empty address is every local address.
type Listen struct {
	Address string `json:"address,omitempty"`
	Port    uint32 `json:"port"`
	TCP     bool   `json:"tcp,omitempty"`
	UDP     bool   `json:"udp,omitempty"`
}

// Ingress is the transport a hop's listener terminates.
type Ingress struct {
	Security string `json:"security"`
	// Carrier is meaningful for SecurityAnixOps only.
	Carrier string `json:"carrier,omitempty"`
}

// Upstream is one place a hop sends traffic: the next hop's node (an AnixOps
// egress) or a target (a raw egress).
type Upstream struct {
	Address  string `json:"address"`
	Port     uint32 `json:"port"`
	Weight   uint32 `json:"weight"`
	Priority uint32 `json:"priority"`
	Egress   Egress `json:"egress"`
	// NodeRef and PeerIdentity name the next node and the identity it must
	// present (AnixOps egress).
	NodeRef      string `json:"node_ref,omitempty"`
	PeerIdentity string `json:"peer_identity,omitempty"`
}

// Egress is the transport an upstream is dialled with.
type Egress struct {
	Security   string `json:"security"`
	Carrier    string `json:"carrier,omitempty"`
	ServerName string `json:"server_name,omitempty"`
}

// Breaker is the circuit breaker of a hop's upstreams: after MaxFails
// failures in a row an upstream is skipped for OpenMs.
type Breaker struct {
	MaxFails uint32 `json:"max_fails"`
	OpenMs   uint32 `json:"open_ms"`
}

// Limits of a hop; zero is unlimited.
type Limits struct {
	BandwidthBps uint64 `json:"bandwidth_bps,omitempty"`
	QuotaBytes   uint64 `json:"quota_bytes,omitempty"`
	MaxConns     uint32 `json:"max_conns,omitempty"`
}

// Key identifies a hop.
type Key struct {
	Route string
	Hop   uint32
}

func (k Key) String() string { return fmt.Sprintf("%s/%d", k.Route, k.Hop) }

// Key returns the hop's key.
func (h Hop) Key() Key { return Key{h.Route, h.Hop} }

// ListenerKey is what makes a hop's listener: when it changes, the hop gets a
// new listener (and a new counter epoch). Everything else about a hop is hot.
func (h Hop) ListenerKey() string {
	return fmt.Sprintf("%s|%d|%t|%t|%s|%s", h.Listen.Address, h.Listen.Port, h.Listen.TCP, h.Listen.UDP, h.Ingress.Security, h.Ingress.Carrier)
}

// Encode writes the document deterministically (indented, newline at the end).
func Encode(c *Config) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		return nil, fmt.Errorf("relayctl: encode configuration: %w", err)
	}
	return b.Bytes(), nil
}

// Digest is the SHA-256 of the encoded document, hex.
func Digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// ErrInvalid wraps every refusal of a malformed document.
var ErrInvalid = errors.New("relayctl: invalid configuration")

// Parse reads a configuration document strictly (unknown fields are an
// error) and checks its shape: the format, sorted unique hops, names from the
// closed sets, ports in range. It does not check what only the host can (the
// files, the ports being free).
func Parse(content []byte) (*Config, error) {
	var c Config
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("%w: trailing data", ErrInvalid)
	}
	if err := c.Check(); err != nil {
		return nil, err
	}
	return &c, nil
}

// Check checks the document's shape.
func (c *Config) Check() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
	}
	if c.Format != Format {
		return bad("format %q, want %q", c.Format, Format)
	}
	if (c.Link != nil) && (c.Link.Cert == "" || c.Link.Key == "" || c.Link.CA == "") {
		return bad("link needs cert, key and ca")
	}
	var prev *Key
	for i := range c.Hops {
		h := &c.Hops[i]
		k := h.Key()
		if prev != nil && (k.Route < prev.Route || (k.Route == prev.Route && k.Hop <= prev.Hop)) {
			return bad("hop %s is out of order or repeated", k)
		}
		prev = &k
		if h.Route == "" || strings.ContainsAny(h.Route, " \t\r\n/") {
			return bad("hop %s: route id", k)
		}
		if !slices.Contains([]string{"entry", "relay", "exit"}, h.Role) {
			return bad("hop %s: role %q", k, h.Role)
		}
		if h.Listen.Port == 0 || h.Listen.Port > 65535 || (!h.Listen.TCP && !h.Listen.UDP) {
			return bad("hop %s: listen %d", k, h.Listen.Port)
		}
		switch h.Ingress.Security {
		case SecurityRaw:
			if h.Ingress.Carrier != "" {
				return bad("hop %s: a raw ingress has no carrier", k)
			}
		case SecurityAnixOps:
			if !slices.Contains([]string{CarrierAuto, CarrierTLSTCP, CarrierQUIC, CarrierPlain}, h.Ingress.Carrier) {
				return bad("hop %s: carrier %q", k, h.Ingress.Carrier)
			}
			if !h.Listen.TCP && !h.Listen.UDP {
				return bad("hop %s: no socket", k)
			}
		default:
			return bad("hop %s: ingress security %q", k, h.Ingress.Security)
		}
		if len(h.Upstreams) == 0 {
			return bad("hop %s: no upstream", k)
		}
		if !slices.Contains([]string{BalanceRoundRobin, BalanceRandom, BalanceIPHash, BalanceLeastConn, BalanceFailover}, h.Balance) {
			return bad("hop %s: balance %q", k, h.Balance)
		}
		if h.Breaker.MaxFails == 0 || h.Breaker.OpenMs == 0 {
			return bad("hop %s: breaker", k)
		}
		seen := map[string]bool{}
		for j, u := range h.Upstreams {
			id := fmt.Sprintf("%s:%d", u.Address, u.Port)
			if u.Address == "" || u.Port == 0 || u.Port > 65535 || u.Weight == 0 {
				return bad("hop %s: upstream %d", k, j)
			}
			if seen[id] {
				return bad("hop %s: upstream %s twice", k, id)
			}
			seen[id] = true
			switch u.Egress.Security {
			case SecurityRaw:
			case SecurityAnixOps:
				if !slices.Contains([]string{CarrierAuto, CarrierTLSTCP, CarrierQUIC, CarrierPlain}, u.Egress.Carrier) {
					return bad("hop %s: upstream %s carrier %q", k, id, u.Egress.Carrier)
				}
				if u.Egress.Carrier != CarrierPlain && (u.Egress.ServerName == "" || u.PeerIdentity == "") {
					return bad("hop %s: upstream %s needs a server name and a peer identity", k, id)
				}
			default:
				return bad("hop %s: upstream %s egress security %q", k, id, u.Egress.Security)
			}
		}
	}
	return nil
}
