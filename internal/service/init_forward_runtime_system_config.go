package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/anixops/v2board/internal/model"
	"gorm.io/gorm"
)

const (
	forwardRuntimeBackendEnvVar              = "FORWARD_RUNTIME_BACKEND"
	forwardRuntimeAnsibleConfigJSONEnvVar    = "FORWARD_RUNTIME_ANSIBLE_CONFIG_JSON"
	forwardRuntimeAnsibleInventoryEnvVar     = "FORWARD_RUNTIME_ANSIBLE_INVENTORY"
	forwardRuntimeAnsibleApplyEnvVar         = "FORWARD_RUNTIME_ANSIBLE_PLAYBOOK_APPLY"
	forwardRuntimeAnsibleRemoveEnvVar        = "FORWARD_RUNTIME_ANSIBLE_PLAYBOOK_REMOVE"
	forwardRuntimeAnsibleWorkdirEnvVar       = "FORWARD_RUNTIME_ANSIBLE_WORKDIR"
	forwardRuntimeAnsibleTargetPatternEnvVar = "FORWARD_RUNTIME_ANSIBLE_TARGET_PATTERN"
	forwardRuntimeAnsibleCommandEnvVar       = "FORWARD_RUNTIME_ANSIBLE_COMMAND"
	forwardRuntimeAnsibleTimeoutEnvVar       = "FORWARD_RUNTIME_ANSIBLE_TIMEOUT_SECONDS"
	forwardRuntimeAnsibleBecomeEnvVar        = "FORWARD_RUNTIME_ANSIBLE_BECOME"
	forwardRuntimeAnsibleExtraVarsEnvVar     = "FORWARD_RUNTIME_ANSIBLE_EXTRA_VARS_JSON"
	forwardRuntimeAnsibleEnvEnvVar           = "FORWARD_RUNTIME_ANSIBLE_ENV_JSON"
)

func InitForwardRuntimeSystemConfigFromEnv(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	configService := NewSystemConfigService(db)
	backend := strings.TrimSpace(strings.ToLower(os.Getenv(forwardRuntimeBackendEnvVar)))
	switch backend {
	case "":
	case model.ForwardRuntimeBackendGost, model.ForwardRuntimeBackendIptablesAnsible:
		if err := configService.Set(
			forwardRuntimeBackendConfigKey,
			backend,
			"string",
			forwardRuntimeConfigGroup,
			"Forward runtime backend injected from environment",
		); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid %s value: %s", forwardRuntimeBackendEnvVar, backend)
	}

	cfg := &panelForwardAnsibleConfig{}

	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleConfigJSONEnvVar)); value != "" {
		if err := json.Unmarshal([]byte(value), cfg); err != nil {
			return fmt.Errorf("invalid %s value: %w", forwardRuntimeAnsibleConfigJSONEnvVar, err)
		}
	}

	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleInventoryEnvVar)); value != "" {
		cfg.Inventory = value
	}
	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleApplyEnvVar)); value != "" {
		cfg.ApplyPlaybook = value
	}
	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleRemoveEnvVar)); value != "" {
		cfg.RemovePlaybook = value
	}
	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleWorkdirEnvVar)); value != "" {
		cfg.WorkingDir = value
	}
	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleTargetPatternEnvVar)); value != "" {
		cfg.TargetPattern = value
	}
	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleCommandEnvVar)); value != "" {
		cfg.Command = value
	}

	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleTimeoutEnvVar)); value != "" {
		timeout, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("invalid %s value: %w", forwardRuntimeAnsibleTimeoutEnvVar, err)
		}
		cfg.TimeoutSeconds = timeout
	}

	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleBecomeEnvVar)); value != "" {
		cfg.Become = strings.EqualFold(value, "true") || value == "1"
	}

	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleExtraVarsEnvVar)); value != "" {
		if err := json.Unmarshal([]byte(value), &cfg.ExtraVars); err != nil {
			return fmt.Errorf("invalid %s value: %w", forwardRuntimeAnsibleExtraVarsEnvVar, err)
		}
	}

	if value := strings.TrimSpace(os.Getenv(forwardRuntimeAnsibleEnvEnvVar)); value != "" {
		if err := json.Unmarshal([]byte(value), &cfg.Environment); err != nil {
			return fmt.Errorf("invalid %s value: %w", forwardRuntimeAnsibleEnvEnvVar, err)
		}
	}

	shouldPersist := backend == model.ForwardRuntimeBackendIptablesAnsible ||
		strings.TrimSpace(cfg.Inventory) != "" ||
		strings.TrimSpace(cfg.ApplyPlaybook) != "" ||
		strings.TrimSpace(cfg.RemovePlaybook) != "" ||
		strings.TrimSpace(cfg.WorkingDir) != "" ||
		strings.TrimSpace(cfg.TargetPattern) != "" ||
		strings.TrimSpace(cfg.Command) != "" ||
		cfg.TimeoutSeconds > 0 ||
		cfg.Become ||
		len(cfg.ExtraVars) > 0 ||
		len(cfg.Environment) > 0
	if !shouldPersist {
		return nil
	}

	cfg.ensureDefaults()
	if err := configService.SetJSON(
		forwardRuntimeAnsibleConfigJSONKey,
		cfg,
		forwardRuntimeConfigGroup,
		"Forward runtime ansible config injected from environment",
	); err != nil {
		return err
	}

	if err := configService.Set(
		forwardRuntimeAnsibleInventoryConfigKey,
		cfg.Inventory,
		"string",
		forwardRuntimeConfigGroup,
		"Forward ansible inventory injected from environment",
	); err != nil {
		return err
	}
	if err := configService.Set(
		forwardRuntimeAnsibleApplyPlaybookConfigKey,
		cfg.ApplyPlaybook,
		"string",
		forwardRuntimeConfigGroup,
		"Forward ansible apply playbook injected from environment",
	); err != nil {
		return err
	}
	if err := configService.Set(
		forwardRuntimeAnsibleRemovePlaybookConfigKey,
		cfg.RemovePlaybook,
		"string",
		forwardRuntimeConfigGroup,
		"Forward ansible remove playbook injected from environment",
	); err != nil {
		return err
	}
	if err := configService.Set(
		forwardRuntimeAnsibleBecomeConfigKey,
		strconv.FormatBool(cfg.Become),
		"bool",
		forwardRuntimeConfigGroup,
		"Forward ansible become flag injected from environment",
	); err != nil {
		return err
	}

	extraVarsJSON, err := json.Marshal(cfg.ExtraVars)
	if err != nil {
		return err
	}
	return configService.Set(
		forwardRuntimeAnsibleExtraVarsConfigKey,
		string(extraVarsJSON),
		"json",
		forwardRuntimeConfigGroup,
		"Forward ansible extra vars injected from environment",
	)
}
