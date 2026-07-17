package plugincontrol

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	gostMeshAPIVersion    = "anixops.gost-mesh/v1"
	gostMeshMaxConfigSize = 256 << 10
	gostMeshMaxTunnels    = 128
	gostMeshMaxCIDRs      = 128
	gostMeshMaxPathBytes  = 4096
	gostMeshMaxTableID    = 252
	gostMeshMaxPriority   = 32765
)

type gostMeshValidationConfig struct {
	APIVersion     string                     `json:"api_version"`
	Apply          bool                       `json:"apply"`
	RollbackOnExit bool                       `json:"rollback_on_exit"`
	Tunnels        []gostMeshValidationTunnel `json:"tunnels"`
}

type gostMeshValidationTunnel struct {
	ID        string                    `json:"id"`
	Role      string                    `json:"role"`
	Transport string                    `json:"transport"`
	TUN       gostMeshValidationTUN     `json:"tun"`
	Routing   gostMeshValidationRouting `json:"routing"`
	Listen    *gostMeshValidationListen `json:"listen,omitempty"`
	Remote    *gostMeshValidationRemote `json:"remote,omitempty"`
	TLS       gostMeshValidationTLS     `json:"tls"`
	WSSPath   string                    `json:"wss_path"`
	Health    gostMeshValidationHealth  `json:"health"`
}

type gostMeshValidationTUN struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Peer    string `json:"peer_address"`
	Port    int    `json:"port"`
	MTU     int    `json:"mtu"`
}

type gostMeshValidationRouting struct {
	SourceCIDRs []string `json:"source_cidrs"`
	RouteCIDRs  []string `json:"route_cidrs"`
	Table       int      `json:"table"`
	Priority    int      `json:"priority"`
}

type gostMeshValidationListen struct {
	Address string `json:"address"`
	Port    int    `json:"port"`
}

type gostMeshValidationRemote struct {
	Address string `json:"host"`
	Port    int    `json:"port"`
}

