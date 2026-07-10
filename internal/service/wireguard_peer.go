package service

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	defaultWireGuardCIDR = "10.66.0.0/24"
	defaultWireGuardMTU  = 1280
)

var wireGuardPeerMu sync.Mutex

func (s *SubscriptionService) applyWireGuardPeer(parsed *model.ParsedNode, protocol *model.NodeProtocol, ctx *model.TemplateRenderContext) {
	if parsed.Settings == nil {
		parsed.Settings = make(map[string]any)
	}

	settings := parsed.Settings
	cidr := stringSetting(settings, "cidr", defaultWireGuardCIDR)
	peer, err := s.GetOrCreateWireGuardPeer(protocol.ID, ctx.UserID, cidr)
	if err != nil {
		parsed.Settings["wireguard_error"] = err.Error()
		return
	}

	parsed.PrivateKey = peer.PrivateKey
	parsed.PresharedKey = peer.PresharedKey
	parsed.PeerIP = peer.PeerIP
	parsed.Settings["peer_public_key"] = peer.PublicKey

	parsed.PublicKey = firstStringSetting(settings, "server_public_key", "public_key")
	if parsed.PublicKey == "" {
		parsed.PublicKey = wireGuardPublicKeyFromPrivate(stringSetting(settings, "server_private_key", ""))
	}
	parsed.AllowedIPs = stringSliceSetting(settings, "allowed_ips", []string{"0.0.0.0/0"})
	parsed.DNS = stringSliceSetting(settings, "dns", []string{"1.1.1.1", "8.8.8.8"})
	parsed.MTU = intSetting(settings, "mtu", defaultWireGuardMTU)
	if parsed.MTU <= 0 {
		parsed.MTU = defaultWireGuardMTU
	}

	if _, ok := settings["endpoint"]; !ok {
		settings["endpoint"] = net.JoinHostPort(parsed.Server, fmt.Sprintf("%d", parsed.Port))
	}
	if _, ok := settings["tunnel_type"]; !ok {
		settings["tunnel_type"] = "quic"
	}
}

func (s *SubscriptionService) GetOrCreateWireGuardPeer(protocolID, userID uint, cidr string) (*model.WireGuardPeer, error) {
	if protocolID == 0 || userID == 0 {
		return nil, errors.New("wireguard protocol_id and user_id are required")
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return nil, fmt.Errorf("invalid wireguard cidr: %w", err)
	}
	if prefix.Bits() == 0 {
		return nil, errors.New("wireguard cidr must not be the entire address space")
	}

	// The service is constructed per request, so a package-level mutex is used
	// to keep allocation atomic across subscription and node-sync requests in
	// the same panel process. The database unique indexes remain the final guard
	// for multi-process deployments.
	wireGuardPeerMu.Lock()
	defer wireGuardPeerMu.Unlock()

	var peer model.WireGuardPeer
	if err := s.db.Where("node_protocol_id = ? AND user_id = ?", protocolID, userID).First(&peer).Error; err == nil {
		addr, parseErr := netip.ParseAddr(strings.TrimSpace(peer.PeerIP))
		if parseErr == nil && prefix.Contains(addr) && !isWireGuardReservedAddress(prefix, addr) {
			return &peer, nil
		}
		peerIP, allocErr := s.allocateWireGuardPeerIPLocked(protocolID, prefix)
		if allocErr != nil {
			return nil, allocErr
		}
		if err := s.db.Model(&peer).Update("peer_ip", peerIP).Error; err != nil {
			return nil, err
		}
		peer.PeerIP = peerIP
		return &peer, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	privateKey, publicKey, err := generateWireGuardKeypair()
	if err != nil {
		return nil, err
	}
	presharedKey, err := generateWireGuardPresharedKey()
	if err != nil {
		return nil, err
	}

	peer = model.WireGuardPeer{
		NodeProtocolID: protocolID,
		UserID:         userID,
		PrivateKey:     privateKey,
		PublicKey:      publicKey,
		PresharedKey:   presharedKey,
	}
	for attempt := 0; attempt < 3; attempt++ {
		peer.PeerIP, err = s.allocateWireGuardPeerIPLocked(protocolID, prefix)
		if err != nil {
			return nil, err
		}
		if err := s.db.Create(&peer).Error; err == nil {
			return &peer, nil
		} else if !isWireGuardUniqueConstraintError(err) {
			return nil, err
		}

		// Another panel replica may have allocated the same address or created
		// this user's peer concurrently. Re-read before selecting a new address.
		var existing model.WireGuardPeer
		lookupErr := s.db.Where("node_protocol_id = ? AND user_id = ?", protocolID, userID).First(&existing).Error
		if lookupErr == nil {
			return &existing, nil
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return nil, lookupErr
		}
	}
	return nil, errors.New("wireguard peer allocation conflicted with another writer")
}

func isWireGuardUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "duplicate")
}

