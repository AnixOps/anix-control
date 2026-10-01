package handler

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/stretchr/testify/require"
)

// The affiliate package reads the frontend settings this handler writes
// through kapi_affiliate_settings_v1, which shows exactly one key.
func TestInviteSettingsKeyIsTheViewsKey(t *testing.T) {
	require.Equal(t, packagestore.InviteSettingsKey, inviteFrontendConfigKey)
}
