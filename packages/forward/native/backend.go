package native

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// The system configuration keys that choose the forward runtime backend,
// the rows kapi_forward_runtime_settings_v1 shows.
const (
	nodeXModeKey           = "forward.runtime.nodex_mode"
	runtimeBackendKey      = "forward.runtime_backend"
	localAnsibleBackendKey = "forward.runtime.ansible.backend"
)

// runtimeSetting is a backend key's value, "" when it is not set, as the
// kernel's SystemConfigService.Get reads it.
func runtimeSetting(db *gorm.DB, key string) (string, error) {
	var rows []RuntimeSetting
	if err := db.Where("key = ?", key).Limit(1).Find(&rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", nil
	}
	return rows[0].Value, nil
}

// resolveRuntimeBackend is the backend new forwards run on: gost in NodeX
// mode, the local Ansible backend when NodeX mode is off, else the
// configured backend (gost when unset).
func resolveRuntimeBackend(db *gorm.DB) (string, error) {
	mode, err := runtimeSetting(db, nodeXModeKey)
	if err != nil {
		return "", err
	}
	nodeXMode, err := parseBoolSetting(mode, nodeXModeKey)
	if err != nil {
		return "", err
	}
	if nodeXMode != nil {
		if *nodeXMode {
			return backendGost, nil
		}
		return resolveLocalAnsibleBackend(db)
	}
	value, err := runtimeSetting(db, runtimeBackendKey)
	if err != nil {
		return "", err
	}
	switch backend, ok := normalizeRuntimeBackend(value); {
	case strings.TrimSpace(value) == "":
		return backendGost, nil
	case ok:
		return backend, nil
	default:
		return "", fmt.Errorf("invalid %s value: %s", runtimeBackendKey, value)
	}
}

func resolveLocalAnsibleBackend(db *gorm.DB) (string, error) {
	for _, key := range []string{localAnsibleBackendKey, runtimeBackendKey} {
		value, err := runtimeSetting(db, key)
		if err != nil {
			return "", err
		}
		if backend, ok := normalizeLocalAnsibleBackend(value); ok {
			return backend, nil
		}
	}
	return defaultLocalAnsibleMode, nil
}

func normalizeRuntimeBackend(value string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case backendGost:
		return backendGost, true
	case backendNftablesAnsible, backendIptablesAnsible:
		// iptables is retired and runs as nftables.
		return backendNftablesAnsible, true
	case backendCleanAgent:
		return backendCleanAgent, true
	default:
		return "", false
	}
}

func normalizeLocalAnsibleBackend(value string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case backendNftablesAnsible, backendIptablesAnsible:
		return backendNftablesAnsible, true
	default:
		return "", false
	}
}

// isExecutionNodeBackend reports whether forwards on the backend run on one
// execution node rather than an entry and an exit node.
func isExecutionNodeBackend(backend string) bool {
	_, local := normalizeLocalAnsibleBackend(backend)
	return local || backend == backendCleanAgent
}

func parseBoolSetting(value, key string) (*bool, error) {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "":
		return nil, nil
	case "1", "true", "yes", "on":
		enabled := true
		return &enabled, nil
	case "0", "false", "no", "off":
		enabled := false
		return &enabled, nil
	default:
		return nil, fmt.Errorf("invalid %s value: %s", key, value)
	}
}
