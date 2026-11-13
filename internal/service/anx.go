package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/entitlement"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// ANX nodes: anx-node (the Agent's kernel) terminates ANX-A, the private
// protocol under the A2 jacket, for users. A user's key is derived from its
// UUID, so Control and the Agent (anix-agent core/anixcore) agree on it
// without a table; the server key pair is the one the Agent's anx-server
// made, whose public half the Agent reports in NetworkStatus.public_keys.
const (
	defaultANXServerName = "www.apple.com"
	defaultANXDest       = "www.apple.com:443"

	// anxUserKeyLabel separates the user key from every other use of a UUID.
	anxUserKeyLabel = "anx/panel-user-key/v1"

	// anxPublicKeyKind is the kind of the server key the Agent reports.
	anxPublicKeyKind = "anx_line"
)

var defaultANXALPN = []string{"h2", "http/1.1"}

// anxMaxStreamsLimit is the largest max_streams anx-node takes (a u16); it
// defaults to 256 there, so Control sends max_streams only when set.
const anxMaxStreamsLimit = 65535

// ANXUserKey is a user's ANX key: 64 hex digits, the HMAC-SHA-256 of a
// fixed label under the UUID. "" without a UUID.
func ANXUserKey(uuid string) string {
	uuid = strings.ToLower(strings.TrimSpace(uuid))
	if uuid == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(uuid))
	mac.Write([]byte(anxUserKeyLabel))
	return hex.EncodeToString(mac.Sum(nil))
}

// buildANXConfig sets the node config fields of an ANX protocol, with the
// defaults spelled out.
func buildANXConfig(config, settings map[string]any) {
	config["server_names"] = stringSliceSetting(settings, "server_names", []string{defaultANXServerName})
	config["dest"] = stringSetting(settings, "dest", defaultANXDest)
	config["alpn"] = stringSliceSetting(settings, "alpn", defaultANXALPN)
	// The other A2 options of the anx-node inbound go out only when set, so
	// an older anx-node, which refuses keys it does not know, keeps working
	// until a node uses them.
	if anxBoolSetting(settings, "allow_no_sni") {
		config["allow_no_sni"] = true
	}
	if anxBoolSetting(settings, "shape_plain") {
		config["shape_plain"] = true
	}
	if n := intSetting(settings, "max_streams", 0); n > 0 && n <= anxMaxStreamsLimit {
		config["max_streams"] = n
	}
	// The wg jacket (WireGuard-shaped UDP, for users dialing a domestic
	// entry): anx-node opens this UDP port beside the A2 TCP port.
	if n := ANXWGPort(settings); n > 0 {
		config["wg_listen_port"] = n
	}
	// The bare jacket (random-looking TCP, anx spec §5.2) is for users
	// dialing a domestic entry only; validateANXProtocol ties it to the
	// no-SNI mode, the mode domestic entries run.
	if n := ANXBarePort(settings); n > 0 {
		config["bare_listen_port"] = n
	}
	// The cover site's TLS stack preset and per-key overrides (anx spec §4
	// item 6): ValidateANXSettings checks the shape, anx-node the values.
	if stack := stringSetting(settings, "dest_stack", ""); stack != "" {
		config["dest_stack"] = stack
	}
	if behavior, ok := settings["dest_behavior"].(map[string]any); ok && len(behavior) > 0 {
		config["dest_behavior"] = behavior
	}
	// Per-dest overrides (anx #208), keyed by an entry of the dest list.
	// anx-node refuses a key that is not in the list, so entries left over
	// from an earlier list stay stored but do not go out.
	if behaviors := anxDestBehaviors(config["dest"].(string), settings); len(behaviors) > 0 {
		config["dest_behaviors"] = behaviors
	}
	// The handshake generation: anx-node accepts both generations until
	// min_wire_gen is 2. Like the options above it goes out only when set,
	// for anx-nodes older than the second generation.
	if anxGenSetting(settings, "min_wire_gen") == 2 {
		config["min_wire_gen"] = 2
	}
}