func (s *SubscriptionService) BuildWireGuardRuntimeUserExtras(protocol *model.NodeProtocol, users []*model.User) (map[uint]map[string]string, error) {
	if protocol == nil || protocol.Type != model.ProtocolWireGuard {
		return nil, nil
	}
	if isWireGuardExitProtocol(protocol) {
		return nil, nil
	}
	cidr := defaultWireGuardCIDR
	if protocol.Settings != nil && *protocol.Settings != "" {
		cidr = stringSetting(parseWireGuardSettings(*protocol.Settings), "cidr", defaultWireGuardCIDR)
	}

	extras := make(map[uint]map[string]string, len(users))
	for _, user := range users {
		if user == nil {
			continue
		}
		peer, err := s.GetOrCreateWireGuardPeer(protocol.ID, user.ID, cidr)
		if err != nil {
			return nil, err
		}
		extras[user.ID] = map[string]string{
			"wireguard_protocol_id":     fmt.Sprintf("%d", protocol.ID),
			"wireguard_peer_ip":         peer.PeerIP,
			"wireguard_public_key":      peer.PublicKey,
			"wireguard_preshared_key":   peer.PresharedKey,
			"wireguard_peer_public_key": peer.PublicKey,
		}
	}
	return extras, nil
}

func parseWireGuardSettings(raw string) map[string]any {
	var settings map[string]any
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return nil
	}
	return settings
}

// IsWireGuardExitProtocol identifies the overseas relay role. Exit nodes do
// not terminate user WireGuard peers, so they must neither allocate peer keys,
// appear in user subscriptions, nor receive user runtime metadata.
func IsWireGuardExitProtocol(protocol *model.NodeProtocol) bool {
	if protocol == nil || protocol.Type != model.ProtocolWireGuard || protocol.Settings == nil {
		return false
	}
	relay, ok := parseWireGuardSettings(*protocol.Settings)["relay"].(map[string]any)
	if !ok {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(stringSetting(relay, "role", "entry")), "exit")
}

func isWireGuardExitProtocol(protocol *model.NodeProtocol) bool {
	return IsWireGuardExitProtocol(protocol)
}

func (s *SubscriptionService) allocateWireGuardPeerIP(protocolID uint, cidr string) (string, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return "", fmt.Errorf("invalid wireguard cidr: %w", err)
	}
	wireGuardPeerMu.Lock()
	defer wireGuardPeerMu.Unlock()
	return s.allocateWireGuardPeerIPLocked(protocolID, prefix)
}

