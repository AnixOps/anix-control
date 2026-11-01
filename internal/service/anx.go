package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

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
