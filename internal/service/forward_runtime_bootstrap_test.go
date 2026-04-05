package service

import (
	"os"
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

func TestForwardRuntimeBootstrap(t *testing.T) {
	suite.Run(t, new(ForwardRuntimeBootstrapTestSuite))
}
