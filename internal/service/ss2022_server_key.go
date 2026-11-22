package service

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/model"
)

// validateShadowsocksProtocol checks that a Shadowsocks 2022 protocol (a
// 2022-blake3-* cipher) has its server_key: nodes and subscriptions use the
// stored key only (0041_ss2022_server_keys stored the keys protocols of
// earlier releases were derived). The admin form generates a random one;
// the secret validation checks its length.
func validateShadowsocksProtocol(protocol *model.NodeProtocol) error {
	settings := map[string]any{}
	if protocol.Settings != nil && strings.TrimSpace(*protocol.Settings) != "" {
		if err := json.Unmarshal([]byte(*protocol.Settings), &settings); err != nil {
			return nil // the configuration builder reports a document that is not an object
		}
	}
	if !strings.HasPrefix(firstStringSetting(settings, "cipher", "method"), "2022-blake3-") {
		return nil
	}
	if strings.TrimSpace(stringSetting(settings, "server_key", "")) == "" {
		return errors.New("a Shadowsocks 2022 cipher needs a server_key")
	}
	return nil
}
