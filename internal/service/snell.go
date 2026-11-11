package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// Snell nodes: anixcore serves Snell (networkcore_anixops
// crates/inbound-snell) on TCP with the protocol's own encryption. The
// kernel takes one PSK per inbound and tells users apart by the key each
// request names (the Surge userkey), so a protocol stores the shared psk
// and a user's key is their UUID: traffic and online IPs stay per user.
// The Agent (anix-agent core/anixcore) sends the same keys. A client that
// cannot name a user key cannot log in.
const defaultSnellVersion = 4

// snellModes are the version 6 modes of the kernel's inbound.
var snellModes = []string{"default", "unshaped", "unsafe-raw"}

// buildSnellConfig sets the node config fields of a Snell protocol; obfs
// and mode go out only when set.
func buildSnellConfig(config, settings map[string]any) {
	config["psk"] = stringSetting(settings, "psk", "")
	config["version"] = intSetting(settings, "version", defaultSnellVersion)
	for _, key := range []string{"obfs", "mode"} {
		if value := stringSetting(settings, key, ""); value != "" {
			config[key] = value
		}
	}
}

// validateSnellProtocol checks a Snell protocol against what the kernel's
// inbound accepts: no TLS, no transport, a psk (12 bytes or more for
// version 6), version 1 to 6, obfs http up to version 4 and a mode for
// version 6 only.
func validateSnellProtocol(protocol *model.NodeProtocol) error {
	settings := map[string]any{}
	if protocol.Settings != nil && strings.TrimSpace(*protocol.Settings) != "" {
		if err := json.Unmarshal([]byte(*protocol.Settings), &settings); err != nil {
			return fmt.Errorf("snell settings must be a JSON object: %v", err)
		}
	}
	if protocol.TLS != 0 {
		return errors.New("snell has its own encryption and takes no TLS")
	}
	if protocol.Transport != nil && *protocol.Transport != "" && !strings.EqualFold(strings.TrimSpace(*protocol.Transport), "tcp") {
		return errors.New("snell runs on TCP only")
	}
	// Whitespace, control characters and ',' would break the Surge proxy line.
	psk := stringSetting(settings, "psk", "")
	if psk == "" || len(psk) > 255 || strings.ContainsFunc(psk, func(r rune) bool { return r <= ' ' || r == ',' || r == 0x7f }) {
		return errors.New("snell psk must be 1 to 255 bytes without spaces, control characters or commas")
	}
	version := intSetting(settings, "version", defaultSnellVersion)
	if version < 1 || version > 6 {
		return errors.New("snell version must be from 1 to 6")
	}
	// The official version 6 server and clients take 12 bytes or more; the
	// placeholder stands for a stored psk when a write is checked first.
	if version == 6 && len(psk) < 12 && psk != NodeSecretPlaceholder {
		return errors.New("snell version 6 needs a psk of at least 12 bytes")
	}
	switch obfs := stringSetting(settings, "obfs", ""); {
	case obfs == "":
	case obfs != "http":
		return errors.New("snell obfs must be http or empty")
	case version > 4:
		return errors.New("snell obfs applies to versions 1 to 4 only")
	}
	switch mode := stringSetting(settings, "mode", ""); {
	case mode == "":
	case !slices.Contains(snellModes, mode):
		return fmt.Errorf("snell mode must be one of %s", strings.Join(snellModes, ", "))
	case version != 6:
		return errors.New("snell mode applies to version 6 only")
	}
	return nil
}
