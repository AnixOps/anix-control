package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadRejectsSubscribePathsThatNormalizeToV2(t *testing.T) {
	unsafePaths := []string{
		"api/v2",
		"/api/v2/hidden/",
		"  /api//v2/hidden  ",
		"api/v2/../v2/hidden",
		"api/:segment",
		"api/*rest",
	}

	for _, subscribePath := range unsafePaths {
		t.Run(subscribePath, func(t *testing.T) {
			resetConfig()
			t.Cleanup(resetConfig)

			path := filepath.Join(t.TempDir(), "config.yaml")
			contents := "app:\n  subscribe_path: " + subscribePath + "\n"
			require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))

			_, err := Load(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "subscribe_path")
		})
	}
}

func TestLoadNormalizesSafeCustomSubscribePath(t *testing.T) {
	resetConfig()
	t.Cleanup(resetConfig)

	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "app:\n  subscribe_path: ' /custom//subscribe/ '\n"
	require.NoError(t, os.WriteFile(path, []byte(contents), 0o644))

	loaded, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "custom/subscribe", loaded.App.SubscribePath)
}

func TestPrepareSubscribePathNormalizesAndStoresValue(t *testing.T) {
	resetConfig()
	t.Cleanup(resetConfig)

	cfg := &Config{App: AppConfig{SubscribePath: " /custom//subscribe/ "}}
	assert.Equal(t, "custom/subscribe", PrepareSubscribePath(cfg))
	assert.Equal(t, "custom/subscribe", cfg.App.SubscribePath)
	assert.Same(t, cfg, Get())
}