// applyANXLicense adds the license options of the anx-node inbound (anx
// spec §2.4.1) while ANX licensing is on (internal/entitlement): the
// license CA the node trusts and, when required, require_license. Like the
// other optional keys they go out only when set. anx-node refuses
// require_license beside the wg jacket (its sessions never present a
// license), so such a node only accepts licenses.
func applyANXLicense(licenses *entitlement.Service, config map[string]any) {
	ca, require, err := licenses.ANXNodeLicense()
	if err != nil {
		log.Printf("anx license settings: %v", err)
		return
	}
	if ca == "" {
		return
	}
	config["license_ca"] = []string{ca}
	if _, wg := config["wg_listen_port"]; require && !wg {
		config["require_license"] = true
	}
}

// anxGenSetting is a handshake generation setting (wire_gen, min_wire_gen):
// 2 when set to 2, else 1, the generation every anx-node speaks.
func anxGenSetting(settings map[string]any, key string) int {
	if intSetting(settings, key, 1) == 2 {
		return 2
	}
	return 1
}

// ANXClientPublicKey is the server public key as clients get it. A node set
// to the second handshake generation (settings wire_gen 2) publishes its key
// with the suffix "@2": a client builds its login for the generation the key
// text names and never falls back, so the suffix goes out only once the node
// runs an anx-node that knows the second generation. A key the Agent already
// reported with a generation is passed through.
func ANXClientPublicKey(publicKey string, settings map[string]any) string {
	publicKey = strings.TrimSpace(publicKey)
	if publicKey == "" || strings.Contains(publicKey, "@") || anxGenSetting(settings, "wire_gen") != 2 {
		return publicKey
	}
	return publicKey + "@2"
}

// ANXWGPort is the UDP port of an ANX node's wg jacket, 0 when it is off.
func ANXWGPort(settings map[string]any) int {
	if n := intSetting(settings, "wg_listen_port", 0); n > 0 && n <= 65535 {
		return n
	}
	return 0
}

// anxDestStacks are the TLS stack presets of anx-node (dest_stack).
var anxDestStacks = []string{"openssl", "boringssl", "go", "rustls"}

// anxDestBehaviorKeys are the keys of the anx-node inbound's dest_behavior
// (anx public/BOUNDARY.md).
var anxDestBehaviorKeys = map[string]bool{
	"max_ccs": true, "ccs_overflow": true, "bad_ccs": true, "bad_record": true,
	"plaintext_handshake": true, "plaintext_alert": true, "unknown_content_type": true,
	"plaintext_stage": true, "unknown_type_stage": true, "heartbeat_as_record": true,
	"record_version": true, "max_record_len": true, "oversize_record": true,
	"read_ahead": true, "bad_finished": true, "unexpected_handshake": true,
	"handshake_timeout_secs": true, "timeout": true, "dest_close_grace_ms": true,
	"session_tickets": true, "flight_records": true, "flight_gap_ms": true,
	// TLS session resumption (anx #216): measured by dest-probe as one set.
	"ticket_lifetime": true, "ticket_ids": true, "ticket_prefix": true, "ticket_reuse": true,
	"resume_exts": true, "resume_flight": true, "resume_tickets": true, "resume_ticket_ids": true,
	"bad_binder": true,
}

// anxMaxTicketLifetime is the longest ticket_lifetime anx-node accepts (s).
const anxMaxTicketLifetime = 604800

// anxResumptionRequired are the keys anx-node needs once ticket_lifetime is
// non-zero (it refuses to start without them).
var anxResumptionRequired = []string{"ticket_ids", "resume_exts"}

