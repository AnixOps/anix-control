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

// The e-mail configuration is one JSON value holding the SMTP password: an
// administrator's answer shows the value with the password masked, and a
// value sent back with the placeholder keeps the stored password.
func TestEmailConfigPasswordIsMaskedInsideTheValue(t *testing.T) {
	const key = "notification.email.config"
	stored := `{"host":"smtp.test", "port":465,"password" : "smtp-secret","username":"u"}`
	masked := MaskSystemConfigFields(key, stored)
	require.Equal(t, `{"host":"smtp.test", "port":465,"password" : "********","username":"u"}`, masked, "only the password changes")
	display, sensitive, hasValue := MaskSystemConfigValue(key, stored)
	require.Equal(t, masked, display)
	require.False(t, sensitive, "the value stays editable")
	require.True(t, hasValue)

	for value, want := range map[string]string{
		`{"host":"h","password":""}`:              `{"host":"h","password":""}`,
		`{"host":"h","password":"  "}`:            `{"host":"h","password":"  "}`,
		`{"host":"h","password":null}`:            `{"host":"h","password":null}`,
		`{"host":"h","password":7}`:               `{"host":"h","password":"********"}`,
		`{"password":"a","password":"b"}`:         `{"password":"********","password":"********"}`,
		`{"nested":{"password":"x"},"port":1.50}`: `{"nested":{"password":"x"},"port":1.50}`,
		`[1,2]`:    `[1,2]`,
		`{"host":`: "********",
		"   ":      "   ",
	} {
		require.Equal(t, want, MaskSystemConfigFields(key, value), value)
	}
	require.Equal(t, `{"password":"x"}`, MaskSystemConfigFields("site.json", `{"password":"x"}`), "other keys have no masked fields")

	kept, ok := KeepSystemConfigFields(key, `{"host":"new","password":"********","port":25}`, stored)
	require.True(t, ok)
	require.Equal(t, `{"host":"new","password":"smtp-secret","port":25}`, kept)
	replaced, ok := KeepSystemConfigFields(key, `{"host":"new","password":"rotated"}`, stored)
	require.False(t, ok)
	require.Equal(t, `{"host":"new","password":"rotated"}`, replaced)
	whole, ok := KeepSystemConfigFields(key, "********", stored)
	require.True(t, ok)
	require.Equal(t, stored, whole)
	none, ok := KeepSystemConfigFields(key, `{"password":"********"}`, "")
	require.False(t, ok, "nothing stored, nothing kept")
	require.Equal(t, `{"password":""}`, none, "the placeholder is never stored")
	escaped, ok := KeepSystemConfigFields(key, `{"password":"********"}`, `{"password":"a<b"}`)
	require.True(t, ok)
	require.Equal(t, `{"password":"a\u003cb"}`, escaped, "as encoding/json writes it")
	other, ok := KeepSystemConfigFields("site.json", `{"password":"********"}`, `{"password":"x"}`)
	require.False(t, ok)
	require.Equal(t, `{"password":"********"}`, other)
}