func (s *SubscriptionService) allocateWireGuardPeerIPLocked(protocolID uint, prefix netip.Prefix) (string, error) {
	if prefix.Bits() == 0 {
		return "", errors.New("wireguard cidr must not be the entire address space")
	}

	var existing []string
	if err := s.db.Model(&model.WireGuardPeer{}).
		Where("node_protocol_id = ?", protocolID).
		Pluck("peer_ip", &existing).Error; err != nil {
		return "", err
	}
	used := make(map[string]bool, len(existing)+1)
	for _, ip := range existing {
		used[ip] = true
	}

	if prefix.Addr().Is4() {
		ones := prefix.Bits()
		if ones > 30 {
			return "", errors.New("wireguard cidr must contain at least two usable IPv4 addresses")
		}
		baseBytes := prefix.Masked().Addr().As4()
		base := uint64(binary.BigEndian.Uint32(baseBytes[:]))
		total := uint64(1) << uint(32-ones)
		// Reserve host .1 for the WireGuard entry interface and the broadcast
		// address for IPv4 networks.
		for offset := uint64(2); offset < total-1; offset++ {
			candidate := uint64ToIPv4(base + offset)
			if !used[candidate] {
				return candidate, nil
			}
		}
		return "", errors.New("wireguard cidr has no available peer addresses")
	}

	// IPv6 has no broadcast address. Start at ::2 (reserving ::1 for the
	// interface) and scan only as far as the number of existing peers requires,
	// which keeps /64 allocations practical without iterating the address space.
	candidate := prefix.Masked().Addr()
	maxAttempts := len(existing) + 1024
	for i := 0; i < maxAttempts; i++ {
		candidate = candidate.Next()
		if !prefix.Contains(candidate) {
			break
		}
		if isWireGuardReservedAddress(prefix, candidate) {
			continue
		}
		if !used[candidate.String()] {
			return candidate.String(), nil
		}
	}
	return "", errors.New("wireguard cidr has no available peer addresses")
}

func isWireGuardReservedAddress(prefix netip.Prefix, addr netip.Addr) bool {
	if !prefix.Contains(addr) || addr == prefix.Masked().Addr() {
		return true
	}
	if interfaceAddr := prefix.Masked().Addr().Next(); interfaceAddr.IsValid() && addr == interfaceAddr {
		return true
	}
	if prefix.Addr().Is4() && prefix.Bits() < 31 {
		baseBytes := prefix.Masked().Addr().As4()
		addrBytes := addr.As4()
		base := binary.BigEndian.Uint32(baseBytes[:])
		mask := uint32(^uint32(0) >> uint(prefix.Bits()))
		return binary.BigEndian.Uint32(addrBytes[:]) == base|mask
	}
	return false
}

func generateWireGuardKeypair() (privateKey string, publicKey string, err error) {
	key, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	privateKey = base64.StdEncoding.EncodeToString(key.Bytes())
	publicKey = base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
	return privateKey, publicKey, nil
}

// GenerateWireGuardKeypair exposes server-key generation to the authenticated
// admin workflow without persisting private material until the protocol is
// explicitly saved.
func GenerateWireGuardKeypair() (privateKey string, publicKey string, err error) {
	return generateWireGuardKeypair()
}

func generateWireGuardPresharedKey() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

func uint64ToIPv4(v uint64) string {
	if v > uint64(^uint32(0)) {
		return ""
	}
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(v)) // #nosec G115 -- v is explicitly bounded to MaxUint32 above.
	return netip.AddrFrom4(b).String()
}

func firstStringSetting(settings map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringSetting(settings, key, ""); value != "" {
			return value
		}
	}
	return ""
}

func stringSetting(settings map[string]any, key string, defaultValue string) string {
	if settings == nil {
		return defaultValue
	}
	if value, ok := settings[key].(string); ok && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value)
	}
	return defaultValue
}

func intSetting(settings map[string]any, key string, defaultValue int) int {
	if settings == nil {
		return defaultValue
	}
	return toIntSetting(settings[key], defaultValue)
}

func toIntSetting(value any, defaultValue int) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case jsonNumber:
		n, err := v.Int64()
		if err == nil {
			return int(n)
		}
	case string:
		var out int
		if _, err := fmt.Sscanf(v, "%d", &out); err == nil {
			return out
		}
	}
	return defaultValue
}

type jsonNumber interface {
	Int64() (int64, error)
}

func stringSliceSetting(settings map[string]any, key string, defaultValue []string) []string {
	if settings == nil {
		return append([]string(nil), defaultValue...)
	}
	value, ok := settings[key]
	if !ok {
		return append([]string(nil), defaultValue...)
	}
	switch v := value.(type) {
	case []string:
		return compactStrings(v, defaultValue)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return compactStrings(out, defaultValue)
	case string:
		return compactStrings(strings.Split(v, ","), defaultValue)
	default:
		return append([]string(nil), defaultValue...)
	}
}

func compactStrings(values []string, defaultValue []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return append([]string(nil), defaultValue...)
	}
	return out
}
