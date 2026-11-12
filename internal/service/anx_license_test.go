package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/entitlement"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// The license's user_id is the one anx-node derives from the user key the
// panel hands the node (anx spec §2.1).
func TestANXLicenseUserIDMatchesTheUserKey(t *testing.T) {
	uuid := "C9A1E7B0-5B0E-4A4B-9A54-2F3F1C1B0D11"
	key, err := hex.DecodeString(ANXUserKey(uuid))
	require.NoError(t, err)
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("anx/v1/user-id"))
	got, ok := entitlement.ANXUserID(uuid)
	require.True(t, ok)
	require.Equal(t, mac.Sum(nil)[:8], got[:])
}

func TestANXNodeConfigLicense(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}))
	licenses := &entitlement.Service{DB: db, KEK: func() string { return strings.Repeat("42", 32) }}

	// Off: nothing goes out, for anx-nodes that do not know the keys.
	config := map[string]any{}
	applyANXLicense(licenses, config)
	require.NotContains(t, config, "license_ca")

	require.NoError(t, db.Create(&model.SystemConfig{Key: entitlement.KeyANXEnabled, Value: "true"}).Error)
	config = map[string]any{}
	applyANXLicense(licenses, config)
	require.Equal(t, []string{"c83c22218d9d871e9eb6196777b1ee9715a34420d42646c09e6bbedf3687de26"}, config["license_ca"])
	require.NotContains(t, config, "require_license")

	require.NoError(t, db.Create(&model.SystemConfig{Key: entitlement.KeyANXRequire, Value: "true"}).Error)
	config = map[string]any{}
	applyANXLicense(licenses, config)
	require.Equal(t, true, config["require_license"])

	// anx-node refuses require_license beside the wg jacket.
	config = map[string]any{"wg_listen_port": 51820}
	applyANXLicense(licenses, config)
	require.Contains(t, config, "license_ca")
	require.NotContains(t, config, "require_license")
}
