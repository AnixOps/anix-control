package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
)

// NodeSecretPlaceholder stands for a node secret in the administrator's
// answers: a secret setting of a node protocol or of a node's raw
// configuration, a forward node's API token, a registration key. Sending it
// back in a write keeps the stored secret, as for system configuration and
// payment gateway secrets. Nodes and agents read their configuration through
// their own authenticated paths (UniProxy, the gRPC node service, agent
// control), which build it from the stored rows and never see the
// placeholder.
const NodeSecretPlaceholder = SensitiveSystemConfigPlaceholder

// nodeProtocolSecretColumns are the JSON columns of v2_node_protocol that can
// hold secrets. A protocol keeps no secret in a column of its own.
var nodeProtocolSecretColumns = []string{"settings", "tls_settings", "transport_settings", "reality_settings", "custom_config"}

// IsNodeSecretKey reports whether a key of a node protocol's settings, or of
// a node's raw configuration, names a secret: Reality's and TLS's
// private_key (privateKey), WireGuard's server_private_key and
// preshared_key, Shadowsocks' server_key, psk and password, the Hysteria2
// obfs-password and auth, and any other *_key, password, passwd, pass,
// secret, token, credential or seed. Public keys, Reality's short_id and the
// paths of key files (key_file, wss_key_file) are not secrets. The rule is
// nodesecrets.IsSecretKey, which also places the secrets the credential
// split moves.
func IsNodeSecretKey(key string) bool {
	return nodesecrets.IsSecretKey(key)
}

// RedactNodeSecretsJSON replaces the secret values of a node protocol
// setting or a raw configuration (JSON text) with NodeSecretPlaceholder. A
// string or array under a secret key is replaced whole; an object's own keys
// are checked, and so are the objects in arrays. A value without a secret is
// returned unchanged, and one that is not JSON is replaced whole, since it
// cannot be told apart.
//
// The rule is nodesecrets.Redact, which also writes the documents a
// finalized table keeps (section 4.4), so the stored redacted document and
// this answer are the same bytes.
func RedactNodeSecretsJSON(config string) string {
	return nodesecrets.Redact(config)
}

// KeepNodeSecretsJSON returns an incoming node protocol setting or raw
// configuration (JSON text) in which every secret sent as
// NodeSecretPlaceholder has its stored value (an empty string when nothing
// is stored), so an administrator can save what a masked answer showed
// without retyping its secrets. A placeholder sent for the whole value keeps
// the whole stored value. Arrays are matched by position.
func KeepNodeSecretsJSON(incoming, stored string) string {
	return v2compat.KeepNodeSecrets(incoming, stored)
}

// decodeNodeSecretJSON decodes one JSON value, keeping numbers as written.
func decodeNodeSecretJSON(config string) (any, bool) {
	decoder := json.NewDecoder(strings.NewReader(config))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return value, true
}

func encodeNodeSecretJSON(value any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return NodeSecretPlaceholder
	}
	return strings.TrimSuffix(buffer.String(), "\n")
}

// RedactNodeProtocol masks the secrets in a node protocol's settings for an
// administrator's answer. It changes the protocol it is given, so it is for
// a row read for that answer only, never one that is saved afterwards. The
// protocol's node, when loaded, has its raw configuration masked too.
func RedactNodeProtocol(protocol *model.NodeProtocol) {
	if protocol == nil {
		return
	}
	for _, field := range nodeProtocolSecretFields(protocol) {
		if *field != nil {
			redacted := RedactNodeSecretsJSON(**field)
			*field = &redacted
		}
	}
	if protocol.Node != nil {
		RedactNode(protocol.Node)
	}
}

// RedactNodeProtocols masks the protocols of an administrator's answer.
func RedactNodeProtocols(protocols []model.NodeProtocol) {
	for i := range protocols {
		RedactNodeProtocol(&protocols[i])
	}
}

// RedactNode masks a node's raw configuration and the secrets of its loaded
// protocols for an administrator's answer (see RedactNodeProtocol). The
// node's API key and shared secret are never serialized (json:"-").
func RedactNode(node *model.Node) {
	if node == nil {
		return
	}
	if node.RawConfig != nil {
		redacted := RedactNodeSecretsJSON(*node.RawConfig)
		node.RawConfig = &redacted
	}
	for i := range node.Protocols {
		RedactNodeProtocol(&node.Protocols[i])
	}
}

// keepNodeProtocolSecrets gives the settings of a protocol about to be
// created the values its placeholders stand for: none, since nothing is
// stored yet.
func keepNodeProtocolSecrets(protocol *model.NodeProtocol) {
	for _, field := range nodeProtocolSecretFields(protocol) {
		if *field != nil {
			kept := KeepNodeSecretsJSON(**field, "")
			*field = &kept
		}
	}
}

// keepNodeProtocolSecretUpdates gives the settings of a protocol update (by
// column name, as normalizeProtocolUpdates leaves them) the stored values
// their placeholders stand for.
func keepNodeProtocolSecretUpdates(updates map[string]any, current *model.NodeProtocol) {
	stored := map[string]*string{
		"settings":           current.Settings,
		"tls_settings":       current.TLSSettings,
		"transport_settings": current.TransportSettings,
		"reality_settings":   current.RealitySettings,
		"custom_config":      current.CustomConfig,
	}
	for _, column := range nodeProtocolSecretColumns {
		text, ok := updates[column].(string)
		if !ok {
			continue
		}
		previous := ""
		if stored[column] != nil {
			previous = *stored[column]
		}
		updates[column] = KeepNodeSecretsJSON(text, previous)
	}
}

func nodeProtocolSecretFields(protocol *model.NodeProtocol) []**string {
	return []**string{
		&protocol.Settings, &protocol.TLSSettings, &protocol.TransportSettings,
		&protocol.RealitySettings, &protocol.CustomConfig,
	}
}

// KeepNodeRawConfig returns an incoming raw configuration with the stored
// values of the secrets it sends as NodeSecretPlaceholder (see
// KeepNodeSecretsJSON).
func KeepNodeRawConfig(incoming string, stored *string) string {
	previous := ""
	if stored != nil {
		previous = *stored
	}
	return KeepNodeSecretsJSON(incoming, previous)
}

// MaskAuthorizedKeys masks registration keys for an administrator's
// answer. A key is shown once, in the answer that generates it; a node
// registers with it by its hash.
func MaskAuthorizedKeys(keys []model.AuthorizedKey) {
	for i := range keys {
		if keys[i].Key != "" {
			keys[i].Key = NodeSecretPlaceholder
		}
	}
}
