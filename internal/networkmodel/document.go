// Package networkmodel is Control's side of the device network model
// (docs/design/network-model, docs/design/network-model/implementation.md):
// the model an administrator declares (lines on nodes, ingresses and
// egresses on lines, transports between devices, paths from user ingresses
// to internet egresses), the ANX-Line link keys Control generates and
// seals, and the planner that turns the model into one
// anixops.network.v1 NetworkPlan per device (sdk/network).
//
// The model is one document, replaced whole (PUT /api/v4/kernel/network)
// so a change that spans devices is checked and stored atomically: every
// device's plan must pass sdk/network.Check and the cross-device rules of
// Plan before anything is written.
package networkmodel

import (
	"errors"
	"fmt"
	"github.com/AnixOps/anix-control/sdk/network"
)

// Document is the network model as the administrator declares it. Ids are
// the administrator's (1 to 64 ASCII letters, digits, '-' and '_'),
// unique per component type; they become the plan's component ids, and a
// transport's id is its ANX-Line link id.
type Document struct {
	Lines      []Line      `json:"lines"`
	Ingresses  []Ingress   `json:"ingresses"`
	Egresses   []Egress    `json:"egresses"`
	Transports []Transport `json:"transports"`
	Paths      []Path      `json:"paths"`
}

// Line is an uplink of a node (proxy-<id> or forward-<id>).
type Line struct {
	ID        string   `json:"id"`
	Node      string   `json:"node"`
	Name      string   `json:"name,omitempty"`
	Interface string   `json:"interface"`
	Carrier   string   `json:"carrier,omitempty"`
	Families  []string `json:"families,omitempty"` // "v4", "v6"
	GatewayV4 string   `json:"gateway_v4,omitempty"`
	GatewayV6 string   `json:"gateway_v6,omitempty"`
}

// Endpoint is an address and port.
type Endpoint struct {
	Address string `json:"address"`
	Port    uint32 `json:"port"`
}

// PortRange is an inclusive port range.
type PortRange struct {
	First uint32 `json:"first"`
	Last  uint32 `json:"last"`
}

// PortMap maps public ports to local ports (a nat ingress).
type PortMap struct {
	Public PortRange `json:"public"`
	Local  PortRange `json:"local"`
}

// PairingRule sends matching traffic to the paths with a label.
type PairingRule struct {
	Match     string `json:"match"`
	PathLabel string `json:"path_label"`
}

// Pairing is a user ingress's pairing policy (design section 5). Mode is
// same_line (the default), cross, failover or rules; Egresses are final
// egresses of the ingress's paths.
type Pairing struct {
	Mode            string        `json:"mode,omitempty"`
	Egresses        []string      `json:"egresses,omitempty"`
	Rules           []PairingRule `json:"rules,omitempty"`
	FallbackPath    string        `json:"fallback_path,omitempty"`
	FailbackDelayMs uint32        `json:"failback_delay_ms,omitempty"`
}

// Ingress is where traffic enters a device: users ("user") or the previous
// hop ("hop").
type Ingress struct {
	ID          string    `json:"id"`
	Line        string    `json:"line"`
	Kind        string    `json:"kind"`     // direct, nat
	Audience    string    `json:"audience"` // user, hop
	Family      string    `json:"family"`   // v4, v6
	Listen      Endpoint  `json:"listen"`
	PublicHost  string    `json:"public_host,omitempty"`
	PortMap     []PortMap `json:"port_map,omitempty"`
	Protocols   []string  `json:"protocols,omitempty"`
	Pairing     *Pairing  `json:"pairing,omitempty"`
	Disabled    bool      `json:"disabled,omitempty"`
	KindVersion uint32    `json:"kind_version,omitempty"`
}

// HealthCheck is an egress's health probe.
type HealthCheck struct {
	Targets           []string `json:"targets"`
	IntervalMs        uint32   `json:"interval_ms"`
	TimeoutMs         uint32   `json:"timeout_ms"`
	FailureThreshold  uint32   `json:"failure_threshold,omitempty"`
	RecoveryThreshold uint32   `json:"recovery_threshold,omitempty"`
}

