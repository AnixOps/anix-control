package systemdreport

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfigReadsEachNodesSettings(t *testing.T) {
	config, err := ParseConfig([]byte(`{"interval_seconds": 30, "systemd_services": {"nodes": {
		"12": {"enabled": true, "include": ["nginx*.service", "ssh?.service"], "exclude": ["*-debug.service"]},
		"13": {"enabled": false}
	}}}`))
	require.NoError(t, err)
	node := config.Node(12)
	assert.True(t, node.Enabled)
	assert.True(t, node.Selected("nginx.service"))
	assert.True(t, node.Selected("sshd.service"))
	assert.False(t, node.Selected("nginx-debug.service"), "excluded")
	assert.False(t, node.Selected("cron.service"), "not included")
	assert.False(t, config.Node(13).Enabled)
	assert.Equal(t, NodeConfig{}, config.Node(14), "a node without an entry is off")
}

func TestParseConfigWithoutTheKeyEnablesNoNode(t *testing.T) {
	for _, document := range []string{`{}`, `{"interval_seconds": 30}`, `{"systemd_services": null}`, `{"systemd_services": {}}`} {
		config, err := ParseConfig([]byte(document))
		require.NoError(t, err, document)
		assert.Empty(t, config.Nodes, document)
		assert.False(t, config.Node(1).Enabled, document)
	}
}

func TestParseConfigRefusesMalformedSettings(t *testing.T) {
	tooManyGlobs := make([]string, MaxGlobs+1)
	for index := range tooManyGlobs {
		tooManyGlobs[index] = fmt.Sprintf("%q", fmt.Sprintf("u%d.service", index))
	}
	for name, document := range map[string]string{
		"not an object":      `[]`,
		"settings a list":    `{"systemd_services": []}`,
		"unknown field":      `{"systemd_services": {"nodes": {}, "all": true}}`,
		"unknown node field": `{"systemd_services": {"nodes": {"1": {"enabled": true, "description": "x"}}}}`,
		"node id zero":       `{"systemd_services": {"nodes": {"0": {"enabled": true}}}}`,
		"node id padded":     `{"systemd_services": {"nodes": {"01": {"enabled": true}}}}`,
		"node id a name":     `{"systemd_services": {"nodes": {"edge": {"enabled": true}}}}`,
		"node id too large":  `{"systemd_services": {"nodes": {"4294967296": {"enabled": true}}}}`,
		"enabled a string":   `{"systemd_services": {"nodes": {"1": {"enabled": "yes"}}}}`,
		"bad glob":           `{"systemd_services": {"nodes": {"1": {"enabled": true, "include": ["nginx["]}}}}`,
		"empty glob":         `{"systemd_services": {"nodes": {"1": {"enabled": true, "exclude": [""]}}}}`,
		"glob with a space":  `{"systemd_services": {"nodes": {"1": {"enabled": true, "exclude": ["a b"]}}}}`,
		"glob with a slash":  `{"systemd_services": {"nodes": {"1": {"enabled": true, "exclude": ["a/b"]}}}}`,
		"glob too long":      `{"systemd_services": {"nodes": {"1": {"enabled": true, "exclude": ["` + strings.Repeat("a", MaxGlobLength+1) + `"]}}}}`,
		"too many globs":     `{"systemd_services": {"nodes": {"1": {"enabled": true, "include": [` + strings.Join(tooManyGlobs, ",") + `]}}}}`,
	} {
		_, err := ParseConfig([]byte(document))
		require.ErrorIs(t, err, ErrInvalidConfig, name)
	}
}

func TestValidGlob(t *testing.T) {
	for _, glob := range []string{"*", "nginx.service", "nginx*.service", "ssh?.service", "[a-c]*.service", "[^x]*", `a@b\x2d1.service`} {
		assert.True(t, ValidGlob(glob), glob)
	}
	for _, glob := range []string{"", "[", "a[", "x\\", "a b", "a/b", "ü.service"} {
		assert.False(t, ValidGlob(glob), glob)
	}
}
