package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
	_ = os.Unsetenv(forwardRuntimeNodeXModeEnvVar)
	_ = os.Unsetenv(forwardRuntimeBackendEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleConfigJSONEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleInventoryEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleApplyEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleRemoveEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleWorkdirEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleTargetPatternEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleCommandEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleTimeoutEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleBecomeEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleExtraVarsEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleEnvEnvVar)
	_ = os.Unsetenv(forwardRuntimeNodeXBaseURLEnvVar)
	_ = os.Unsetenv(forwardRuntimeNodeXTokenEnvVar)
	_ = os.Unsetenv(forwardRuntimeNodeXTimeoutSecondsEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleHostAliasEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleHostEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsiblePortEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleUserEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsiblePasswordEnvVar)
	_ = os.Unsetenv(forwardRuntimeAnsibleBecomePasswordEnvVar)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_SeedsConfigs() {
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendIptablesAnsible)
	s.T().Setenv(forwardRuntimeAnsibleConfigJSONEnvVar, `{"inventory":"/app/config/deploy/ansible/inventory.ini","playbookApply":"/app/config/deploy/ansible/playbooks/forward_apply.yml","playbookRemove":"/app/config/deploy/ansible/playbooks/forward_remove.yml","workingDir":"/app/config/deploy/ansible"}`)

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, backend)

	ansibleConfig, err := configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), ansibleConfig, `"inventory":"/app/config/deploy/ansible/inventory.ini"`)
	assert.Contains(s.T(), ansibleConfig, `"workingDir":"/app/config/deploy/ansible"`)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_RejectsInvalidBackend() {
	s.T().Setenv(forwardRuntimeBackendEnvVar, "bad-backend")

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeBackendEnvVar)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_RejectsInvalidJSON() {
	s.T().Setenv(forwardRuntimeAnsibleConfigJSONEnvVar, `{"inventory":`)

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeAnsibleConfigJSONEnvVar)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_EnvOverridesJSON() {
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendIptablesAnsible)
	s.T().Setenv(forwardRuntimeAnsibleConfigJSONEnvVar, `{"inventory":"/app/config/deploy/ansible/inventory.ini","workingDir":"/app/config/deploy/ansible"}`)
	s.T().Setenv(forwardRuntimeAnsibleInventoryEnvVar, "/home/v2board/.config/v2board/forward-runtime/inventory.ini")

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	inventory, err := configService.Get(forwardRuntimeAnsibleInventoryConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "/home/v2board/.config/v2board/forward-runtime/inventory.ini", inventory)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_SeedsNodeXConfig() {
	s.T().Setenv(forwardRuntimeNodeXBaseURLEnvVar, "http://127.0.0.1:18080")
	s.T().Setenv(forwardRuntimeNodeXTokenEnvVar, "nodex-secret")
	s.T().Setenv(forwardRuntimeNodeXTimeoutSecondsEnvVar, "45")

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	baseURL, err := configService.Get(forwardRuntimeNodeXBaseURLConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "http://127.0.0.1:18080", baseURL)

	token, err := configService.Get(forwardRuntimeNodeXTokenConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "nodex-secret", token)

	timeout, err := configService.Get(forwardRuntimeNodeXTimeoutSecondsConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "45", timeout)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_NodeXModeSeedsDerivedBackend() {
	s.T().Setenv(forwardRuntimeNodeXModeEnvVar, "true")
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendIptablesAnsible)
	s.T().Setenv(forwardRuntimeNodeXBaseURLEnvVar, "http://127.0.0.1:18080")
	s.T().Setenv(forwardRuntimeNodeXTokenEnvVar, "nodex-secret")

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	mode, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "true", mode)

	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendGost, backend)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_NodeXModeDisabledSeedsIptablesBackend() {
	s.T().Setenv(forwardRuntimeNodeXModeEnvVar, "false")
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendGost)

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	mode, err := configService.Get(forwardRuntimeNodeXModeConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), "false", mode)

	backend, err := configService.Get(forwardRuntimeBackendConfigKey)
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), model.ForwardRuntimeBackendIptablesAnsible, backend)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_NodeXModeRequiresBaseURL() {
	s.T().Setenv(forwardRuntimeNodeXModeEnvVar, "true")
	s.T().Setenv(forwardRuntimeNodeXTokenEnvVar, "nodex-secret")

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXBaseURLConfigKey)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_NodeXModeRequiresToken() {
	s.T().Setenv(forwardRuntimeNodeXModeEnvVar, "true")
	s.T().Setenv(forwardRuntimeNodeXBaseURLEnvVar, "http://127.0.0.1:18080")

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXTokenConfigKey)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_RejectsInvalidNodeXMode() {
	s.T().Setenv(forwardRuntimeNodeXModeEnvVar, "maybe")

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXModeEnvVar)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_RejectsInvalidExtraVarsJSON() {
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendIptablesAnsible)
	s.T().Setenv(forwardRuntimeAnsibleExtraVarsEnvVar, `{"retry":`)

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeAnsibleExtraVarsEnvVar)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_RejectsInvalidEnvironmentJSON() {
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendIptablesAnsible)
	s.T().Setenv(forwardRuntimeAnsibleEnvEnvVar, `{"ANSIBLE_DEBUG":}`)

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeAnsibleEnvEnvVar)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_SeedsEnvironmentVars() {
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendIptablesAnsible)
	s.T().Setenv(forwardRuntimeAnsibleEnvEnvVar, `{"ANSIBLE_DEBUG":"true","CUSTOM_VAR":"configured"}`)

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	ansibleConfig, err := configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)

	var cfg panelForwardAnsibleConfig
	assert.NoError(s.T(), json.Unmarshal([]byte(ansibleConfig), &cfg))
	assert.Equal(s.T(), "true", cfg.Environment["ANSIBLE_DEBUG"])
	assert.Equal(s.T(), "configured", cfg.Environment["CUSTOM_VAR"])
	assert.Equal(s.T(), defaultForwardAnsibleConfigPath, cfg.Environment["ANSIBLE_CONFIG"])
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeSystemConfigFromEnv_SeedsExtraVars() {
	s.T().Setenv(forwardRuntimeBackendEnvVar, model.ForwardRuntimeBackendIptablesAnsible)
	s.T().Setenv(forwardRuntimeAnsibleExtraVarsEnvVar, `{"forward_retry":5,"note":"env"}`)

	err := InitForwardRuntimeSystemConfigFromEnv(database.Get())
	assert.NoError(s.T(), err)

	configService := NewSystemConfigService(database.Get())
	ansibleConfig, err := configService.Get(forwardRuntimeAnsibleConfigJSONKey)
	assert.NoError(s.T(), err)

	var cfg panelForwardAnsibleConfig
	assert.NoError(s.T(), json.Unmarshal([]byte(ansibleConfig), &cfg))
	assert.Equal(s.T(), float64(5), cfg.ExtraVars["forward_retry"])
	assert.Equal(s.T(), "env", cfg.ExtraVars["note"])
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeAnsibleInventoryFromEnv_WritesPasswordInventory() {
	inventoryPath := filepath.Join(s.T().TempDir(), "inventory.ini")
	s.T().Setenv(forwardRuntimeAnsibleInventoryEnvVar, inventoryPath)
	s.T().Setenv(forwardRuntimeAnsibleHostEnvVar, "203.0.113.10")
	s.T().Setenv(forwardRuntimeAnsibleUserEnvVar, "debian")
	s.T().Setenv(forwardRuntimeAnsiblePasswordEnvVar, `p@ss"word`)
	s.T().Setenv(forwardRuntimeAnsibleBecomePasswordEnvVar, `sudo"pass`)

	path, err := InitForwardRuntimeAnsibleInventoryFromEnv()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), inventoryPath, path)

	content, err := os.ReadFile(path)
	assert.NoError(s.T(), err)
	assert.Contains(s.T(), string(content), `[forward_nodes]`)
	assert.Contains(s.T(), string(content), `ansible_host="203.0.113.10"`)
	assert.Contains(s.T(), string(content), `ansible_user="debian"`)
	assert.Contains(s.T(), string(content), `ansible_password="p@ss\"word"`)
	assert.Contains(s.T(), string(content), `ansible_become_password="sudo\"pass"`)
}

func (s *ForwardRuntimeBootstrapTestSuite) TestInitForwardRuntimeAnsibleInventoryFromEnv_RejectsPartialPasswordConfig() {
	s.T().Setenv(forwardRuntimeAnsibleHostEnvVar, "203.0.113.10")
	s.T().Setenv(forwardRuntimeAnsibleUserEnvVar, "root")

	_, err := InitForwardRuntimeAnsibleInventoryFromEnv()
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeAnsiblePasswordEnvVar)
}

func TestForwardRuntimeBootstrap(t *testing.T) {
	suite.Run(t, new(ForwardRuntimeBootstrapTestSuite))
}