// anxDestSiteKeys are the dest_behavior keys that hold the cover site's own
// data (anx spec §4 items 8 and 9), measured by anx-lab dest-probe: record
// lengths "len/len/…" (min, max length, max count), or "none".
var anxDestSiteKeys = map[string][3]int{
	"session_tickets": {35, 16401, 8},
	"flight_records":  {18, 16401, 16},
	"resume_tickets":  {35, 16401, 8},
	"resume_flight":   {18, 16401, 16},
}

// anxDests splits a dest list ("host:port[,host:port…]") as anx-node does.
func anxDests(dest string) []string {
	var dests []string
	for _, item := range strings.Split(dest, ",") {
		if item = strings.TrimSpace(item); item != "" {
			dests = append(dests, item)
		}
	}
	return dests
}

// anxDestBehaviors is settings dest_behaviors limited to the entries of dest.
func anxDestBehaviors(dest string, settings map[string]any) map[string]any {
	stored, _ := settings["dest_behaviors"].(map[string]any)
	behaviors := map[string]any{}
	for _, item := range anxDests(dest) {
		if behavior, ok := stored[item].(map[string]any); ok && len(behavior) > 0 {
			behaviors[item] = behavior
		}
	}
	return behaviors
}

func validateANXDestBehaviors(value any) error {
	behaviors, isObject := value.(map[string]any)
	if !isObject {
		return errors.New(`ANX dest_behaviors must be an object of dest ("host:port") to its dest_behavior`)
	}
	for dest, behavior := range behaviors {
		if strings.TrimSpace(dest) != dest || dest == "" || strings.ContainsAny(dest, ", \t") {
			return fmt.Errorf("ANX dest_behaviors has an invalid dest %q (one host:port of the dest list)", dest)
		}
		if err := validateANXDestBehavior(behavior); err != nil {
			return fmt.Errorf("%w (dest_behaviors %s)", err, dest)
		}
	}
	return nil
}

func validateANXDestBehavior(value any) error {
	behavior, isObject := value.(map[string]any)
	if !isObject {
		return errors.New("ANX dest_behavior must be an object (paste the dest_behavior line anx-lab dest-probe prints)")
	}
	for key, item := range behavior {
		if !anxDestBehaviorKeys[key] {
			return fmt.Errorf("ANX dest_behavior has an unknown key %q", key)
		}
		if limits, site := anxDestSiteKeys[key]; site {
			text, isString := item.(string)
			if !isString || !validANXRecordLengths(text, limits) {
				return fmt.Errorf("ANX dest_behavior %s must be \"none\" or record lengths \"len/len/…\" (%d to %d, at most %d)", key, limits[0], limits[1], limits[2])
			}
			continue
		}
		if key == "ticket_lifetime" {
			if _, ok := anxTicketLifetime(item); !ok {
				return fmt.Errorf("ANX dest_behavior ticket_lifetime must be whole seconds, 0 to %d", anxMaxTicketLifetime)
			}
			continue
		}
		switch v := item.(type) {
		case string:
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("ANX dest_behavior %s is empty", key)
			}
		case bool:
		case float64:
			if v != float64(int64(v)) || v < 0 {
				return fmt.Errorf("ANX dest_behavior %s must be a whole number", key)
			}
		default:
			return fmt.Errorf("ANX dest_behavior %s must be a string, a whole number or true / false", key)
		}
	}
	if lifetime, _ := anxTicketLifetime(behavior["ticket_lifetime"]); lifetime > 0 {
		for _, key := range anxResumptionRequired {
			if _, ok := behavior[key]; !ok {
				return fmt.Errorf("ANX dest_behavior turns on session resumption (ticket_lifetime) without %s: paste the whole dest_behavior line anx-lab dest-probe prints", key)
			}
		}
	}
	return nil
}

// anxTicketLifetime reads ticket_lifetime (a number or a numeric string);
// absent is 0.
func anxTicketLifetime(value any) (int64, bool) {
	var n int64
	switch v := value.(type) {
	case nil:
		return 0, true
	case float64:
		if v != float64(int64(v)) {
			return 0, false
		}
		n = int64(v)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil {
			return 0, false
		}
		n = parsed
	default:
		return 0, false
	}
	return n, n >= 0 && n <= anxMaxTicketLifetime
}

