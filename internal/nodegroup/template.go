package nodegroup

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v5/internal/service"
)

// Protocol templates (docs/design/node-onboarding, "Protocol Templates"): a
// proxy group's JSON list of the protocols every new node of the group
// gets. Control generates each node's secrets (a Reality key pair and short
// ID, a Shadowsocks 2022 server key, a WireGuard server key pair) and picks
// each port: the fixed port, or a random one of port_range that no other
// protocol of the node uses. Control cannot see which ports are busy on the
// machine: a bind failure shows in the node's runtime health.
//
// This release takes the protocols that need no certificate on the node:
// tls "none", and "reality" for VLESS. "acme" and "self_signed" come with
// certificates delivered by Control.

// Template limits.
const (
	MaxTemplateEntries = 16
	maxTemplateName    = 100
	maxNodeNameRunes   = 64
)

// Template TLS modes.
const (
	TemplateTLSNone       = "none"
	TemplateTLSReality    = "reality"
	templateTLSACME       = "acme"
	templateTLSSelfSigned = "self_signed"
)

// NodeProtocol TLS column values (model.NodeProtocol.TLS).
const (
	protocolTLSNone    = 0
	protocolTLSReality = 2
)

// TemplateEntry is one protocol of a template.
type TemplateEntry struct {
	Name string `json:"name"`
	Type string `json:"type"`
	// Port is fixed; PortRange [from, to] picks a random port per node.
	// Exactly one is set.
	Port              int             `json:"port,omitempty"`
	PortRange         []int           `json:"port_range,omitempty"`
	Transport         string          `json:"transport,omitempty"`
	TransportSettings json.RawMessage `json:"transport_settings,omitempty"`
	TLS               string          `json:"tls"`
	Reality           *RealityEntry   `json:"reality,omitempty"`
	Settings          json.RawMessage `json:"settings,omitempty"`
}

