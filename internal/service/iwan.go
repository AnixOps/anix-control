package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/netip"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// Panabit iWAN nodes (networkcore_anixops docs/protocols/iwan.md §12.4):
// anixcore terminates a UDP layer-3 tunnel whose users get addresses from
// the node's pool. iWAN has no confidentiality or integrity (§9).
//
// A user logs in with the panel user id and IWANPassword of its UUID; the
// Agent (anix-agent core/anixcore) derives the same password.
const (
	defaultIWANPool        = "10.66.0.0/16"
	defaultIWANMTU         = 1400
	minIWANMTU             = 576
	maxIWANMTU             = 1600
	defaultIWANDNSUpstream = "1.1.1.1"
	iwanPasswordLength     = 16
)

// IWANPassword is a user's iWAN password: the first 16 hex digits of its
// UUID, lower case (iWAN passwords are at most 16 bytes).
func IWANPassword(uuid string) string {
	digits := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(uuid), "-", ""))
	if len(digits) < iwanPasswordLength {
		return ""
	}
	return digits[:iwanPasswordLength]
}

// iwanFixedAddresses reads `fixed_addresses` (panel user id → IPv4).
func iwanFixedAddresses(settings map[string]any) map[string]string {
	out := map[string]string{}
	if raw, ok := settings["fixed_addresses"].(map[string]any); ok {
		for id, value := range raw {
			if address, ok := value.(string); ok && strings.TrimSpace(address) != "" {
				out[strings.TrimSpace(id)] = strings.TrimSpace(address)
			}
		}
	}
	return out
}

func iwanBoolSetting(settings map[string]any, key string, defaultValue bool) bool {
	if value, ok := settings[key].(bool); ok {
		return value
	}
	return defaultValue
}

// buildIWANConfig sets the node config fields of an iWAN protocol, with
// the kernel's defaults spelled out.
func buildIWANConfig(config, settings map[string]any) {
	config["pool"] = stringSetting(settings, "pool", defaultIWANPool)
	config["mtu"] = intSetting(settings, "mtu", defaultIWANMTU)
	config["dns_upstream"] = stringSetting(settings, "dns_upstream", defaultIWANDNSUpstream)
	config["allow_encrypt"] = iwanBoolSetting(settings, "allow_encrypt", true)
	config["allow_client_to_client"] = iwanBoolSetting(settings, "allow_client_to_client", false)
	config["fixed_addresses"] = iwanFixedAddresses(settings)
}

// applyIWANUser fills a subscriber's iWAN login: user id, password, the
// fixed address if the node gives one, and the MTU. Other users' fixed
// addresses are removed from the settings the subscription carries.
func applyIWANUser(parsed *model.ParsedNode, ctx *model.TemplateRenderContext) {
	if parsed.Settings == nil {
		parsed.Settings = make(map[string]any)
	}
	if ctx != nil {
		parsed.Username = strconv.FormatUint(uint64(ctx.UserID), 10)
		parsed.Password = IWANPassword(ctx.UUID)
		parsed.PeerIP = iwanFixedAddresses(parsed.Settings)[parsed.Username]
	}
	parsed.MTU = intSetting(parsed.Settings, "mtu", defaultIWANMTU)
	delete(parsed.Settings, "fixed_addresses")
}

func validateIWANProtocol(protocol *model.NodeProtocol) error {
	settings := map[string]any{}
	if protocol.Settings != nil && strings.TrimSpace(*protocol.Settings) != "" {
		if err := json.Unmarshal([]byte(*protocol.Settings), &settings); err != nil {
			return fmt.Errorf("iWAN settings must be a JSON object: %v", err)
		}
	}
	return ValidateIWANSettings(settings)
}

// ValidateIWANSettings checks an iWAN protocol's settings against what
// the anixcore iwan inbound accepts.
func ValidateIWANSettings(settings map[string]any) error {
	poolText := defaultIWANPool
	if value, ok := settings["pool"]; ok {
		text, isString := value.(string)
		if !isString {
			return errors.New("iWAN pool must be a string")
		}
		poolText = strings.TrimSpace(text)
	}
	pool, err := netip.ParsePrefix(poolText)
	if err != nil || !pool.Addr().Is4() || pool.Bits() < 8 || pool.Bits() > 30 || pool.Masked() != pool {
		return fmt.Errorf("iWAN pool %q must be an IPv4 network from /8 to /30, such as 10.66.0.0/16", poolText)
	}
	if value, ok := settings["mtu"]; ok {
		mtu, isNumber := value.(float64)
		if !isNumber || mtu != math.Trunc(mtu) || mtu < minIWANMTU || mtu > maxIWANMTU {
			return fmt.Errorf("iWAN mtu must be an integer from %d to %d", minIWANMTU, maxIWANMTU)
		}
	}
	if value, ok := settings["dns_upstream"]; ok {
		text, isString := value.(string)
		if !isString || !validIWANUpstream(strings.TrimSpace(text)) {
			return errors.New("iWAN dns_upstream must be an IP address or IP:port")
		}
	}
	for _, key := range []string{"allow_encrypt", "allow_client_to_client"} {
		if value, ok := settings[key]; ok {
			if _, isBool := value.(bool); !isBool {
				return fmt.Errorf("iWAN %s must be true or false", key)
			}
		}
	}
	value, ok := settings["fixed_addresses"]
	if !ok || value == nil {
		return nil
	}
	fixed, isObject := value.(map[string]any)
	if !isObject {
		return errors.New("iWAN fixed_addresses must map user ids to addresses")
	}
	gateway := pool.Addr().Next()
	last := lastIPv4(pool)
	owners := map[netip.Addr]string{}
	for id, raw := range fixed {
		if userID, err := strconv.ParseUint(strings.TrimSpace(id), 10, 32); err != nil || userID == 0 {
			return fmt.Errorf("iWAN fixed_addresses key %q must be a user id", id)
		}
		text, isString := raw.(string)
		address, err := netip.ParseAddr(strings.TrimSpace(text))
		if !isString || err != nil || !address.Is4() || !pool.Contains(address) ||
			address == pool.Addr() || address == gateway || address == last {
			return fmt.Errorf("iWAN fixed address of user %s must be a pool address other than the network, gateway %s and broadcast", id, gateway)
		}
		if owner, taken := owners[address]; taken {
			return fmt.Errorf("iWAN fixed address %s is given to users %s and %s", address, owner, id)
		}
		owners[address] = id
	}
	return nil
}

func validIWANUpstream(text string) bool {
	if address, err := netip.ParseAddr(text); err == nil {
		return !address.IsUnspecified()
	}
	addrPort, err := netip.ParseAddrPort(text)
	return err == nil && !addrPort.Addr().IsUnspecified() && addrPort.Port() != 0
}

func lastIPv4(prefix netip.Prefix) netip.Addr {
	octets := prefix.Addr().As4()
	value := uint32(octets[0])<<24 | uint32(octets[1])<<16 | uint32(octets[2])<<8 | uint32(octets[3])
	value |= math.MaxUint32 >> prefix.Bits()
	return netip.AddrFrom4([4]byte{byte(value >> 24), byte(value >> 16), byte(value >> 8), byte(value)})
}