func validANXRecordLengths(text string, limits [3]int) bool {
	if text == "none" {
		return true
	}
	parts := strings.Split(text, "/")
	if len(parts) > limits[2] {
		return false
	}
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < limits[0] || n > limits[1] {
			return false
		}
	}
	return true
}

// ANXBarePort is the TCP port of an ANX node's bare jacket, 0 when it is off.
func ANXBarePort(settings map[string]any) int {
	if n := intSetting(settings, "bare_listen_port", 0); n > 0 && n <= 65535 {
		return n
	}
	return 0
}

// anxBoolSetting is a boolean ANX setting; anything but true is false.
func anxBoolSetting(settings map[string]any, key string) bool {
	value, _ := settings[key].(bool)
	return value
}

// ANXNoSNI reports whether an ANX node runs the A2 no-SNI mode: clients dial
// it without SNI, so subscriptions leave the SNI out.
func ANXNoSNI(settings map[string]any) bool {
	return anxBoolSetting(settings, "allow_no_sni")
}

// applyANXUser fills a subscriber's ANX login: the user key, the SNI the
// client sends (the first server name) and the ALPN. The server's public
// key comes from applyANXServerKey.
func applyANXUser(parsed *model.ParsedNode, ctx *model.TemplateRenderContext) {
	if parsed.Settings == nil {
		parsed.Settings = make(map[string]any)
	}
	if ctx != nil {
		parsed.Password = ANXUserKey(ctx.UUID)
	}
	parsed.ServerName = ""
	if names := stringSliceSetting(parsed.Settings, "server_names", []string{defaultANXServerName}); len(names) > 0 && !ANXNoSNI(parsed.Settings) {
		parsed.ServerName = names[0]
	}
	parsed.ALPN = strings.Join(stringSliceSetting(parsed.Settings, "alpn", defaultANXALPN), ",")
}

// anxServerPublicKey is the public key of the anx-server key pair a node
// reported, or "" while it has not reported one. nodeID is the node's id;
// a node is proxy-<id> or forward-<id> in the network model.
func anxServerPublicKey(db *gorm.DB, nodeID uint) string {
	var rows []model.NetworkDevice
	refs := []string{"proxy-" + strconv.FormatUint(uint64(nodeID), 10), "forward-" + strconv.FormatUint(uint64(nodeID), 10)}
	if err := db.Where("node_ref IN ?", refs).Find(&rows).Error; err != nil {
		return ""
	}
	for _, row := range rows {
		if row.PublicKeysJSON == "" {
			continue
		}
		keys := map[string]string{}
		if json.Unmarshal([]byte(row.PublicKeysJSON), &keys) == nil && strings.TrimSpace(keys[anxPublicKeyKind]) != "" {
			return strings.TrimSpace(keys[anxPublicKeyKind])
		}
	}
	return ""
}

func validateANXProtocol(protocol *model.NodeProtocol) error {
	settings := map[string]any{}
	if protocol.Settings != nil && strings.TrimSpace(*protocol.Settings) != "" {
		if err := json.Unmarshal([]byte(*protocol.Settings), &settings); err != nil {
			return fmt.Errorf("ANX settings must be a JSON object: %v", err)
		}
	}
	return ValidateANXSettings(settings)
}

