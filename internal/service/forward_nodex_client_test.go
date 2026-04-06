package service

import (
	"testing"

	"github.com/anixops/v2board/internal/model"
	"github.com/stretchr/testify/assert"
)

func TestResolveNodeXBaseURL_PrefersConfiguredBaseURL(t *testing.T) {
	settings := &nodeXForwardRuntimeSettings{BaseURL: " https://example.com/base/ "}
	result, err := resolveNodeXBaseURL(nil, settings, false)
	assert.NoError(t, err)
	assert.Equal(t, "https://example.com/base", result)
}

func TestResolveNodeXBaseURL_AllowsEmptyWhenOptional(t *testing.T) {
	result, err := resolveNodeXBaseURL(nil, &nodeXForwardRuntimeSettings{}, false)
	assert.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestResolveNodeXBaseURL_RequiresConfigured(t *testing.T) {
	url, err := resolveNodeXBaseURL(nil, &nodeXForwardRuntimeSettings{}, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), forwardRuntimeNodeXBaseURLConfigKey)
	assert.Equal(t, "", url)
}

func TestResolveNodeXBaseURL_DoesNotFallbackToNodeWhenRequired(t *testing.T) {
	url, err := resolveNodeXBaseURL(&nodeXForwardNodePayload{
		Host:    "localhost",
		APIPort: 18080,
	}, &nodeXForwardRuntimeSettings{}, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), forwardRuntimeNodeXBaseURLConfigKey)
	assert.Equal(t, "", url)
}

func TestResolveNodeXToken_PrefersSettings(t *testing.T) {
	config := &nodeXForwardRuntimeSettings{Token: " cfg-token "}
	token, err := resolveNodeXToken(nil, config, true)
	assert.NoError(t, err)
	assert.Equal(t, "cfg-token", token)
}

func TestResolveNodeXToken_AllowsEmptyWhenOptional(t *testing.T) {
	token, err := resolveNodeXToken(nil, &nodeXForwardRuntimeSettings{}, false)
	assert.NoError(t, err)
	assert.Equal(t, "", token)
}

func TestResolveNodeXToken_RequiresConfigured(t *testing.T) {
	_, err := resolveNodeXToken(nil, &nodeXForwardRuntimeSettings{}, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), forwardRuntimeNodeXTokenConfigKey)
}

func TestResolveNodeXToken_DoesNotFallbackToNodeWhenRequired(t *testing.T) {
	token, err := resolveNodeXToken(&nodeXForwardNodePayload{APIToken: "node-token"}, &nodeXForwardRuntimeSettings{}, true)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), forwardRuntimeNodeXTokenConfigKey)
	assert.Equal(t, "", token)
}

func TestRequiresConfiguredNodeXControlPlane(t *testing.T) {
	req := nodeXForwardExecuteRequest{
		ResourceType: nodeXForwardResourceTypePanelForward,
	}
	assert.True(t, requiresConfiguredNodeXControlPlane(req))
	req.Backend = model.ForwardRuntimeBackendIptablesAnsible
	req.AnsibleRuntime = &panelForwardAnsibleRuntimePayload{}
	assert.False(t, requiresConfiguredNodeXControlPlane(req))
}