// Egress is where traffic leaves a device: to the internet ("internet") or
// towards the next hop ("hop").
type Egress struct {
	ID           string       `json:"id"`
	Line         string       `json:"line"`
	Kind         string       `json:"kind"`     // nat_direct, dedicated_ip, hop
	Audience     string       `json:"audience"` // internet, hop
	Family       string       `json:"family"`   // v4, v6
	Source       string       `json:"source"`   // an address or "auto"
	Gateway      string       `json:"gateway,omitempty"`
	Table        uint32       `json:"table,omitempty"`
	RulePriority uint32       `json:"rule_priority,omitempty"`
	Health       *HealthCheck `json:"health,omitempty"`
	MTU          uint32       `json:"mtu,omitempty"`
	AdvMSS       uint32       `json:"advmss,omitempty"`
	Masquerade   bool         `json:"masquerade,omitempty"`
	Disabled     bool         `json:"disabled,omitempty"`
	KindVersion  uint32       `json:"kind_version,omitempty"`
}

// AnxLine is a transport's ANX-Line options. The dialing end reaches the
// accepting end's hop ingress at its public host (or listen address): the
// ingress port carries c1 (TCP); QUICPort and FakeTCPPort offer c2 and c3
// on the same host.
type AnxLine struct {
	CarrierOrder []string `json:"carrier_order,omitempty"`
	Multipath    bool     `json:"multipath,omitempty"`
	FEC          bool     `json:"fec,omitempty"`
	Egress       string   `json:"egress,omitempty"` // l1 (default), l2
	ServerName   string   `json:"server_name,omitempty"`
	QUICPort     uint32   `json:"quic_port,omitempty"`
	FakeTCPPort  uint32   `json:"faketcp_port,omitempty"`
}

// Transport is a segment between two devices, from one device's hop
// egress to another's hop ingress.
//
// Kind anx_line is the bare ANX-Line, only for trusted segments: Trusted
// says the segment runs over an IX, a private line or a domestic network
// only. Every other segment, cross-border ones above all, takes kind anx
// (ANX-A): ANX-Line under the A2 jacket and shaped, planned as an anx_line
// transport with egress l2 on c1 (TCP). WireGuard is never used across
// borders (owner, 2026-10-28).
type Transport struct {
	ID          string   `json:"id"`
	Kind        string   `json:"kind"` // anx_line, anx
	FromEgress  string   `json:"from_egress"`
	ToIngress   string   `json:"to_ingress"`
	Trusted     bool     `json:"trusted,omitempty"`
	MTU         uint32   `json:"mtu,omitempty"`
	AnxLine     *AnxLine `json:"anx_line,omitempty"`
	KindVersion uint32   `json:"kind_version,omitempty"`
}

// KindAnx is the jacketed transport kind (ANX-A) of the document; plans
// carry it as anx_line with egress l2.
const KindAnx = "anx"

// isLink reports whether a transport of kind runs as an ANX-Line link,
// with a link key.
func isLink(kind string) bool { return kind == network.TransportAnxLine || kind == KindAnx }

// Path is a user ingress, the transports in order (none: the egress is on
// the ingress's device) and the internet egress it leaves by.
type Path struct {
	ID       string   `json:"id"`
	Name     string   `json:"name,omitempty"`
	Ingress  string   `json:"ingress"`
	Segments []string `json:"segments,omitempty"`
	Egress   string   `json:"egress"`
	Labels   []string `json:"labels,omitempty"`
	Disabled bool     `json:"disabled,omitempty"`
}

// MaxComponents caps each list of the document.
const MaxComponents = 1024

// ErrInvalid wraps every refusal of a document; Violations lists them.
var ErrInvalid = errors.New("network model: invalid")

// Violation is one problem of a document, at a JSON-ish path
// ("transports[2].to_ingress").
type Violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Invalid is the error of a refused document.
type Invalid struct {
	Violations []Violation
}

func (e *Invalid) Error() string {
	if len(e.Violations) == 0 {
		return ErrInvalid.Error()
	}
	v := e.Violations[0]
	if len(e.Violations) == 1 {
		return fmt.Sprintf("%v: %s: %s", ErrInvalid, v.Field, v.Message)
	}
	return fmt.Sprintf("%v: %s: %s (and %d more)", ErrInvalid, v.Field, v.Message, len(e.Violations)-1)
}

func (e *Invalid) Unwrap() error { return ErrInvalid }

type violations struct{ list []Violation }

func (v *violations) add(field, format string, args ...any) {
	if len(v.list) < 100 {
		v.list = append(v.list, Violation{Field: field, Message: fmt.Sprintf(format, args...)})
	}
}

func (v *violations) err() error {
	if len(v.list) == 0 {
		return nil
	}
	return &Invalid{Violations: v.list}
}
