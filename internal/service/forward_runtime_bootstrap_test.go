package service

import (
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