type gostMeshValidationTLS struct {
	CAFile     string `json:"ca_file"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	ServerName string `json:"server_name"`
}

type gostMeshValidationHealth struct {
	Enabled             bool   `json:"enabled"`
	Target              string `json:"target"`
	SourceAddress       string `json:"source_address"`
	IntervalSeconds     int    `json:"interval_seconds"`
	TimeoutSeconds      int    `json:"timeout_seconds"`
	FailureThreshold    int    `json:"failure_threshold"`
	RestartDelaySeconds int    `json:"restart_delay_seconds"`
	RestartLimit        int    `json:"restart_limit"`
}

type gostMeshRawValidationConfig struct {
	APIVersion     string          `json:"api_version"`
	Apply          *bool           `json:"apply"`
	RollbackOnExit *bool           `json:"rollback_on_exit"`
	Tunnels        json.RawMessage `json:"tunnels"`
}

type gostMeshValidatedTunnel struct {
	tunnel      gostMeshValidationTunnel
	tunNetwork  netip.Prefix
	sourceCIDRs []netip.Prefix
	routeCIDRs  []netip.Prefix
}

var gostMeshForbiddenHealthTargetPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/4"),
}

func (e *GostMeshExecutor) ValidateConfiguration(ctx context.Context, raw json.RawMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateGostMeshConfiguration(raw); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPluginInput, err)
	}
	return nil
}

func validateGostMeshConfiguration(contents []byte) error {
	if len(contents) > gostMeshMaxConfigSize {
		return fmt.Errorf("plugin config exceeds %d bytes", gostMeshMaxConfigSize)
	}
	if !utf8.Valid(contents) {
		return errors.New("plugin config must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	var raw *gostMeshRawValidationConfig
	if err := decoder.Decode(&raw); err != nil {
		return fmt.Errorf("decode plugin config: %w", err)
	}
	if raw == nil {
		return errors.New("plugin config must be a JSON object")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("plugin config must contain exactly one JSON object")
		}
		return fmt.Errorf("decode trailing plugin config data: %w", err)
	}
	if raw.Apply == nil {
		return errors.New("apply must be declared")
	}
	if raw.RollbackOnExit == nil {
		return errors.New("rollback_on_exit must be declared")
	}
	tunnelJSON := bytes.TrimSpace(raw.Tunnels)
	if len(tunnelJSON) == 0 || tunnelJSON[0] != '[' {
		return errors.New("tunnels must be declared as an array")
	}
	var tunnels []gostMeshValidationTunnel
	tunnelDecoder := json.NewDecoder(bytes.NewReader(tunnelJSON))
	tunnelDecoder.DisallowUnknownFields()
	if err := tunnelDecoder.Decode(&tunnels); err != nil {
		return fmt.Errorf("decode tunnels: %w", err)
	}
	if err := tunnelDecoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("tunnels must contain exactly one array")
	}
	if len(tunnels) > gostMeshMaxTunnels {
		return fmt.Errorf("tunnels exceed %d entries", gostMeshMaxTunnels)
	}
	config := gostMeshValidationConfig{
		APIVersion:     raw.APIVersion,
		Apply:          *raw.Apply,
		RollbackOnExit: *raw.RollbackOnExit,
		Tunnels:        tunnels,
	}
	if err := validateGostMeshCanonicalStrings(config); err != nil {
		return err
	}
	return config.validate()
}

func validateGostMeshCanonicalStrings(config gostMeshValidationConfig) error {
	if config.APIVersion != strings.TrimSpace(config.APIVersion) {
		return errors.New("api_version must not contain surrounding whitespace")
	}
	for index, tunnel := range config.Tunnels {
		prefix := fmt.Sprintf("tunnel %d", index)
		checks := []struct {
			field     string
			value     string
			canonical string
		}{
			{"id", tunnel.ID, strings.TrimSpace(tunnel.ID)},
			{"role", tunnel.Role, strings.ToLower(strings.TrimSpace(tunnel.Role))},
			{"transport", tunnel.Transport, strings.ToLower(strings.TrimSpace(tunnel.Transport))},
			{"tun.name", tunnel.TUN.Name, strings.TrimSpace(tunnel.TUN.Name)},
			{"tun.address", tunnel.TUN.Address, strings.TrimSpace(tunnel.TUN.Address)},
			{"tun.peer_address", tunnel.TUN.Peer, strings.TrimSpace(tunnel.TUN.Peer)},
			{"tls.ca_file", tunnel.TLS.CAFile, strings.TrimSpace(tunnel.TLS.CAFile)},
			{"tls.cert_file", tunnel.TLS.CertFile, strings.TrimSpace(tunnel.TLS.CertFile)},
			{"tls.key_file", tunnel.TLS.KeyFile, strings.TrimSpace(tunnel.TLS.KeyFile)},
			{"tls.server_name", tunnel.TLS.ServerName, strings.TrimSpace(tunnel.TLS.ServerName)},
			{"wss_path", tunnel.WSSPath, strings.TrimSpace(tunnel.WSSPath)},
			{"health.target", tunnel.Health.Target, strings.TrimSpace(tunnel.Health.Target)},
			{"health.source_address", tunnel.Health.SourceAddress, strings.TrimSpace(tunnel.Health.SourceAddress)},
		}
		if tunnel.Listen != nil {
			checks = append(checks, struct {
				field     string
				value     string
				canonical string
			}{"listen.address", tunnel.Listen.Address, strings.TrimSpace(tunnel.Listen.Address)})
		}
		if tunnel.Remote != nil {
			checks = append(checks, struct {
				field     string
				value     string
				canonical string
			}{"remote.host", tunnel.Remote.Address, strings.TrimSpace(tunnel.Remote.Address)})
		}
		for _, check := range checks {
			if check.value != check.canonical {
				return fmt.Errorf("%s: %s must use its canonical form", prefix, check.field)
			}
		}
		for cidrIndex, value := range tunnel.Routing.SourceCIDRs {
			if value != strings.TrimSpace(value) {
				return fmt.Errorf("%s: routing.source_cidrs[%d] must use its canonical form", prefix, cidrIndex)
			}
		}
		for cidrIndex, value := range tunnel.Routing.RouteCIDRs {
			if value != strings.TrimSpace(value) {
				return fmt.Errorf("%s: routing.route_cidrs[%d] must use its canonical form", prefix, cidrIndex)
			}
		}
	}
	return nil
}

func (config gostMeshValidationConfig) validate() error {
	if config.APIVersion != gostMeshAPIVersion {
		return fmt.Errorf("api_version must be %q", gostMeshAPIVersion)
	}
	if !config.RollbackOnExit {
		return errors.New("rollback_on_exit must remain enabled for the gost-mesh v1 crash-safe lifecycle")
	}
	if config.Apply && len(config.Tunnels) == 0 {
		return errors.New("apply=true requires at least one tunnel")
	}
	if len(config.Tunnels) > gostMeshMaxTunnels {
		return fmt.Errorf("tunnels exceed %d entries", gostMeshMaxTunnels)
	}

	seenIDs := make(map[string]struct{}, len(config.Tunnels))
	seenTUNNames := make(map[string]string, len(config.Tunnels))
	seenTables := make(map[int]string, len(config.Tunnels))
	seenPriorities := make(map[int]string, len(config.Tunnels))
	validated := make([]gostMeshValidatedTunnel, 0, len(config.Tunnels))
	for index, tunnel := range config.Tunnels {
		checked, err := validateGostMeshTunnel(tunnel)
		if err != nil {
			return fmt.Errorf("tunnel %d: %w", index, err)
		}
		if _, exists := seenIDs[tunnel.ID]; exists {
			return fmt.Errorf("tunnel id %q is duplicated", tunnel.ID)
		}
		seenIDs[tunnel.ID] = struct{}{}
		if owner, exists := seenTUNNames[tunnel.TUN.Name]; exists {
			return fmt.Errorf("tunnels %q and %q use the same TUN name %q", owner, tunnel.ID, tunnel.TUN.Name)
		}
		seenTUNNames[tunnel.TUN.Name] = tunnel.ID
		if tunnel.Role == "entry" {
			if owner, exists := seenTables[tunnel.Routing.Table]; exists {
				return fmt.Errorf("tunnels %q and %q use the same routing table %d", owner, tunnel.ID, tunnel.Routing.Table)
			}
			seenTables[tunnel.Routing.Table] = tunnel.ID
			for offset := range tunnel.Routing.SourceCIDRs {
				priority := tunnel.Routing.Priority + offset
				if owner, exists := seenPriorities[priority]; exists {
					return fmt.Errorf("tunnels %q and %q use the same routing priority %d", owner, tunnel.ID, priority)
				}
				seenPriorities[priority] = tunnel.ID
			}
		}
		for _, previous := range validated {
			if checked.tunNetwork.Overlaps(previous.tunNetwork) {
				return fmt.Errorf("tunnels %q and %q use overlapping TUN networks", previous.tunnel.ID, tunnel.ID)
			}
			if tunnel.Role == "exit" && previous.tunnel.Role == "exit" && tunnel.TUN.Port == previous.tunnel.TUN.Port {
				return fmt.Errorf("exit tunnels %q and %q use the same TUN port %d", previous.tunnel.ID, tunnel.ID, tunnel.TUN.Port)
			}
			if tunnel.Role == "exit" && previous.tunnel.Role == "exit" &&
				tunnel.Transport == previous.tunnel.Transport && gostMeshEndpointsConflict(*tunnel.Listen, *previous.tunnel.Listen) {
				return fmt.Errorf("exit tunnels %q and %q have conflicting %s listeners", previous.tunnel.ID, tunnel.ID, tunnel.Transport)
			}
			if gostMeshPrefixesOverlap(checked.sourceCIDRs, previous.sourceCIDRs) {
				return fmt.Errorf("tunnels %q and %q use overlapping source CIDRs", previous.tunnel.ID, tunnel.ID)
			}
			if gostMeshPrefixesOverlap(checked.routeCIDRs, previous.routeCIDRs) {
				return fmt.Errorf("tunnels %q and %q use overlapping route CIDRs", previous.tunnel.ID, tunnel.ID)
			}
		}
		validated = append(validated, checked)
	}
	return nil
}

func validateGostMeshTunnel(tunnel gostMeshValidationTunnel) (gostMeshValidatedTunnel, error) {
	if !safeGostMeshID(tunnel.ID) {
		return gostMeshValidatedTunnel{}, errors.New("id is invalid")
	}
	if tunnel.Role != "entry" && tunnel.Role != "exit" {
		return gostMeshValidatedTunnel{}, errors.New("role must be entry or exit")
	}
	if tunnel.Transport != "quic" && tunnel.Transport != "wss" {
		return gostMeshValidatedTunnel{}, errors.New("transport must be quic or wss")
	}
	tunNetwork, err := validateGostMeshTUN(tunnel.TUN)
	if err != nil {
		return gostMeshValidatedTunnel{}, err
	}
	sourceCIDRs, err := validateGostMeshCIDRs(tunnel.Routing.SourceCIDRs, "routing.source_cidrs")
	if err != nil {
		return gostMeshValidatedTunnel{}, err
	}
	routeCIDRs, err := validateGostMeshCIDRs(tunnel.Routing.RouteCIDRs, "routing.route_cidrs")
	if err != nil {
		return gostMeshValidatedTunnel{}, err
	}
	if err := validateGostMeshHealth(tunnel.Health); err != nil {
		return gostMeshValidatedTunnel{}, err
	}
	if tunnel.Transport == "wss" {
		if err := validateGostMeshWSSPath(tunnel.WSSPath); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
	} else if tunnel.WSSPath != "" {
		return gostMeshValidatedTunnel{}, errors.New("wss_path is only valid for the wss transport")
	}

	switch tunnel.Role {
	case "entry":
		if tunnel.Routing.Table < 1 || tunnel.Routing.Table > gostMeshMaxTableID {
			return gostMeshValidatedTunnel{}, fmt.Errorf("routing.table must be between 1 and %d for entry", gostMeshMaxTableID)
		}
		if tunnel.Routing.Priority < 1 || tunnel.Routing.Priority > gostMeshMaxPriority {
			return gostMeshValidatedTunnel{}, fmt.Errorf("routing.priority must be between 1 and %d for entry", gostMeshMaxPriority)
		}
		if len(sourceCIDRs) > 0 && tunnel.Routing.Priority+len(sourceCIDRs)-1 > gostMeshMaxPriority {
			return gostMeshValidatedTunnel{}, fmt.Errorf("routing.priority range must not exceed %d", gostMeshMaxPriority)
		}
		if tunnel.Remote == nil {
			return gostMeshValidatedTunnel{}, errors.New("entry requires remote")
		}
		if tunnel.Listen != nil {
			return gostMeshValidatedTunnel{}, errors.New("entry must not declare listen")
		}
		if err := validateGostMeshRemote(*tunnel.Remote); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if len(sourceCIDRs) == 0 {
			return gostMeshValidatedTunnel{}, errors.New("entry requires routing.source_cidrs")
		}
		if tunnel.Health.Enabled {
			sourceAddress, _ := netip.ParseAddr(tunnel.Health.SourceAddress)
			contained := false
			for _, sourceCIDR := range sourceCIDRs {
				if sourceCIDR.Contains(sourceAddress) {
					contained = true
					break
				}
			}
			if !contained {
				return gostMeshValidatedTunnel{}, errors.New("health.source_address must belong to routing.source_cidrs")
			}
		}
		if len(routeCIDRs) != 0 {
			return gostMeshValidatedTunnel{}, errors.New("entry must not declare routing.route_cidrs")
		}
		if err := validateGostMeshTLSFile(tunnel.TLS.CAFile, "tls.ca_file"); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if err := validateGostMeshTLSFile(tunnel.TLS.CertFile, "tls.cert_file"); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if err := validateGostMeshTLSFile(tunnel.TLS.KeyFile, "tls.key_file"); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if tunnel.TLS.CertFile == tunnel.TLS.KeyFile {
			return gostMeshValidatedTunnel{}, errors.New("tls.cert_file and tls.key_file must be different files")
		}
		if !safeGostMeshServerName(tunnel.TLS.ServerName) {
			return gostMeshValidatedTunnel{}, errors.New("entry requires a valid tls.server_name")
		}
	case "exit":
		if tunnel.Routing.Table != 0 || tunnel.Routing.Priority != 0 {
			return gostMeshValidatedTunnel{}, errors.New("exit routing.table and routing.priority must be omitted or 0")
		}
		if tunnel.Listen == nil {
			return gostMeshValidatedTunnel{}, errors.New("exit requires listen")
		}
		if tunnel.Remote != nil {
			return gostMeshValidatedTunnel{}, errors.New("exit must not declare remote")
		}
		if err := validateGostMeshListen(*tunnel.Listen); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if len(routeCIDRs) == 0 {
			return gostMeshValidatedTunnel{}, errors.New("exit requires routing.route_cidrs")
		}
		if len(sourceCIDRs) != 0 {
			return gostMeshValidatedTunnel{}, errors.New("exit must not declare routing.source_cidrs")
		}
		if err := validateGostMeshTLSFile(tunnel.TLS.CertFile, "tls.cert_file"); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if err := validateGostMeshTLSFile(tunnel.TLS.KeyFile, "tls.key_file"); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if tunnel.TLS.CertFile == tunnel.TLS.KeyFile {
			return gostMeshValidatedTunnel{}, errors.New("tls.cert_file and tls.key_file must be different files")
		}
		if err := validateGostMeshTLSFile(tunnel.TLS.CAFile, "tls.ca_file"); err != nil {
			return gostMeshValidatedTunnel{}, err
		}
		if tunnel.TLS.ServerName != "" {
			return gostMeshValidatedTunnel{}, errors.New("exit must not declare tls.server_name")
		}
	}

	return gostMeshValidatedTunnel{
		tunnel: tunnel, tunNetwork: tunNetwork, sourceCIDRs: sourceCIDRs, routeCIDRs: routeCIDRs,
	}, nil
}

func validateGostMeshTUN(config gostMeshValidationTUN) (netip.Prefix, error) {
	if !safeGostMeshInterfaceName(config.Name) {
		return netip.Prefix{}, errors.New("tun.name is invalid")
	}
	prefix, err := netip.ParsePrefix(config.Address)
	if err != nil || !prefix.Addr().Is4() {
		return netip.Prefix{}, errors.New("tun.address must be an IPv4 prefix")
	}
	address := prefix.Addr()
	if !safeGostMeshIPv4Address(address, false) {
		return netip.Prefix{}, errors.New("tun.address must contain a usable IPv4 host address")
	}
	if prefix.Bits() >= 32 || prefix.Masked().Addr() == address || gostMeshIPv4Broadcast(prefix, address) {
		return netip.Prefix{}, errors.New("tun.address must contain a usable host address and peer")
	}
	peer, err := netip.ParseAddr(config.Peer)
	if err != nil || !peer.Is4() || !safeGostMeshIPv4Address(peer, false) {
		return netip.Prefix{}, errors.New("tun.peer_address must be a usable IPv4 address")
	}
	if peer == address || !prefix.Contains(peer) || prefix.Masked().Addr() == peer || gostMeshIPv4Broadcast(prefix, peer) {
		return netip.Prefix{}, errors.New("tun.peer_address must be a different usable host in tun.address network")
	}
	if config.Port < 1 || config.Port > 65535 {
		return netip.Prefix{}, errors.New("tun.port must be between 1 and 65535")
	}
	if config.MTU < 576 || config.MTU > 9000 {
		return netip.Prefix{}, errors.New("tun.mtu must be between 576 and 9000")
	}
	return prefix.Masked(), nil
}

func validateGostMeshCIDRs(values []string, field string) ([]netip.Prefix, error) {
	if len(values) > gostMeshMaxCIDRs {
		return nil, fmt.Errorf("%s exceeds %d entries", field, gostMeshMaxCIDRs)
	}
	result := make([]netip.Prefix, 0, len(values))
	for index, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !prefix.Addr().Is4() {
			return nil, fmt.Errorf("%s[%d] must be an IPv4 CIDR", field, index)
		}
		if prefix != prefix.Masked() {
			return nil, fmt.Errorf("%s[%d] must use the canonical network address", field, index)
		}
		for _, previous := range result {
			if prefix.Overlaps(previous) {
				return nil, fmt.Errorf("%s contains overlapping CIDRs", field)
			}
		}
		result = append(result, prefix)
	}
	return result, nil
}

func validateGostMeshListen(endpoint gostMeshValidationListen) error {
	address, err := netip.ParseAddr(endpoint.Address)
	if err != nil || !address.Is4() || !safeGostMeshIPv4Address(address, true) {
		return errors.New("listen.address must be an IPv4 address")
	}
	if endpoint.Port < 1 || endpoint.Port > 65535 {
		return errors.New("listen.port must be between 1 and 65535")
	}
	return nil
}

func validateGostMeshRemote(endpoint gostMeshValidationRemote) error {
	if endpoint.Port < 1 || endpoint.Port > 65535 {
		return errors.New("remote.port must be between 1 and 65535")
	}
	if address, err := netip.ParseAddr(endpoint.Address); err == nil {
		if !address.Is4() || !safeGostMeshIPv4Address(address, false) {
			return errors.New("remote.address must be an IPv4 address or DNS name")
		}
		return nil
	}
	if !safeGostMeshDNSName(endpoint.Address) {
		return errors.New("remote.address must be an IPv4 address or DNS name")
	}
	return nil
}

func validateGostMeshTLSFile(value, field string) error {
	if value == "" {
		return fmt.Errorf("%s is required", field)
	}
	if len(value) > gostMeshMaxPathBytes || strings.ContainsAny(value, "\x00\r\n") || !filepath.IsAbs(value) || filepath.Clean(value) != value || value == string(filepath.Separator) {
		return fmt.Errorf("%s must be a clean absolute local file path", field)
	}
	return nil
}

func validateGostMeshWSSPath(value string) error {
	if value == "" || len(value) > 1024 || !strings.HasPrefix(value, "/") || strings.HasPrefix(value, "//") || strings.ContainsAny(value, "\x00\r\n?#") {
		return errors.New("wss_path must be an absolute HTTP path without query or fragment")
	}
	return nil
}

func validateGostMeshHealth(config gostMeshValidationHealth) error {
	if config.IntervalSeconds < 5 || config.IntervalSeconds > 300 {
		return errors.New("health.interval_seconds must be between 5 and 300")
	}
	if config.TimeoutSeconds < 1 || config.TimeoutSeconds > 30 {
		return errors.New("health.timeout_seconds must be between 1 and 30")
	}
	if config.TimeoutSeconds >= config.IntervalSeconds {
		return errors.New("health.timeout_seconds must be less than health.interval_seconds")
	}
	if config.FailureThreshold < 1 || config.FailureThreshold > 20 {
		return errors.New("health.failure_threshold must be between 1 and 20")
	}
	if config.RestartDelaySeconds < 1 || config.RestartDelaySeconds > 300 {
		return errors.New("health.restart_delay_seconds must be between 1 and 300")
	}
	if config.RestartLimit < 1 || config.RestartLimit > 100 {
		return errors.New("health.restart_limit must be between 1 and 100")
	}
	if config.Enabled && config.Target == "" {
		return errors.New("health.target is required when health.enabled=true")
	}
	if config.Enabled && config.SourceAddress == "" {
		return errors.New("health.source_address is required when health.enabled=true")
	}
	if config.Target != "" {
		if err := validateGostMeshHealthTarget(config.Target); err != nil {
			return err
		}
	}
	if config.SourceAddress != "" {
		address, err := netip.ParseAddr(config.SourceAddress)
		if err != nil || !address.Is4() || !safeGostMeshIPv4Address(address, false) {
			return errors.New("health.source_address must be a usable IPv4 address")
		}
	}
	return nil
}

func validateGostMeshHealthTarget(value string) error {
	host, portText, err := net.SplitHostPort(value)
	if err != nil || host == "" {
		return errors.New("health.target must be an IPv4 or DNS host:port endpoint")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return errors.New("health.target port must be between 1 and 65535")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if !safeGostMeshHealthTargetAddress(address) {
			return errors.New("health.target must use a non-loopback, non-link-local, non-multicast IPv4 address or a DNS name")
		}
		return nil
	}
	if !safeGostMeshDNSName(host) {
		return errors.New("health.target must use a non-loopback, non-link-local, non-multicast IPv4 address or a DNS name")
	}
	return nil
}

func safeGostMeshHealthTargetAddress(address netip.Addr) bool {
	if !address.Is4() || !safeGostMeshIPv4Address(address, false) || address.IsLoopback() {
		return false
	}
	for _, prefix := range gostMeshForbiddenHealthTargetPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

func safeGostMeshID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') {
			continue
		}
		if index > 0 && (character == '-' || character == '_' || character == '.') {
			continue
		}
		return false
	}
	return true
}

func safeGostMeshInterfaceName(value string) bool {
	if value == "" || value == "." || value == ".." || len(value) > 15 {
		return false
	}
	for index := range value {
		character := value[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '_' || character == '-' || character == '.' {
			continue
		}
		return false
	}
	return true
}

func safeGostMeshDNSName(value string) bool {
	if value == "" || len(value) > 253 || strings.HasSuffix(value, ".") || strings.ContainsAny(value, "\x00\r\n/: \\") {
		return false
	}
	labels := strings.Split(value, ".")
	for _, label := range labels {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for index := range label {
			character := label[index]
			if (character >= 'a' && character <= 'z') ||
				(character >= 'A' && character <= 'Z') ||
				(character >= '0' && character <= '9') || character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func safeGostMeshServerName(value string) bool {
	if net.ParseIP(value) != nil {
		return false
	}
	return safeGostMeshDNSName(value)
}

func safeGostMeshIPv4Address(address netip.Addr, allowUnspecified bool) bool {
	if !address.IsValid() || !address.Is4() || address.IsMulticast() || address.IsLinkLocalUnicast() {
		return false
	}
	if address.IsUnspecified() {
		return allowUnspecified
	}
	return address.String() != "255.255.255.255"
}

func gostMeshIPv4Broadcast(prefix netip.Prefix, address netip.Addr) bool {
	if !prefix.Addr().Is4() || !address.Is4() || prefix.Bits() >= 31 {
		return false
	}
	network := prefix.Masked().Addr().As4()
	candidate := address.As4()
	mask := net.CIDRMask(prefix.Bits(), 32)
	for index := range network {
		if candidate[index] != network[index]|^mask[index] {
			return false
		}
	}
	return true
}

func gostMeshPrefixesOverlap(left, right []netip.Prefix) bool {
	for _, one := range left {
		for _, two := range right {
			if one.Overlaps(two) {
				return true
			}
		}
	}
	return false
}

func gostMeshEndpointsConflict(left, right gostMeshValidationListen) bool {
	if left.Port != right.Port {
		return false
	}
	leftAddress, _ := netip.ParseAddr(left.Address)
	rightAddress, _ := netip.ParseAddr(right.Address)
	return leftAddress == rightAddress || leftAddress.IsUnspecified() || rightAddress.IsUnspecified()
}
