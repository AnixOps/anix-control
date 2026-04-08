package service

import (
	"encoding/json"
	"testing"

	appconfig "github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type ForwardRuntimeBootstrapTestSuite struct {
	ServiceTestSuite
}

func (s *ForwardRuntimeBootstrapTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(&model.SystemConfig{})
}

func (s *ForwardRuntimeBootstrapTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	database.Get().Exec("DELETE FROM v2_system_config")
	appconfig.Set(nil)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_SeedsNodeXFromConfig() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL:        "http://127.0.0.1:18081",
				Token:          "config-nodex-token",
				TimeoutSeconds: 20,
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, backend)

	baseURL, err := configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "http://127.0.0.1:18081", baseURL)

	token, err := configService.Get(forwardRuntimeNodeXTokenConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "config-nodex-token", token)

	timeout, err := configService.Get(forwardRuntimeNodeXTimeoutSecondsConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "20", timeout)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_SeedsAnsibleFromConfig() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendNftablesAnsible,
			NftablesAnsible: appconfig.ForwardRuntimeAnsibleConfig{
				Inventory:      "config/deploy/ansible/inventory.ini",
				ApplyPlaybook:  "config/deploy/ansible/playbooks/forward_apply_nftables.yml",
				RemovePlaybook: "config/deploy/ansible/playbooks/forward_remove_nftables.yml",
				WorkingDir:     "config/deploy/ansible",
				TargetPattern:  "{{node.host}}",
				TimeoutSeconds: 90,
				Become:         true,
				ExtraVars: map[string]interface{}{
					"retry": 3,
				},
				Environment: map[string]string{
					"ANSIBLE_CONFIG": "config/deploy/ansible/ansible.cfg",
				},
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, backend)

	localBackend, err := configService.Get(forwardRuntimeLocalBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, localBackend)

	ansibleConfig, err := configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)

	var cfg panelForwardAnsibleConfig
	assert.NoError(s.T(), json.Unmarshal([]byte(ansibleConfig), &cfg))
	assert.Equal(s.T(), "config/deploy/ansible/inventory.ini", cfg.Inventory)
	assert.Equal(s.T(), "config/deploy/ansible/playbooks/forward_apply_nftables.yml", cfg.ApplyPlaybook)
	assert.Equal(s.T(), "config/deploy/ansible/playbooks/forward_remove_nftables.yml", cfg.RemovePlaybook)
	assert.Equal(s.T(), "config/deploy/ansible", cfg.WorkingDir)
	assert.Equal(s.T(), "{{node.host}}", cfg.TargetPattern)
	assert.Equal(s.T(), 90, cfg.TimeoutSeconds)
	assert.True(s.T(), cfg.Become)
	assert.Equal(s.T(), "config/deploy/ansible/ansible.cfg", cfg.Environment["ANSIBLE_CONFIG"])
	assert.Equal(s.T(), float64(3), cfg.ExtraVars["retry"])
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_NodeXModeCompatibilityOverridesBackend() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			NodeXMode: boolPtr(true),
			Backend:   model.ForwardRuntimeBackendIptablesAnsible,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL: "http://127.0.0.1:18081",
				Token:   "nodex-token",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	mode, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "true", mode)

	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, backend)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_SeedsDefaultLocalRuntimeValuesWhenOmitted() {
	configService := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXModeConfigKey, "true", "bool", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeBackendConfigKey, model.ForwardRuntimeBackendGost, "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXBaseURLConfigKey, "http://127.0.0.1:18081", "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXTokenConfigKey, "old-token", "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXTimeoutSecondsConfigKey, "20", "int", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeAnsibleConfigJSONKey, `{"inventory":"old.ini"}`, "json", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))
	assert.NoError(s.T(), configService.Set(forwardRuntimeAnsibleInventoryConfigKey, "old.ini", "string", forwardRuntimeConfigGroup, forwardRuntimeBootstrapRemark))

	appconfig.Set(&appconfig.Config{})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	value, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeLocalBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendNftablesAnsible, value)

	value, err = configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeNodeXTokenConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeNodeXTimeoutSecondsConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "", value)

	value, err = configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)
	assert.NotEmpty(s.T(), value)

	var cfg panelForwardAnsibleConfig
	assert.NoError(s.T(), json.Unmarshal([]byte(value), &cfg))
	assert.Equal(s.T(), defaultForwardAnsibleInventoryPath, cfg.Inventory)
	assert.Equal(s.T(), defaultForwardNftablesApplyPlaybookPath, cfg.ApplyPlaybook)
	assert.Equal(s.T(), defaultForwardNftablesRemovePlaybookPath, cfg.RemovePlaybook)
	assert.Equal(s.T(), defaultForwardAnsibleWorkingDir, cfg.WorkingDir)
	assert.Equal(s.T(), defaultForwardAnsibleTargetPattern, cfg.TargetPattern)
	assert.Equal(s.T(), int(defaultForwardRuntimeJobTimeout.Seconds()), cfg.TimeoutSeconds)
	assert.Equal(s.T(), defaultForwardAnsibleConfigPath, cfg.Environment["ANSIBLE_CONFIG"])

	value, err = configService.Get(forwardRuntimeAnsibleInventoryConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), defaultForwardAnsibleInventoryPath, value)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_PreservesUserManagedOverrides() {
	configService := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXModeConfigKey, "false", "bool", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeBackendConfigKey, model.ForwardRuntimeBackendIptablesAnsible, "string", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXBaseURLConfigKey, "http://127.0.0.1:19090", "string", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeNodeXTokenConfigKey, "manual-token", "string", forwardRuntimeConfigGroup, "manual override"))
	assert.NoError(s.T(), configService.Set(forwardRuntimeAnsibleConfigJSONKey, `{"inventory":"manual.ini"}`, "json", forwardRuntimeConfigGroup, "manual override"))

	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL: "http://127.0.0.1:18081",
				Token:   "config-token",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.NoError(s.T(), err)

	value, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "false", value)

	value, err = configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, value)

	value, err = configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "http://127.0.0.1:19090", value)

	value, err = configService.Get(forwardRuntimeNodeXTokenConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "manual-token", value)

	value, err = configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), `{"inventory":"manual.ini"}`, value)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_RejectsInvalidBackend() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: "bad-backend",
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), "forward_runtime.backend")
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_GostRequiresBaseURL() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				Token: "nodex-token",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXBaseURLConfigKey)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfig_GostRequiresToken() {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Backend: model.ForwardRuntimeBackendGost,
			NodeX: appconfig.ForwardRuntimeNodeXConfig{
				BaseURL: "http://127.0.0.1:18081",
			},
		},
	})

	err := InitForwardRuntimeSystemConfig(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXTokenConfigKey)
}

func boolPtr(v bool) *bool {
	return &v
}

func TestForwardRuntimeBootstrap(t *testing.T) {
	suite.Run(t, new(ForwardRuntimeBootstrapTestSuite))
}