// RealityEntry is the camouflage target of a Reality protocol: the server
// name clients present and the destination Control's nodes forward
// unauthenticated handshakes to. The key pair and short ID are per node.
type RealityEntry struct {
	ServerName  string `json:"server_name"`
	Dest        string `json:"dest,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
}

// templateTypes are the protocol types a template may name in this release,
// with their TLS modes and transports.
var templateTypes = map[string]struct {
	tls        []string
	transports []string
}{
	string(model.ProtocolVLESS):       {tls: []string{TemplateTLSNone, TemplateTLSReality}, transports: []string{"tcp", "ws", "grpc", "h2", "httpupgrade"}},
	string(model.ProtocolVMess):       {tls: []string{TemplateTLSNone}, transports: []string{"tcp", "ws", "grpc", "h2", "httpupgrade"}},
	string(model.ProtocolShadowsocks): {tls: []string{TemplateTLSNone}, transports: []string{"tcp"}},
	string(model.ProtocolWireGuard):   {tls: []string{TemplateTLSNone}, transports: []string{"udp"}},
}

// realityTransports are the transports Reality runs on.
var realityTransports = []string{"tcp", "grpc", "h2"}

// generatedKeys are the settings Control generates per node: a template
// that sets one would give every node the same secret.
var generatedKeys = map[string]bool{
	"server_key": true, "server_private_key": true, "server_public_key": true, "public_key": true,
	"private_key": true, "short_id": true, "short_ids": true,
}

var hostnamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// ParseTemplate decodes and checks a template. Unknown fields are refused.
// An empty or null document is an empty template.
func ParseTemplate(raw json.RawMessage) ([]TemplateEntry, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return []TemplateEntry{}, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var entries []TemplateEntry
	if err := decoder.Decode(&entries); err != nil {
		return nil, invalid("protocol_template: %v", err)
	}
	if decoder.More() {
		return nil, invalid("protocol_template: one JSON list expected")
	}
	if entries == nil {
		entries = []TemplateEntry{}
	}
	if err := checkTemplate(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// checkTemplate validates a template and normalizes its entries in place.
func checkTemplate(entries []TemplateEntry) error {
	if len(entries) > MaxTemplateEntries {
		return invalid("protocol_template: at most %d protocols", MaxTemplateEntries)
	}
	names := map[string]bool{}
	fixed := map[int]bool{}
	for i := range entries {
		entry := &entries[i]
		where := fmt.Sprintf("protocol_template[%d]", i)
		entry.Type = strings.ToLower(strings.TrimSpace(entry.Type))
		kind, ok := templateTypes[entry.Type]
		if !ok {
			return invalid("%s: type %q is not supported by protocol templates; use vless, vmess, shadowsocks or wireguard", where, entry.Type)
		}
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Name == "" {
			entry.Name = entry.Type
		}
		if utf8.RuneCountInString(entry.Name) > maxTemplateName || hasControl(entry.Name) {
			return invalid("%s: name must be at most %d characters without control characters", where, maxTemplateName)
		}
		if names[entry.Name] {
			return invalid("%s: name %q is used twice", where, entry.Name)
		}
		names[entry.Name] = true

		entry.TLS = strings.ToLower(strings.TrimSpace(entry.TLS))
		if entry.TLS == "" {
			entry.TLS = TemplateTLSNone
		}
		switch {
		case entry.TLS == templateTLSACME || entry.TLS == templateTLSSelfSigned:
			return invalid("%s: tls %q is not supported yet: use none, or reality for vless", where, entry.TLS)
		case !contains(kind.tls, entry.TLS):
			return invalid("%s: tls %q is not supported for %s (supported: %s)", where, entry.TLS, entry.Type, strings.Join(kind.tls, ", "))
		}

		entry.Transport = strings.ToLower(strings.TrimSpace(entry.Transport))
		if entry.Transport == "" {
			entry.Transport = kind.transports[0]
		}
		if !contains(kind.transports, entry.Transport) {
			return invalid("%s: transport %q is not supported for %s (supported: %s)", where, entry.Transport, entry.Type, strings.Join(kind.transports, ", "))
		}
		if entry.TLS == TemplateTLSReality && !contains(realityTransports, entry.Transport) {
			return invalid("%s: reality runs on %s, not %s", where, strings.Join(realityTransports, ", "), entry.Transport)
		}

		if err := checkPorts(entry, where, fixed, len(entries)); err != nil {
			return err
		}
		if err := checkReality(entry, where); err != nil {
			return err
		}
		var err error
		if entry.Settings, err = checkObject(entry.Settings, where+".settings"); err != nil {
			return err
		}
		if entry.TransportSettings, err = checkObject(entry.TransportSettings, where+".transport_settings"); err != nil {
			return err
		}
		if entry.Type == string(model.ProtocolShadowsocks) {
			var settings map[string]any
			_ = json.Unmarshal(entry.Settings, &settings)
			if shadowsocksCipher(settings) == "" {
				return invalid("%s.settings: shadowsocks needs a method (for example 2022-blake3-aes-128-gcm)", where)
			}
		}
		// The protocol a node would get: the node checks run on it.
		protocol, err := buildProtocol(*entry, i, 0, entry.port())
		if err != nil {
			return invalid("%s: %v", where, err)
		}
		if err := service.ValidateNodeProtocol(&protocol); err != nil {
			return invalid("%s: %v", where, err)
		}
		if findings := service.ValidateProtocolSecrets(&protocol); len(findings) > 0 {
			return invalid("%s: %s %s fails the node secret checks (%s)", where, findings[0].Column, findings[0].Field, findings[0].Reason)
		}
	}
	return nil
}

// port is the fixed port, or the start of the range: the port the checks
// build the protocol with.
func (e TemplateEntry) port() int {
	if e.Port != 0 {
		return e.Port
	}
	if len(e.PortRange) == 2 {
		return e.PortRange[0]
	}
	return 0
}

func checkPorts(entry *TemplateEntry, where string, fixed map[int]bool, entries int) error {
	switch {
	case entry.Port != 0 && len(entry.PortRange) != 0:
		return invalid("%s: set port or port_range, not both", where)
	case entry.Port != 0:
		if entry.Port < 1 || entry.Port > 65535 {
			return invalid("%s: port must be within 1-65535", where)
		}
		if fixed[entry.Port] {
			return invalid("%s: port %d is used by another protocol of the template", where, entry.Port)
		}
		fixed[entry.Port] = true
	case len(entry.PortRange) != 0:
		if len(entry.PortRange) != 2 || entry.PortRange[0] < 1 || entry.PortRange[0] > entry.PortRange[1] || entry.PortRange[1] > 65535 {
			return invalid("%s: port_range must be [from, to] with 1 <= from <= to <= 65535", where)
		}
		// Room for this protocol whatever ports the others take.
		if entry.PortRange[1]-entry.PortRange[0]+1 < entries {
			return invalid("%s: port_range must hold at least %d ports (one per protocol of the template)", where, entries)
		}
	default:
		return invalid("%s: port or port_range is required", where)
	}
	return nil
}

func checkReality(entry *TemplateEntry, where string) error {
	if entry.TLS != TemplateTLSReality {
		if entry.Reality != nil {
			return invalid("%s: reality is set but tls is %s", where, entry.TLS)
		}
		return nil
	}
	if entry.Reality == nil {
		return invalid("%s: tls reality needs reality.server_name", where)
	}
	reality := entry.Reality
	reality.ServerName = strings.TrimSpace(reality.ServerName)
	if len(reality.ServerName) > 253 || !hostnamePattern.MatchString(reality.ServerName) {
		return invalid("%s: reality.server_name must be a host name", where)
	}
	reality.Dest = strings.TrimSpace(reality.Dest)
	if reality.Dest == "" {
		reality.Dest = net.JoinHostPort(reality.ServerName, "443")
	}
	host, port, err := net.SplitHostPort(reality.Dest)
	if number, convErr := strconv.Atoi(port); err != nil || host == "" || convErr != nil || number < 1 || number > 65535 ||
		strings.ContainsAny(host, " /") {
		return invalid("%s: reality.dest must be host:port", where)
	}
	reality.Fingerprint = strings.TrimSpace(reality.Fingerprint)
	if reality.Fingerprint == "" {
		reality.Fingerprint = "chrome"
	}
	if len(reality.Fingerprint) > 32 || hasControl(reality.Fingerprint) {
		return invalid("%s: reality.fingerprint is too long", where)
	}
	return nil
}

// checkObject checks that raw is a JSON object (or absent) whose top-level
// keys hold no secret Control generates per node, and returns it compact.
func checkObject(raw json.RawMessage, where string) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, invalid("%s must be a JSON object", where)
	}
	for key := range object {
		if generatedKeys[strings.ToLower(key)] || nodesecrets.IsSecretKey(key) {
			return nil, invalid("%s: %q is a secret Control generates for each node; leave it out", where, key)
		}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return nil, invalid("%s must be a JSON object", where)
	}
	return compact.Bytes(), nil
}

// buildProtocol is the protocol entry gives a node: its settings with the
// node's own secrets, on port.
func buildProtocol(entry TemplateEntry, sort int, nodeID uint, port int) (model.NodeProtocol, error) {
	protocol := model.NodeProtocol{
		NodeID: nodeID, Name: entry.Name, Type: model.ProtocolType(entry.Type), Port: port, Enable: 1, Show: 1, Sort: sort,
		TLS: protocolTLSNone,
	}
	transport := entry.Transport
	protocol.Transport = &transport
	if len(entry.TransportSettings) > 0 {
		text := string(entry.TransportSettings)
		protocol.TransportSettings = &text
	}
	settings := map[string]any{}
	if len(entry.Settings) > 0 {
		if err := json.Unmarshal(entry.Settings, &settings); err != nil {
			return model.NodeProtocol{}, err
		}
	}
	switch entry.Type {
	case string(model.ProtocolShadowsocks):
		if cipher := shadowsocksCipher(settings); strings.HasPrefix(cipher, "2022-blake3-") {
			key, err := randomBase64(ss2022KeyLength(cipher))
			if err != nil {
				return model.NodeProtocol{}, err
			}
			settings["server_key"] = key
		}
	case string(model.ProtocolWireGuard):
		private, public, err := service.GenerateWireGuardKeypair()
		if err != nil {
			return model.NodeProtocol{}, err
		}
		settings["server_private_key"], settings["server_public_key"] = private, public
	}
	if len(settings) > 0 {
		encoded, err := json.Marshal(settings)
		if err != nil {
			return model.NodeProtocol{}, err
		}
		text := string(encoded)
		protocol.Settings = &text
	}
	if entry.TLS == TemplateTLSReality && entry.Reality != nil {
		private, public, err := realityKeyPair()
		if err != nil {
			return model.NodeProtocol{}, err
		}
		shortID := make([]byte, 8)
		if _, err := rand.Read(shortID); err != nil {
			return model.NodeProtocol{}, err
		}
		protocol.TLS = protocolTLSReality
		tlsSettings, err := json.Marshal(map[string]any{"server_name": entry.Reality.ServerName, "fingerprint": entry.Reality.Fingerprint})
		if err != nil {
			return model.NodeProtocol{}, err
		}
		reality, err := json.Marshal(map[string]any{
			"server_name": entry.Reality.ServerName, "dest": entry.Reality.Dest,
			"private_key": private, "public_key": public, "short_id": hex.EncodeToString(shortID),
		})
		if err != nil {
			return model.NodeProtocol{}, err
		}
		tlsText, realityText := string(tlsSettings), string(reality)
		protocol.TLSSettings, protocol.RealitySettings = &tlsText, &realityText
	}
	return protocol, nil
}

// realityKeyPair returns a new X25519 key pair as the Reality server and
// its clients take it: base64url without padding.
func realityKeyPair() (private, public string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(key.Bytes()), base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes()), nil
}

// shadowsocksCipher is a Shadowsocks protocol's method ("cipher" or
// "method", as the configuration builder reads it).
func shadowsocksCipher(settings map[string]any) string {
	for _, key := range []string{"cipher", "method"} {
		if value, ok := settings[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// ss2022KeyLength is the key length of a Shadowsocks 2022 cipher.
func ss2022KeyLength(cipher string) int {
	if strings.Contains(cipher, "128") {
		return 16
	}
	return 32
}

func randomBase64(length int) (string, error) {
	raw := make([]byte, length)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// assignPorts picks each entry's port for one node: the fixed port, or a
// random port of its range that no other protocol of the node uses.
func assignPorts(entries []TemplateEntry) ([]int, error) {
	used := map[int]bool{}
	for _, entry := range entries {
		if entry.Port != 0 {
			used[entry.Port] = true
		}
	}
	ports := make([]int, len(entries))
	for i, entry := range entries {
		if entry.Port != 0 {
			ports[i] = entry.Port
			continue
		}
		if len(entry.PortRange) != 2 {
			return nil, fmt.Errorf("protocol %q has no port", entry.Name)
		}
		port, err := randomFreePort(entry.PortRange[0], entry.PortRange[1], used)
		if err != nil {
			return nil, fmt.Errorf("protocol %q: %w", entry.Name, err)
		}
		used[port] = true
		ports[i] = port
	}
	return ports, nil
}

// randomFreePort draws ports of [from, to] until one is free. The template
// checks guarantee a free one, since a range holds a port per protocol.
func randomFreePort(from, to int, used map[int]bool) (int, error) {
	size := int64(to - from + 1)
	for attempt := 0; attempt < 64; attempt++ {
		offset, err := rand.Int(rand.Reader, big.NewInt(size))
		if err != nil {
			return 0, err
		}
		if port := from + int(offset.Int64()); !used[port] {
			return port, nil
		}
	}
	for port := from; port <= to; port++ {
		if !used[port] {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port in %d-%d", from, to)
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

func hasControl(value string) bool {
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return true
		}
	}
	return false
}
