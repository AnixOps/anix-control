package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"gorm.io/gorm"
)

// Validate on build, report-only (docs/architecture/node-ops-service.md,
// section 3.8, and decision D7). Before the kernel builds a node's
// configuration from a protocol or a raw configuration, it checks the
// secrets the row holds. A row that fails is reported, with a metric and a
// log line naming the node, the protocol and the field, never the value. It
// is not left out of the configuration: exclusion is enforced one release
// later, once a staging copy shows no finding.

// Why a secret fails validation.
const (
	// NodeSecretReasonPlaceholder: the field holds the administrators' mask
	// (********), so the node would receive the mask as its key.
	NodeSecretReasonPlaceholder = "placeholder"
	// NodeSecretReasonTombstone: the field holds a tombstone of the
	// credential split.
	NodeSecretReasonTombstone = "tombstone"
	// NodeSecretReasonWireGuardKey: an enabled WireGuard entry's
	// server_private_key is missing, or a server_private_key is not a
	// 32-byte standard Base64 X25519 key.
	NodeSecretReasonWireGuardKey = "wireguard_key"
	// NodeSecretReasonWireGuardKeyPair: a WireGuard entry's server public
	// key is not the public key of its server_private_key.
	NodeSecretReasonWireGuardKeyPair = "wireguard_key_pair"
	// NodeSecretReasonRealityKey: a Reality private_key is not a 32-byte
	// Base64 key.
	NodeSecretReasonRealityKey = "reality_key"
	// NodeSecretReasonSS2022Key: a Shadowsocks 2022 server_key does not
	// decode to the cipher's key length.
	NodeSecretReasonSS2022Key = "ss2022_key"
)

// NodeSecretFinding names one secret that fails validation: where it is and
// why, never its value.
type NodeSecretFinding struct {
	NodeID uint `json:"node_id"`
	// ProtocolID is zero for a node's raw configuration.
	ProtocolID uint `json:"protocol_id,omitempty"`
	// Type is the protocol's type, or raw_config.
	Type    string `json:"type"`
	Enabled bool   `json:"enabled"`
	Column  string `json:"column"`
	// Field is the secret's JSON pointer; empty for a whole column.
	Field  string `json:"field"`
	Reason string `json:"reason"`
}

func (f NodeSecretFinding) table() string {
	if f.ProtocolID == 0 {
		return nodesecrets.TableNode
	}
	return nodesecrets.TableNodeProtocol
}

// subject names the finding's row and field for a log line.
func (f NodeSecretFinding) subject() string {
	subject := "node " + strconv.FormatUint(uint64(f.NodeID), 10)
	if f.ProtocolID != 0 {
		subject += " protocol " + strconv.FormatUint(uint64(f.ProtocolID), 10)
	}
	subject += " " + f.Column
	if f.Field != "" {
		subject += " " + f.Field
	}
	return subject
}

// ReportNodeSecretFindings counts and logs each finding of a configuration
// build (anixops_node_secrets_invalid_total, once per subject in the log).
// It excludes nothing.
func ReportNodeSecretFindings(findings []NodeSecretFinding) {
	for _, finding := range findings {
		nodesecrets.RecordInvalid(finding.table(), finding.Type, finding.Reason, finding.subject())
	}
}

// ValidateProtocolSecrets checks the secrets of one node protocol, as the
// configuration builder uses them: no secret position holds the placeholder
// or a tombstone, and the keys the node parses (a WireGuard entry's server
// key pair, a Reality private key, a Shadowsocks 2022 server key) are well
// formed.
func ValidateProtocolSecrets(protocol *model.NodeProtocol) []NodeSecretFinding {
	if protocol == nil {
		return nil
	}
	base := NodeSecretFinding{
		NodeID: protocol.NodeID, ProtocolID: protocol.ID, Type: strings.ToLower(strings.TrimSpace(string(protocol.Type))),
		Enabled: protocol.Enable != 0,
	}
	var findings []NodeSecretFinding
	flagged := map[string]bool{}
	add := func(column, field, reason string) {
		finding := base
		finding.Column, finding.Field, finding.Reason = column, field, reason
		findings = append(findings, finding)
		flagged[column+" "+field] = true
	}
	for _, column := range []struct {
		name  string
		value *string
	}{
		{"settings", protocol.Settings},
		{"tls_settings", protocol.TLSSettings},
		{"transport_settings", protocol.TransportSettings},
		{"reality_settings", protocol.RealitySettings},
		{"custom_config", protocol.CustomConfig},
	} {
		for _, finding := range maskedSecretPositions(column.value) {
			add(column.name, finding.Field, finding.Reason)
		}
	}

	settings := decodeSecretDocument(protocol.Settings)
	switch base.Type {
	case string(model.ProtocolWireGuard):
		if wireGuardRelayRole(settings) == "exit" {
			break
		}
		privateKey := stringSetting(settings, "server_private_key", "")
		if flagged["settings /server_private_key"] {
			break
		}
		if strings.TrimSpace(privateKey) == "" {
			if base.Enabled {
				add("settings", "/server_private_key", NodeSecretReasonWireGuardKey)
			}
			break
		}
		derived, err := validateWireGuardKey(privateKey, "server_private_key")
		if err != nil {
			add("settings", "/server_private_key", NodeSecretReasonWireGuardKey)
			break
		}
		for _, key := range []string{"server_public_key", "public_key"} {
			if public := strings.TrimSpace(stringSetting(settings, key, "")); public != "" {
				if public != derived {
					add("settings", "/"+key, NodeSecretReasonWireGuardKeyPair)
				}
				break
			}
		}
	case string(model.ProtocolShadowsocks):
		cipher := firstStringSetting(settings, "cipher", "method")
		serverKey := stringSetting(settings, "server_key", "")
		if !strings.HasPrefix(cipher, "2022-blake3-") || serverKey == "" || flagged["settings /server_key"] {
			break
		}
		for _, part := range strings.Split(serverKey, ":") {
			if decoded, ok := decodeBase64Key(part); !ok || len(decoded) != ss2022KeyLen(cipher) {
				add("settings", "/server_key", NodeSecretReasonSS2022Key)
				break
			}
		}
	}

	if protocol.TLS == 2 {
		reality := decodeSecretDocument(protocol.RealitySettings)
		for _, key := range []string{"private_key", "privateKey"} {
			value, ok := reality[key].(string)
			if !ok || strings.TrimSpace(value) == "" || flagged["reality_settings /"+key] {
				continue
			}
			if decoded, ok := decodeBase64Key(value); !ok || len(decoded) != 32 {
				add("reality_settings", "/"+key, NodeSecretReasonRealityKey)
			}
		}
	}
	return findings
}

