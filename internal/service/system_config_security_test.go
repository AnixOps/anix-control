package service

import (
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/stretchr/testify/require"
)

// kapi_affiliate_settings_v1 shows one v2_system_config row to the
// affiliate package. That is safe only while the kernel itself does not
// treat the key as sensitive: its administrator answers show the value in
// clear.
func TestTheAffiliateSettingsViewShowsNoSensitiveKey(t *testing.T) {
	require.False(t, IsSensitiveSystemConfigKey(packagestore.InviteSettingsKey))
	display, sensitive, _ := MaskSystemConfigValue(packagestore.InviteSettingsKey, `{"code_prefix":"AFF"}`)
	require.False(t, sensitive)
	require.Equal(t, `{"code_prefix":"AFF"}`, display)
	require.True(t, IsSensitiveSystemConfigKey("smtp.password"), "the classifier still flags secrets")
}
