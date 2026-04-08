package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	appconfig "github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const forwardRuntimeBootstrapRemark = "Forward runtime bootstrapped from config.yaml"

type forwardRuntimeBootstrapConfig struct {
	NodeXMode *bool
	Backend   string
	NodeX     appconfig.ForwardRuntimeNodeXConfig
	Ansible   panelForwardAnsibleConfig
}

func InitForwardRuntimeSystemConfig(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	configService := NewSystemConfigService(db)
	bootstrapCfg, err := loadForwardRuntimeBootstrapConfig()
	if err != nil {
		return err
	}

	effectiveBackend := bootstrapCfg.Backend
	if bootstrapCfg.NodeXMode != nil {
		effectiveBackend = forwardRuntimeBackendForMode(*bootstrapCfg.NodeXMode)
	}

	if err := syncForwardRuntimeNodeXMode(configService, bootstrapCfg.NodeXMode); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeBackendConfigKey, effectiveBackend, "string"); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeNodeXBaseURLConfigKey, strings.TrimSpace(bootstrapCfg.NodeX.BaseURL), "string"); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeNodeXTokenConfigKey, strings.TrimSpace(bootstrapCfg.NodeX.Token), "string"); err != nil {
		return err
	}

	nodeXTimeout := ""
	if bootstrapCfg.NodeX.TimeoutSeconds > 0 {
		nodeXTimeout = strconv.Itoa(bootstrapCfg.NodeX.TimeoutSeconds)
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeNodeXTimeoutSecondsConfigKey, nodeXTimeout, "int"); err != nil {
		return err
	}

	if effectiveBackend == model.ForwardRuntimeBackendGost {
		if strings.TrimSpace(bootstrapCfg.NodeX.BaseURL) == "" {
			return fmt.Errorf("%s is required for NodeX forward runtime", forwardRuntimeNodeXBaseURLConfigKey)
		}
		if strings.TrimSpace(bootstrapCfg.NodeX.Token) == "" {
			return fmt.Errorf("%s is required for NodeX forward runtime", forwardRuntimeNodeXTokenConfigKey)
		}
	}

	if shouldPersistForwardRuntimeAnsibleConfig(bootstrapCfg, effectiveBackend) {
		bootstrapCfg.Ansible.ensureDefaults()
		return syncForwardRuntimeAnsibleConfig(configService, &bootstrapCfg.Ansible)
	}

	return clearForwardRuntimeAnsibleConfig(configService)
}

func syncForwardRuntimeNodeXMode(configService *SystemConfigService, value *bool) error {
	serialized := ""
	if value != nil {
		serialized = strconv.FormatBool(*value)
	}
	return syncForwardRuntimeValue(configService, forwardRuntimeNodeXModeConfigKey, serialized, "bool")
}

func syncForwardRuntimeValue(configService *SystemConfigService, key, value, cfgType string) error {
	return configService.Set(key, value, cfgType, forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark)
}

func syncForwardRuntimeAnsibleConfig(configService *SystemConfigService, cfg *panelForwardAnsibleConfig) error {
	if cfg == nil {
		return clearForwardRuntimeAnsibleConfig(configService)
	}

	cfg.ensureDefaults()
	ansibleJSON, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	extraVarsJSON, err := json.Marshal(cfg.ExtraVars)
	if err != nil {
		return err
	}

	if err := syncForwardRuntimeValue(configService, forwardRuntimeAnsibleConfigJSONKey, string(ansibleJSON), "json"); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeAnsibleInventoryConfigKey, cfg.Inventory, "string"); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeAnsibleApplyPlaybookConfigKey, cfg.ApplyPlaybook, "string"); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeAnsibleRemovePlaybookConfigKey, cfg.RemovePlaybook, "string"); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeAnsibleBecomeConfigKey, strconv.FormatBool(cfg.Become), "bool"); err != nil {
		return err
	}
	return syncForwardRuntimeValue(configService, forwardRuntimeAnsibleExtraVarsConfigKey, string(extraVarsJSON), "json")
}