// ValidateANXSettings checks an ANX protocol's settings against what the
// anx-node inbound accepts: server_names (the SNIs the server answers to),
// dest (the site it forwards everyone else to) and alpn.
func ValidateANXSettings(settings map[string]any) error {
	for _, key := range []string{"server_names", "alpn"} {
		value, ok := settings[key]
		if !ok || value == nil {
			continue
		}
		list, isList := value.([]any)
		if !isList || len(list) == 0 {
			return fmt.Errorf("ANX %s must be a non-empty list of strings", key)
		}
		// A server name is a host, so it may not contain '/'. An ALPN entry
		// is an RFC 7301 protocol ID such as "http/1.1", where '/' is
		// normal; only whitespace and ',' (the subscription separator) are
		// refused, and the ID is at most 255 bytes.
		forbidden := " \t,/"
		if key == "alpn" {
			forbidden = " \t,"
		}
		for _, item := range list {
			text, isString := item.(string)
			if !isString || strings.TrimSpace(text) == "" || strings.ContainsAny(text, forbidden) || len(text) > 255 {
				return fmt.Errorf("ANX %s has an invalid entry %v", key, item)
			}
		}
	}
	for _, key := range []string{"allow_no_sni", "shape_plain"} {
		if value, ok := settings[key]; ok && value != nil {
			if _, isBool := value.(bool); !isBool {
				return fmt.Errorf("ANX %s must be true or false", key)
			}
		}
	}
	if value, ok := settings["max_streams"]; ok && value != nil {
		n, isNumber := value.(float64)
		if !isNumber || n != float64(int64(n)) || n < 1 || n > anxMaxStreamsLimit {
			return fmt.Errorf("ANX max_streams must be a whole number from 1 to %d", anxMaxStreamsLimit)
		}
	}
	if value, ok := settings["wg_listen_port"]; ok && value != nil {
		n, isNumber := value.(float64)
		if !isNumber || n != float64(int64(n)) || n < 1 || n > 65535 {
			return errors.New("ANX wg_listen_port must be a port number from 1 to 65535")
		}
	}
	if value, ok := settings["bare_listen_port"]; ok && value != nil {
		n, isNumber := value.(float64)
		if !isNumber || n != float64(int64(n)) || n < 1 || n > 65535 {
			return errors.New("ANX bare_listen_port must be a port number from 1 to 65535")
		}
		// The bare jacket looks like random bytes, which the GFW flags on
		// cross-border paths: it is for domestic entries only, and those run
		// the no-SNI mode.
		if !ANXNoSNI(settings) {
			return errors.New("ANX bare_listen_port is for domestic entries only and needs allow_no_sni")
		}
		if ANXWGPort(settings) == int(n) {
			return errors.New("ANX bare_listen_port must differ from wg_listen_port")
		}
	}
	if value, ok := settings["dest_stack"]; ok && value != nil {
		stack, isString := value.(string)
		if !isString || !slices.Contains(anxDestStacks, stack) {
			return fmt.Errorf("ANX dest_stack must be one of %s", strings.Join(anxDestStacks, ", "))
		}
	}
	if value, ok := settings["dest_behavior"]; ok && value != nil {
		if err := validateANXDestBehavior(value); err != nil {
			return err
		}
	}
	if value, ok := settings["dest_behaviors"]; ok && value != nil {
		if err := validateANXDestBehaviors(value); err != nil {
			return err
		}
	}
	for _, key := range []string{"wire_gen", "min_wire_gen"} {
		if value, ok := settings[key]; ok && value != nil {
			if n, isNumber := value.(float64); !isNumber || (n != 1 && n != 2) {
				return fmt.Errorf("ANX %s must be 1 or 2", key)
			}
		}
	}
	// Refusing the first generation while still publishing a first-generation
	// key would lock every client out.
	if anxGenSetting(settings, "min_wire_gen") == 2 && anxGenSetting(settings, "wire_gen") != 2 {
		return errors.New("ANX min_wire_gen 2 needs wire_gen 2")
	}
	if value, ok := settings["dest"]; ok && value != nil {
		text, isString := value.(string)
		if !isString {
			return errors.New("ANX dest must be a host:port string")
		}
		host, port, err := net.SplitHostPort(strings.TrimSpace(text))
		if err != nil || host == "" {
			return fmt.Errorf("ANX dest %q must be host:port", text)
		}
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("ANX dest %q has an invalid port", text)
		}
	}
	return nil
}
