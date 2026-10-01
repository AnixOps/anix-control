package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/service"
)

// gatewaySecretPlaceholder stands for a payment gateway secret in the
// administrator's gateway responses. Sending it back in a configuration
// keeps the stored secret, as for system configuration secrets.
const gatewaySecretPlaceholder = service.SensitiveSystemConfigPlaceholder

// gatewaySecretKey reports whether a gateway configuration key names a
// secret: EPay's merchant "key", Stripe's secret_key and webhook_secret,
// PayPal's client_secret, Alipay's private_key, WeChat Pay's api_key and
// api_v3_key, and any other *_key, password, token or secret. Public keys
// and Stripe's publishable key are not secrets. The payment package's
// native routes apply the same rule (packages/payment/native).
func gatewaySecretKey(key string) bool {
	name := strings.ToLower(strings.TrimSpace(key))
	switch name {
	case "key":
		return true
	case "public_key", "publishable_key":
		return false
	}
	if strings.HasSuffix(name, "_key") {
		return true
	}
	for _, marker := range []string{"secret", "password", "passwd", "token", "private"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return false
}

// redactGatewayConfig replaces the secret values of a gateway configuration
// (a JSON object) with gatewaySecretPlaceholder. A configuration without a
// secret is returned unchanged; one that is not a JSON object is replaced
// whole, since it cannot be told apart.
func redactGatewayConfig(config string) string {
	if strings.TrimSpace(config) == "" {
		return config
	}
	object, ok := decodeGatewayConfig(config)
	if !ok {
		return gatewaySecretPlaceholder
	}
	if !redactGatewaySecrets(object) {
		return config
	}
	return encodeGatewayConfig(object)
}

func redactGatewaySecrets(object map[string]any) bool {
	changed := false
	for key, value := range object {
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" && gatewaySecretKey(key) {
				object[key] = gatewaySecretPlaceholder
				changed = true
			}
		case map[string]any:
			if redactGatewaySecrets(typed) {
				changed = true
			}
		}
	}
	return changed
}

// keepGatewaySecrets returns an incoming gateway configuration in which
// every secret sent as gatewaySecretPlaceholder has its stored value (none
// when nothing is stored), so an administrator can save a configuration
// read from a redacted response without retyping its secrets.
func keepGatewaySecrets(incoming, stored string) string {
	object, ok := decodeGatewayConfig(incoming)
	if !ok || object == nil {
		return incoming
	}
	previous, _ := decodeGatewayConfig(stored)
	if !restoreGatewaySecrets(object, previous) {
		return incoming
	}
	return encodeGatewayConfig(object)
}

func restoreGatewaySecrets(object, stored map[string]any) bool {
	changed := false
	for key, value := range object {
		switch typed := value.(type) {
		case string:
			if typed == gatewaySecretPlaceholder && gatewaySecretKey(key) {
				if previous, ok := stored[key]; ok {
					object[key] = previous
				} else {
					object[key] = ""
				}
				changed = true
			}
		case map[string]any:
			nested, _ := stored[key].(map[string]any)
			if restoreGatewaySecrets(typed, nested) {
				changed = true
			}
		}
	}
	return changed
}

// decodeGatewayConfig decodes a configuration that is one JSON object (or
// null), keeping numbers as written.
func decodeGatewayConfig(config string) (map[string]any, bool) {
	decoder := json.NewDecoder(strings.NewReader(config))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, false
	}
	return object, true
}

func encodeGatewayConfig(object map[string]any) string {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return gatewaySecretPlaceholder
	}
	return strings.TrimSuffix(buffer.String(), "\n")
}