func clearForwardRuntimeAnsibleConfig(configService *SystemConfigService) error {
	keys := []struct {
		Key  string
		Type string
	}{
		{Key: forwardRuntimeAnsibleConfigJSONKey, Type: "json"},
		{Key: forwardRuntimeAnsibleInventoryConfigKey, Type: "string"},
		{Key: forwardRuntimeAnsibleApplyPlaybookConfigKey, Type: "string"},
		{Key: forwardRuntimeAnsibleRemovePlaybookConfigKey, Type: "string"},
		{Key: forwardRuntimeAnsibleBecomeConfigKey, Type: "bool"},
		{Key: forwardRuntimeAnsibleExtraVarsConfigKey, Type: "json"},
	}
	for _, item := range keys {
		if err := syncForwardRuntimeValue(configService, item.Key, "", item.Type); err != nil {
			return err
		}
	}
	return nil
}

func loadForwardRuntimeBootstrapConfig() (*forwardRuntimeBootstrapConfig, error) {
	if cfg := appconfig.Get(); cfg != nil {
		return buildForwardRuntimeBootstrapConfig(cfg.ForwardRuntime)
	}
	return &forwardRuntimeBootstrapConfig{}, nil
}

func buildForwardRuntimeBootstrapConfig(cfg appconfig.ForwardRuntimeConfig) (*forwardRuntimeBootstrapConfig, error) {
	backend, err := normalizeForwardRuntimeBootstrapBackend(cfg.Backend, "forward_runtime.backend")
	if err != nil {
		return nil, err
	}

	return &forwardRuntimeBootstrapConfig{
		NodeXMode: cfg.NodeXMode,
		Backend:   backend,
		NodeX: appconfig.ForwardRuntimeNodeXConfig{
			BaseURL:        strings.TrimSpace(cfg.NodeX.BaseURL),
			Token:          strings.TrimSpace(cfg.NodeX.Token),
			TimeoutSeconds: cfg.NodeX.TimeoutSeconds,
		},
		Ansible: panelForwardAnsibleConfig{
			Inventory:      strings.TrimSpace(cfg.IptablesAnsible.Inventory),
			ApplyPlaybook:  strings.TrimSpace(cfg.IptablesAnsible.ApplyPlaybook),
			RemovePlaybook: strings.TrimSpace(cfg.IptablesAnsible.RemovePlaybook),
			Become:         cfg.IptablesAnsible.Become,
			ExtraVars:      cloneForwardRuntimeExtraVars(cfg.IptablesAnsible.ExtraVars),
			Command:        strings.TrimSpace(cfg.IptablesAnsible.Command),
			WorkingDir:     strings.TrimSpace(cfg.IptablesAnsible.WorkingDir),
			TargetPattern:  strings.TrimSpace(cfg.IptablesAnsible.TargetPattern),
			Environment:    cloneForwardRuntimeEnvironment(cfg.IptablesAnsible.Environment),
			TimeoutSeconds: cfg.IptablesAnsible.TimeoutSeconds,
		},
	}, nil
}

func shouldPersistForwardRuntimeAnsibleConfig(cfg *forwardRuntimeBootstrapConfig, backend string) bool {
	if cfg == nil {
		return false
	}
	return backend == model.ForwardRuntimeBackendIptablesAnsible ||
		strings.TrimSpace(cfg.Ansible.Inventory) != "" ||
		strings.TrimSpace(cfg.Ansible.ApplyPlaybook) != "" ||
		strings.TrimSpace(cfg.Ansible.RemovePlaybook) != "" ||
		strings.TrimSpace(cfg.Ansible.WorkingDir) != "" ||
		strings.TrimSpace(cfg.Ansible.TargetPattern) != "" ||
		strings.TrimSpace(cfg.Ansible.Command) != "" ||
		cfg.Ansible.TimeoutSeconds > 0 ||
		cfg.Ansible.Become ||
		len(cfg.Ansible.ExtraVars) > 0 ||
		len(cfg.Ansible.Environment) > 0
}

func normalizeForwardRuntimeBootstrapBackend(value, key string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if backend, ok := normalizeForwardRuntimeBackend(trimmed); ok {
		return backend, nil
	}
	return "", fmt.Errorf("invalid %s value: %s", key, value)
}

func cloneForwardRuntimeExtraVars(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]interface{}, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneForwardRuntimeEnvironment(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
