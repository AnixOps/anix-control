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
	NodeXMode     *bool
	Backend       string
	LocalBackend  string
	NodeX         appconfig.ForwardRuntimeNodeXConfig
	Ansible       panelForwardAnsibleConfig
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
		if *bootstrapCfg.NodeXMode {
			effectiveBackend = model.ForwardRuntimeBackendGost
		} else {
			effectiveBackend = normalizeForwardRuntimeLocalBackendOrDefault(bootstrapCfg.LocalBackend)
		}
	}
	localBackend := normalizeForwardRuntimeLocalBackendOrDefault(bootstrapCfg.LocalBackend)

	if err := syncForwardRuntimeNodeXMode(configService, bootstrapCfg.NodeXMode); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeBackendConfigKey, effectiveBackend, "string"); err != nil {
		return err
	}
	if err := syncForwardRuntimeValue(configService, forwardRuntimeLocalBackendConfigKey, localBackend, "string"); err != nil {
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

	if shouldPersistForwardRuntimeAnsibleConfig(bootstrapCfg, localBackend) {
		bootstrapCfg.Ansible.ensureDefaults(localBackend)
		return syncForwardRuntimeAnsibleConfig(configService, localBackend, &bootstrapCfg.Ansible)
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
	shouldSync, err := shouldSyncForwardRuntimeValue(configService, key)
	if err != nil {
		return err
	}
	if !shouldSync {
		return nil
	}
	return configService.Set(key, value, cfgType, forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark)
}

func shouldSyncForwardRuntimeValue(configService *SystemConfigService, key string) (bool, error) {
	if configService == nil || configService.db == nil {
		return false, nil
	}

	var cfg model.SystemConfig
	err := configService.db.Where("key = ?", key).First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	return strings.TrimSpace(cfg.Remark) == forwardRuntimeBootstrapRemark, nil
}

func syncForwardRuntimeAnsibleConfig(configService *SystemConfigService, backend string, cfg *panelForwardAnsibleConfig) error {
	if cfg == nil {
		return clearForwardRuntimeAnsibleConfig(configService)
	}

	cfg.ensureDefaults(backend)
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
	if err := syncForwardRuntimeValue(configService, forwardRuntimeAnsibleExtraVarsConfigKey, string(extraVarsJSON), "json"); err != nil {
		return err
	}

	legacyKeys := []struct {
		Key  string
		Type string
	}{
		{Key: legacyForwardRuntimeAnsibleConfigJSONKey, Type: "json"},
		{Key: legacyForwardRuntimeAnsibleInventoryConfigKey, Type: "string"},
		{Key: legacyForwardRuntimeAnsibleApplyPlaybookConfigKey, Type: "string"},
		{Key: legacyForwardRuntimeAnsibleRemovePlaybookConfigKey, Type: "string"},
		{Key: legacyForwardRuntimeAnsibleBecomeConfigKey, Type: "bool"},
		{Key: legacyForwardRuntimeAnsibleExtraVarsConfigKey, Type: "json"},
	}
	for _, item := range legacyKeys {
		if err := syncForwardRuntimeValue(configService, item.Key, "", item.Type); err != nil {
			return err
		}
	}
	return nil
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
		{Key: legacyForwardRuntimeAnsibleConfigJSONKey, Type: "json"},
		{Key: legacyForwardRuntimeAnsibleInventoryConfigKey, Type: "string"},
		{Key: legacyForwardRuntimeAnsibleApplyPlaybookConfigKey, Type: "string"},
		{Key: legacyForwardRuntimeAnsibleRemovePlaybookConfigKey, Type: "string"},
		{Key: legacyForwardRuntimeAnsibleBecomeConfigKey, Type: "bool"},
		{Key: legacyForwardRuntimeAnsibleExtraVarsConfigKey, Type: "json"},
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

	localBackend, ansibleCfg := selectForwardRuntimeBootstrapAnsibleConfig(cfg, backend)

	return &forwardRuntimeBootstrapConfig{
		NodeXMode:    cfg.NodeXMode,
		Backend:      backend,
		LocalBackend: localBackend,
		NodeX: appconfig.ForwardRuntimeNodeXConfig{
			BaseURL:        strings.TrimSpace(cfg.NodeX.BaseURL),
			Token:          strings.TrimSpace(cfg.NodeX.Token),
			TimeoutSeconds: cfg.NodeX.TimeoutSeconds,
		},
		Ansible: ansibleCfg,
	}, nil
}

func shouldPersistForwardRuntimeAnsibleConfig(cfg *forwardRuntimeBootstrapConfig, backend string) bool {
	if cfg == nil {
		return false
	}
	return isForwardRuntimeLocalAnsibleBackend(backend) ||
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

func selectForwardRuntimeBootstrapAnsibleConfig(cfg appconfig.ForwardRuntimeConfig, backend string) (string, panelForwardAnsibleConfig) {
	switch backend {
	case model.ForwardRuntimeBackendNftablesAnsible:
		return model.ForwardRuntimeBackendNftablesAnsible, mapBootstrapAnsibleConfig(cfg.NftablesAnsible)
	case model.ForwardRuntimeBackendIptablesAnsible:
		return model.ForwardRuntimeBackendIptablesAnsible, mapBootstrapAnsibleConfig(cfg.IptablesAnsible)
	}

	if hasForwardRuntimeAnsibleConfig(cfg.NftablesAnsible) {
		return model.ForwardRuntimeBackendNftablesAnsible, mapBootstrapAnsibleConfig(cfg.NftablesAnsible)
	}
	if hasForwardRuntimeAnsibleConfig(cfg.IptablesAnsible) {
		return model.ForwardRuntimeBackendIptablesAnsible, mapBootstrapAnsibleConfig(cfg.IptablesAnsible)
	}

	return defaultForwardLocalAnsibleBackend, panelForwardAnsibleConfig{}
}

func hasForwardRuntimeAnsibleConfig(cfg appconfig.ForwardRuntimeAnsibleConfig) bool {
	return strings.TrimSpace(cfg.Inventory) != "" ||
		strings.TrimSpace(cfg.ApplyPlaybook) != "" ||
		strings.TrimSpace(cfg.RemovePlaybook) != "" ||
		strings.TrimSpace(cfg.Command) != "" ||
		strings.TrimSpace(cfg.WorkingDir) != "" ||
		strings.TrimSpace(cfg.TargetPattern) != "" ||
		cfg.TimeoutSeconds > 0 ||
		cfg.Become ||
		len(cfg.ExtraVars) > 0 ||
		len(cfg.Environment) > 0
}

func mapBootstrapAnsibleConfig(cfg appconfig.ForwardRuntimeAnsibleConfig) panelForwardAnsibleConfig {
	return panelForwardAnsibleConfig{
		Inventory:      strings.TrimSpace(cfg.Inventory),
		ApplyPlaybook:  strings.TrimSpace(cfg.ApplyPlaybook),
		RemovePlaybook: strings.TrimSpace(cfg.RemovePlaybook),
		Become:         cfg.Become,
		ExtraVars:      cloneForwardRuntimeExtraVars(cfg.ExtraVars),
		Command:        strings.TrimSpace(cfg.Command),
		WorkingDir:     strings.TrimSpace(cfg.WorkingDir),
		TargetPattern:  strings.TrimSpace(cfg.TargetPattern),
		Environment:    cloneForwardRuntimeEnvironment(cfg.Environment),
		TimeoutSeconds: cfg.TimeoutSeconds,
	}
}

func cloneForwardRuntimeExtraVars(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]any, len(input))
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
