package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type stubForwardRuntimeNodeXExecutor struct {
	requests []nodeXForwardExecuteRequest
	result   *nodeXForwardExecuteResult
	err      error
}

type stubNodeXHTTPDoer struct {
	lastRequest *http.Request
	response    *http.Response
	err         error
}

func (s *stubForwardRuntimeNodeXExecutor) Execute(_ context.Context, req nodeXForwardExecuteRequest) (*nodeXForwardExecuteResult, error) {
	s.requests = append(s.requests, req)
	if s.result != nil {
		return s.result, s.err
	}
	return &nodeXForwardExecuteResult{
		Backend: model.ForwardRuntimeBackendGost,
		Status:  model.ForwardRuntimeJobStatusSuccess,
		Message: "ok",
	}, s.err
}

func (s *stubNodeXHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	s.lastRequest = req
	if s.err != nil {
		return nil, s.err
	}
	if s.response != nil {
		return s.response, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"data":{"backend":"gost","status":2,"message":"ok"}}`)),
	}, nil
}

type ForwardRuntimeProviderTestSuite struct {
	ServiceTestSuite
}

func (s *ForwardRuntimeProviderTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	database.AutoMigrate(
		&model.ForwardNode{},
		&model.ForwardRule{},
		&model.SystemConfig{},
	)
}

func (s *ForwardRuntimeProviderTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	db := database.Get()
	db.Exec("DELETE FROM v2_forward_rule")
	db.Exec("DELETE FROM v2_forward_node")
	db.Exec("DELETE FROM v2_system_config")
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeProvider_BuildsLegacyRuleRequest() {
	relayNode := &model.ForwardNode{
		Name:     "relay-runtime-node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "relay.example.com",
		Port:     22,
		APIPort:  19090,
		APIToken: "relay-token",
		Enabled:  true,
	}
	exitNode := &model.ForwardNode{
		Name:    "exit-runtime-node",
		Type:    model.ForwardNodeTypeExit,
		Host:    "203.0.113.20",
		Port:    443,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(relayNode).Error)
	assert.NoError(s.T(), database.Get().Create(exitNode).Error)

	rule := &model.ForwardRule{
		Name:        "legacy-rule-create",
		Enabled:     true,
		RelayNodeID: relayNode.ID,
		ListenPort:  18080,
		Protocol:    "tcp",
		ExitNodeID:  exitNode.ID,
		TargetHost:  "198.51.100.50",
		TargetPort:  443,
	}
	assert.NoError(s.T(), database.Get().Create(rule).Error)
	rule = loadForwardRuleWithoutNodes(s.T(), rule.ID)

	executor := &stubForwardRuntimeNodeXExecutor{}
	provider := &nodeXForwardRuntimeProvider{
		db:     database.Get(),
		client: executor,
	}

	err := provider.CreateForwardRule(context.Background(), rule)
	assert.NoError(s.T(), err)
	if assert.Len(s.T(), executor.requests, 1) {
		req := executor.requests[0]
		assert.Equal(s.T(), nodeXForwardResourceTypeLegacyRule, req.ResourceType)
		assert.Equal(s.T(), model.ForwardRuntimeBackendGost, req.Backend)
		assert.Equal(s.T(), model.ForwardRuntimeJobActionCreate, req.Action)
		if assert.NotNil(s.T(), req.LegacyRule) {
			assert.Equal(s.T(), rule.ID, req.LegacyRule.Rule.ID)
			assert.Equal(s.T(), rule.Name, req.LegacyRule.Rule.Name)
			assert.Equal(s.T(), rule.ListenPort, req.LegacyRule.Rule.ListenPort)
			assert.Equal(s.T(), "tcp", req.LegacyRule.Rule.Protocol)
			assert.Equal(s.T(), rule.TargetHost, req.LegacyRule.Rule.TargetHost)
			assert.Equal(s.T(), rule.TargetPort, req.LegacyRule.Rule.TargetPort)
			assert.Equal(s.T(), relayNode.ID, req.LegacyRule.RelayNode.ID)
			assert.Equal(s.T(), relayNode.APIToken, req.LegacyRule.RelayNode.APIToken)
			assert.Equal(s.T(), exitNode.ID, req.LegacyRule.ExitNode.ID)
		}
	}
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeProvider_SyncUsesSyncActionAndEnabledState() {
	relayNode := &model.ForwardNode{
		Name:     "relay-runtime-node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "relay.example.com",
		Port:     22,
		APIPort:  19090,
		APIToken: "relay-token",
		Enabled:  true,
	}
	exitNode := &model.ForwardNode{
		Name:    "exit-runtime-node",
		Type:    model.ForwardNodeTypeExit,
		Host:    "203.0.113.21",
		Port:    53,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(relayNode).Error)
	assert.NoError(s.T(), database.Get().Create(exitNode).Error)

	rule := &model.ForwardRule{
		Name:        "legacy-rule-sync",
		Enabled:     true,
		RelayNodeID: relayNode.ID,
		ListenPort:  28080,
		Protocol:    "udp",
		ExitNodeID:  exitNode.ID,
		TargetHost:  "203.0.113.5",
		TargetPort:  53,
	}
	assert.NoError(s.T(), database.Get().Create(rule).Error)
	rule = loadForwardRuleWithoutNodes(s.T(), rule.ID)
	rule.Enabled = false

	executor := &stubForwardRuntimeNodeXExecutor{}
	provider := &nodeXForwardRuntimeProvider{
		db:     database.Get(),
		client: executor,
	}

	err := provider.SyncForwardRule(context.Background(), rule)
	assert.NoError(s.T(), err)
	if assert.Len(s.T(), executor.requests, 1) {
		req := executor.requests[0]
		assert.Equal(s.T(), model.ForwardRuntimeJobActionSync, req.Action)
		if assert.NotNil(s.T(), req.LegacyRule) {
			assert.False(s.T(), req.LegacyRule.Rule.Enabled)
			assert.Equal(s.T(), "udp", req.LegacyRule.Rule.Protocol)
		}
	}
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeProvider_PropagatesExecutorErrors() {
	relayNode := &model.ForwardNode{
		Name:     "relay-runtime-node",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "relay.example.com",
		Port:     22,
		APIPort:  19090,
		APIToken: "relay-token",
		Enabled:  true,
	}
	exitNode := &model.ForwardNode{
		Name:    "exit-runtime-node",
		Type:    model.ForwardNodeTypeExit,
		Host:    "203.0.113.22",
		Port:    8080,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(relayNode).Error)
	assert.NoError(s.T(), database.Get().Create(exitNode).Error)

	rule := &model.ForwardRule{
		Name:        "legacy-rule-error",
		Enabled:     true,
		RelayNodeID: relayNode.ID,
		ListenPort:  48080,
		Protocol:    "tcp",
		ExitNodeID:  exitNode.ID,
		TargetHost:  "198.51.100.80",
		TargetPort:  9000,
	}
	assert.NoError(s.T(), database.Get().Create(rule).Error)
	rule = loadForwardRuleWithoutNodes(s.T(), rule.ID)

	executor := &stubForwardRuntimeNodeXExecutor{err: assert.AnError}
	provider := &nodeXForwardRuntimeProvider{
		db:     database.Get(),
		client: executor,
	}

	err := provider.UpdateForwardRule(context.Background(), rule)
	assert.ErrorIs(s.T(), err, assert.AnError)
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeProvider_ReturnsErrorWhenRelayNodeMissing() {
	relayNode := &model.ForwardNode{
		Name:     "relay-runtime-node-missing-relay",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "relay-missing.example.com",
		Port:     22,
		APIPort:  19092,
		APIToken: "relay-missing-token",
		Enabled:  true,
	}
	exitNode := &model.ForwardNode{
		Name:    "exit-runtime-node-missing-relay",
		Type:    model.ForwardNodeTypeExit,
		Host:    "203.0.113.40",
		Port:    4443,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(relayNode).Error)
	assert.NoError(s.T(), database.Get().Create(exitNode).Error)

	rule := &model.ForwardRule{
		Name:        "missing-relay",
		Enabled:     true,
		RelayNodeID: relayNode.ID,
		ListenPort:  3890,
		Protocol:    "tcp",
		ExitNodeID:  exitNode.ID,
		TargetHost:  "missing-relay.example.com",
		TargetPort:  443,
	}
	assert.NoError(s.T(), database.Get().Create(rule).Error)
	assert.NoError(s.T(), database.Get().Delete(&model.ForwardNode{}, relayNode.ID).Error)
	rule = loadForwardRuleWithoutNodes(s.T(), rule.ID)

	executor := &stubForwardRuntimeNodeXExecutor{}
	provider := &nodeXForwardRuntimeProvider{
		db:     database.Get(),
		client: executor,
	}

	err := provider.CreateForwardRule(context.Background(), rule)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), fmt.Sprintf("node %d not found", rule.RelayNodeID))
	assert.Len(s.T(), executor.requests, 0)
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeProvider_ReturnsErrorWhenExitNodeMissing() {
	relayNode := &model.ForwardNode{
		Name:     "relay-runtime-node-missing-exit",
		Type:     model.ForwardNodeTypeRelay,
		Host:     "198.51.100.30",
		Port:     22,
		APIPort:  19091,
		APIToken: "relay-missing-token",
		Enabled:  true,
	}
	exitNode := &model.ForwardNode{
		Name:    "exit-runtime-node-missing-exit",
		Type:    model.ForwardNodeTypeExit,
		Host:    "203.0.113.41",
		Port:    5555,
		Enabled: true,
	}
	assert.NoError(s.T(), database.Get().Create(relayNode).Error)
	assert.NoError(s.T(), database.Get().Create(exitNode).Error)

	rule := &model.ForwardRule{
		Name:        "missing-exit",
		Enabled:     true,
		RelayNodeID: relayNode.ID,
		ListenPort:  5090,
		Protocol:    "tcp",
		ExitNodeID:  exitNode.ID,
		TargetHost:  "missing-exit.example.com",
		TargetPort:  8443,
	}
	assert.NoError(s.T(), database.Get().Create(rule).Error)
	assert.NoError(s.T(), database.Get().Delete(&model.ForwardNode{}, exitNode.ID).Error)
	rule = loadForwardRuleWithoutNodes(s.T(), rule.ID)

	executor := &stubForwardRuntimeNodeXExecutor{}
	provider := &nodeXForwardRuntimeProvider{
		db:     database.Get(),
		client: executor,
	}

	err := provider.CreateForwardRule(context.Background(), rule)
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), fmt.Sprintf("node %d not found", rule.ExitNodeID))
	assert.Len(s.T(), executor.requests, 0)
}

func (s *ForwardRuntimeProviderTestSuite) TestForwardRuleService_DefaultProviderDispatchesToNodeXClient() {
	nodeService := NewForwardNodeService(database.Get())
	ruleService := NewForwardRuleService(database.Get(), nodeService)
	_, ok := ruleService.runtimeProvider.(*nodeXForwardRuntimeProvider)
	assert.True(s.T(), ok)
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeClient_UsesConfiguredTimeoutSeconds() {
	configService := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configService.Set(
		forwardRuntimeNodeXBaseURLConfigKey,
		"http://127.0.0.1:65535",
		"string",
		"forward",
		"test NodeX runtime URL",
	))
	assert.NoError(s.T(), configService.Set(
		forwardRuntimeNodeXTokenConfigKey,
		"",
		"string",
		"forward",
		"test NodeX runtime token",
	))
	assert.NoError(s.T(), configService.Set(
		forwardRuntimeNodeXTimeoutSecondsConfigKey,
		"1",
		"int",
		"forward",
		"test NodeX runtime timeout",
	))

	client := newNodeXForwardRuntimeClient(configService)
	settings, err := client.loadSettings()
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), time.Second, settings.Timeout)
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeClient_PrefersConfiguredControlPlaneEndpointForPanelForward() {
	configService := NewSystemConfigService(database.Get())
	assert.NoError(s.T(), configService.Set(
		forwardRuntimeNodeXBaseURLConfigKey,
		"http://127.0.0.1:18080",
		"string",
		"forward",
		"test NodeX runtime URL",
	))
	assert.NoError(s.T(), configService.Set(
		forwardRuntimeNodeXTokenConfigKey,
		"control-plane-token",
		"string",
		"forward",
		"test NodeX runtime token",
	))

	doer := &stubNodeXHTTPDoer{}
	client := &nodeXForwardRuntimeClient{
		configService: configService,
		httpClient:    doer,
	}

	_, err := client.Execute(context.Background(), nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
		Backend:      model.ForwardRuntimeBackendIptablesAnsible,
		Action:       model.ForwardRuntimeJobActionCreate,
		PanelForward: &nodeXPanelForwardRequest{
			Forward: nodeXPanelForwardPayload{
				ID:         1,
				UserID:     2,
				Name:       "panel-forward",
				InPort:     10001,
				RemoteAddr: "198.51.100.20:443",
				Strategy:   "fifo",
				Status:     model.ForwardStatusActive,
			},
			Tunnel: nodeXPanelTunnelPayload{
				ID:            3,
				Name:          "tunnel",
				InNodeID:      4,
				Protocol:      "tcp",
				TCPListenAddr: "0.0.0.0",
			},
			IngressNode: nodeXForwardNodePayload{
				ID:       4,
				Name:     "relay",
				Host:     "203.0.113.10",
				Port:     22,
				APIPort:  19090,
				APIToken: "relay-token",
			},
		},
	})
	assert.NoError(s.T(), err)
	if assert.NotNil(s.T(), doer.lastRequest) {
		assert.Equal(s.T(), "http://127.0.0.1:18080"+defaultForwardRuntimeNodeXExecutePath, doer.lastRequest.URL.String())
		assert.Equal(s.T(), "Bearer control-plane-token", doer.lastRequest.Header.Get("Authorization"))
		assert.Equal(s.T(), "control-plane-token", doer.lastRequest.Header.Get("X-API-Key"))
		assert.Equal(s.T(), "4", doer.lastRequest.Header.Get("X-Forward-Node-ID"))
	}
}

func (s *ForwardRuntimeProviderTestSuite) TestNodeXForwardRuntimeClient_RequiresConfiguredControlPlaneEndpoint() {
	client := newNodeXForwardRuntimeClient(NewSystemConfigService(database.Get()))

	_, err := client.Execute(context.Background(), nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
		Backend:      model.ForwardRuntimeBackendGost,
		Action:       model.ForwardRuntimeJobActionCreate,
		PanelForward: &nodeXPanelForwardRequest{
			Forward: nodeXPanelForwardPayload{
				ID:         1,
				UserID:     2,
				Name:       "panel-forward",
				InPort:     10001,
				RemoteAddr: "198.51.100.20:443",
				Strategy:   "fifo",
				Status:     model.ForwardStatusActive,
			},
			Tunnel: nodeXPanelTunnelPayload{
				ID:            3,
				Name:          "tunnel",
				InNodeID:      4,
				Protocol:      "tcp",
				TCPListenAddr: "0.0.0.0",
			},
			IngressNode: nodeXForwardNodePayload{
				ID:       4,
				Name:     "relay",
				Host:     "203.0.113.10",
				Port:     22,
				APIPort:  19090,
				APIToken: "relay-token",
			},
		},
	})
	assert.Error(s.T(), err)
	assert.Contains(s.T(), err.Error(), forwardRuntimeNodeXBaseURLConfigKey)
}

func TestForwardRuntimeProvider(t *testing.T) {
	suite.Run(t, new(ForwardRuntimeProviderTestSuite))
}

func loadForwardRuleWithoutNodes(t *testing.T, ruleID uint) *model.ForwardRule {
	t.Helper()
	var rule model.ForwardRule
	assert.NoError(t, database.Get().First(&rule, ruleID).Error)
	rule.RelayNode = nil
	rule.ExitNode = nil
	return &rule
}
