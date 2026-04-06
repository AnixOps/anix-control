package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type PanelForwardRuntimeDiagnosticsTestSuite struct {
	ServiceTestSuite
}

func (s *PanelForwardRuntimeDiagnosticsTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(&model.SystemConfig{})
}

func (s *PanelForwardRuntimeDiagnosticsTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	database.Get().Exec("DELETE FROM v2_system_config")
}

func (s *PanelForwardRuntimeDiagnosticsTestSuite) TestNodeXRuntimeStatusReadsConfiguredControlPlane() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case defaultForwardRuntimeNodeXStatusPath:
			assert.Equal(s.T(), "Bearer runtime-token", r.Header.Get("Authorization"))
			assert.Equal(s.T(), "runtime-token", r.Header.Get("X-API-Key"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"v0.0.16","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status","authRequired":true,"supports":{"resourceTypes":["panel_forward"],"backends":["gost","iptables_ansible"],"actions":["create","update","delete"]},"modes":{"gost":{"supported":true},"iptablesAnsible":{"supported":true,"ready":true,"command":"ansible-playbook","commandFound":true,"inventoryPath":"inventory.ini","inventoryExists":true,"applyPlaybookPath":"apply.yml","applyPlaybookExists":true,"removePlaybookPath":"remove.yml","removePlaybookExists":true,"workingDir":".","workingDirExists":true,"targetPattern":"relay","become":false,"timeoutSeconds":30}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configSvc := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXBaseURLConfigKey, server.URL, "string", forwardRuntimeConfigGroup, "runtime status url"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXTokenConfigKey, "runtime-token", "string", forwardRuntimeConfigGroup, "runtime status token"))

	client := newNodeXForwardRuntimeClient(configSvc)
	status, err := client.Status(context.Background())
	assert.NoError(s.T(), err)
	if assert.NotNil(s.T(), status) {
		assert.Equal(s.T(), "v0.0.16", status.Version)
		assert.True(s.T(), status.AuthRequired)
		assert.True(s.T(), status.Modes.IptablesAnsible.Ready)
	}
}

func (s *PanelForwardRuntimeDiagnosticsTestSuite) TestNodeXDoctorSummarizesHealthStatusAndCommands() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case defaultForwardRuntimeNodeXHealthPath:
			_, _ = w.Write([]byte("ok"))
		case defaultForwardRuntimeNodeXStatusPath:
			assert.Equal(s.T(), "Bearer doctor-token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"version":"v0.0.17-test.5","executePath":"/api/v2/internal/forward/runtime/execute","statusPath":"/api/v2/internal/forward/runtime/status","authRequired":true,"supports":{"resourceTypes":["panel_forward","legacy_rule"],"backends":["gost","iptables_ansible"],"actions":["create","update","delete","pause","resume","sync"]},"modes":{"gost":{"supported":true},"iptablesAnsible":{"supported":true,"ready":false,"command":"ansible-playbook","commandFound":false,"inventoryPath":"inventory.ini","inventoryExists":false,"applyPlaybookPath":"apply.yml","applyPlaybookExists":true,"removePlaybookPath":"remove.yml","removePlaybookExists":true,"workingDir":"playbooks","workingDirExists":true,"targetPattern":"relay","become":false,"timeoutSeconds":45,"issues":["inventory missing: inventory.ini"]}}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	configSvc := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXBaseURLConfigKey, server.URL, "string", forwardRuntimeConfigGroup, "doctor url"))
	assert.NoError(s.T(), configSvc.Set(forwardRuntimeNodeXTokenConfigKey, "doctor-token", "string", forwardRuntimeConfigGroup, "doctor token"))

	client := newNodeXForwardRuntimeClient(configSvc)
	summary, err := client.Doctor(context.Background())
	assert.NoError(s.T(), err)
	if assert.NotNil(s.T(), summary) {
		assert.Equal(s.T(), server.URL, summary.BaseURL)
		assert.True(s.T(), summary.Health.OK)
		assert.True(s.T(), summary.RuntimeStatus.OK)
		assert.Equal(s.T(), "v0.0.17-test.5", summary.RuntimeStatus.Version)
		assert.Contains(s.T(), summary.RuntimeStatus.Issues, "inventory missing: inventory.ini")
		assert.Contains(s.T(), summary.Commands.PowerShell[1], server.URL)
		assert.Contains(s.T(), summary.Commands.Bash[1], fmt.Sprintf("BASE_URL=%s", server.URL))
		assert.Contains(s.T(), summary.Commands.References, "docs/reference/upgrade.md")
	}
}

func TestPanelForwardRuntimeDiagnosticsTestSuite(t *testing.T) {
	suite.Run(t, new(PanelForwardRuntimeDiagnosticsTestSuite))
}
