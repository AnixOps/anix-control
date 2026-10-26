package service

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// ensureRealityKeyPair completes the key pair of a Reality protocol (tls 2):
// one without a private key gets a new X25519 key pair, and one with a
// private key but no public key gets that key's public key. The Agent
// cannot build a Reality inbound without the private key, and it refuses
// the node's whole configuration then; subscriptions hand out public_key.
// A private key that does not decode is left for the secret validation to
// report. It answers whether it changed RealitySettings.
func ensureRealityKeyPair(protocol *model.NodeProtocol) (bool, error) {
	if protocol == nil || protocol.TLS != 2 {
		return false, nil
	}
	settings := map[string]any{}
	if protocol.RealitySettings != nil && strings.TrimSpace(*protocol.RealitySettings) != "" {
		if err := json.Unmarshal([]byte(*protocol.RealitySettings), &settings); err != nil || settings == nil {
			// Not an object: the configuration builder reports it.
			return false, nil
		}
	}
	private := realitySetting(settings, "private_key", "privateKey")
	public := realitySetting(settings, "public_key", "publicKey", "pbk")
	switch {
	case private == "":
		key, err := ecdh.X25519().GenerateKey(rand.Reader)
		if err != nil {
			return false, err
		}
		delete(settings, "privateKey")
		delete(settings, "publicKey")
		delete(settings, "pbk")
		settings["private_key"] = base64.RawURLEncoding.EncodeToString(key.Bytes())
		settings["public_key"] = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	case public == "":
		decoded, ok := decodeBase64Key(private)
		if !ok {
			return false, nil
		}
		key, err := ecdh.X25519().NewPrivateKey(decoded)
		if err != nil {
			return false, nil
		}
		settings["public_key"] = base64.RawURLEncoding.EncodeToString(key.PublicKey().Bytes())
	default:
		return false, nil
	}
	encoded, err := json.Marshal(settings)
	if err != nil {
		return false, err
	}
	text := string(encoded)
	protocol.RealitySettings = &text
	return true, nil
}

// realitySetting is the first non-empty string of keys in settings.
func realitySetting(settings map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := settings[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