// ValidateRawConfigSecrets checks the secrets of a node's raw
// configuration: no secret position holds the placeholder or a tombstone.
// The kernel already refuses a WireGuard raw configuration that fails its
// runtime validation.
func ValidateRawConfigSecrets(node *model.Node) []NodeSecretFinding {
	if node == nil {
		return nil
	}
	var findings []NodeSecretFinding
	for _, finding := range maskedSecretPositions(node.RawConfig) {
		finding.NodeID, finding.Type, finding.Enabled, finding.Column = node.ID, "raw_config", true, "raw_config"
		findings = append(findings, finding)
	}
	return findings
}

// maskedSecretPositions answers the secret positions of a column that hold
// the placeholder or a tombstone, with Field and Reason set.
func maskedSecretPositions(document *string) []NodeSecretFinding {
	if document == nil {
		return nil
	}
	var findings []NodeSecretFinding
	for _, position := range nodesecrets.SecretPositions(*document) {
		value := position.Value
		if position.Pointer != "" {
			var text string
			if err := json.Unmarshal([]byte(position.Value), &text); err != nil {
				continue
			}
			value = text
		}
		value = strings.TrimSpace(value)
		switch {
		case position.Moved || value == nodesecrets.Placeholder:
			findings = append(findings, NodeSecretFinding{Field: position.Pointer, Reason: NodeSecretReasonPlaceholder})
		case nodesecrets.IsTombstone(value):
			findings = append(findings, NodeSecretFinding{Field: position.Pointer, Reason: NodeSecretReasonTombstone})
		}
	}
	return findings
}

func decodeSecretDocument(document *string) map[string]any {
	if document == nil || strings.TrimSpace(*document) == "" {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(*document), &decoded); err != nil {
		return nil
	}
	return decoded
}

// decodeBase64Key decodes a key in any of the Base64 forms the node
// software accepts: standard or URL alphabet, padded or not.
func decodeBase64Key(value string) ([]byte, bool) {
	value = strings.TrimSpace(value)
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if decoded, err := encoding.DecodeString(value); err == nil {
			return decoded, true
		}
	}
	return nil, false
}

// NodeSecretValidation is the outcome of ScanNodeSecrets.
type NodeSecretValidation struct {
	// Protocols and RawConfigs count the rows checked.
	Protocols  int64               `json:"protocols"`
	RawConfigs int64               `json:"raw_configs"`
	Findings   []NodeSecretFinding `json:"findings"`
	// Excluded is always zero: validation reports and excludes nothing
	// (D7).
	Excluded int `json:"excluded"`
}

// nodeSecretScanBatch bounds the rows of one scan query.
const nodeSecretScanBatch = 500

// ScanNodeSecrets checks every node protocol and every node raw
// configuration as the configuration builder would use them (their secrets
// read through the node credential split, in the table's phase), and
// reports the findings. It changes nothing.
func ScanNodeSecrets(ctx context.Context, db *gorm.DB) (*NodeSecretValidation, error) {
	result := &NodeSecretValidation{Findings: []NodeSecretFinding{}}
	var protocols []model.NodeProtocol
	err := db.WithContext(ctx).Order("id").FindInBatches(&protocols, nodeSecretScanBatch, func(*gorm.DB, int) error {
		nodesecrets.ResolveProtocols(db, protocols)
		for i := range protocols {
			result.Findings = append(result.Findings, ValidateProtocolSecrets(&protocols[i])...)
		}
		result.Protocols += int64(len(protocols))
		return nil
	}).Error
	if err != nil {
		return nil, err
	}
	var nodes []model.Node
	err = db.WithContext(ctx).Select("id", "raw_config").Where("raw_config IS NOT NULL AND raw_config <> ''").Order("id").
		FindInBatches(&nodes, nodeSecretScanBatch, func(*gorm.DB, int) error {
			for i := range nodes {
				nodesecrets.ResolveNodeRawConfig(db, &nodes[i])
				result.Findings = append(result.Findings, ValidateRawConfigSecrets(&nodes[i])...)
			}
			result.RawConfigs += int64(len(nodes))
			return nil
		}).Error
	if err != nil {
		return nil, err
	}
	return result, nil
}
