package service

import (
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
)

func TestIsNodeSecretKey(t *testing.T) {
	for _, key := range []string{
		"private_key", "privateKey", "server_private_key", "preshared_key", "server_key", "key", "keys",
		"password", "obfs-password", "obfs_password", "auth", "auth_str", "psk", "pass", "seed",
		"token", "api_token", "secret", "client_secret", "credentials", "api_key",
	} {
		require.True(t, IsNodeSecretKey(key), key)
	}
	for _, key := range []string{
		"public_key", "publicKey", "server_public_key", "short_id", "short_ids", "shortId",
		"key_file", "wss_key_file", "private_key_path", "cert_file", "obfs", "dest", "server_names",
		"fingerprint", "flow", "method", "cipher", "cidr", "authority", "host", "path", "",
	} {
		require.False(t, IsNodeSecretKey(key), key)
	}
}

func TestRedactNodeSecretsJSON(t *testing.T) {
	redacted := RedactNodeSecretsJSON(`{"private_key":"reality-private","public_key":"reality-public","short_id":"6ba85179",` +
		`"nested":{"password":"hy2-auth","port":443},"users":[{"password":"user-pass","email":"a@example.test"}],` +
		`"psk":["k1","k2"],"password_required":true,"server_key":"","token":null,"large":12345678901234567890}`)
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(redacted), &got))
	require.Equal(t, NodeSecretPlaceholder, got["private_key"])
	require.Equal(t, "reality-public", got["public_key"])
	require.Equal(t, "6ba85179", got["short_id"])
	require.Equal(t, NodeSecretPlaceholder, got["nested"].(map[string]any)["password"])
	require.EqualValues(t, 443, got["nested"].(map[string]any)["port"])
	user := got["users"].([]any)[0].(map[string]any)
	require.Equal(t, NodeSecretPlaceholder, user["password"])
	require.Equal(t, "a@example.test", user["email"])
	require.Equal(t, NodeSecretPlaceholder, got["psk"])
	require.Equal(t, true, got["password_required"], "a flag is not a secret")
	require.Equal(t, "", got["server_key"], "an empty secret shows that none is set")
	require.Nil(t, got["token"])
	require.Contains(t, redacted, "12345678901234567890", "numbers keep their digits")

	// Nothing to mask: the text is returned as it is.
	plain := `{ "flow": "xtls-rprx-vision" }`
	require.Equal(t, plain, RedactNodeSecretsJSON(plain))
	require.Equal(t, "", RedactNodeSecretsJSON(""))
	// Text that is not JSON cannot be told apart and is masked whole.
	require.Equal(t, NodeSecretPlaceholder, RedactNodeSecretsJSON(`private_key=abc`))
}

func TestKeepNodeSecretsJSON(t *testing.T) {
	stored := `{"private_key":"reality-private","public_key":"old-public","users":[{"password":"p1"},{"password":"p2"}],"psk":["k1"]}`
	masked := RedactNodeSecretsJSON(stored)
	require.NotContains(t, masked, "reality-private")

	// Saved back unchanged, every secret keeps its stored value.
	var kept map[string]any
	require.NoError(t, json.Unmarshal([]byte(KeepNodeSecretsJSON(masked, stored)), &kept))
	var original map[string]any
	require.NoError(t, json.Unmarshal([]byte(stored), &original))
	require.Equal(t, original, kept)

	// A new value replaces the stored one; other placeholders keep theirs.
	incoming := `{"private_key":"new-private","public_key":"new-public","users":[{"password":"********"}],"psk":"********"}`
	require.NoError(t, json.Unmarshal([]byte(KeepNodeSecretsJSON(incoming, stored)), &kept))
	require.Equal(t, "new-private", kept["private_key"])
	require.Equal(t, "new-public", kept["public_key"])
	require.Equal(t, "p1", kept["users"].([]any)[0].(map[string]any)["password"])
	require.Equal(t, []any{"k1"}, kept["psk"])

	// Nothing stored: the placeholder stands for no secret.
	require.JSONEq(t, `{"private_key":""}`, KeepNodeSecretsJSON(`{"private_key":"********"}`, ""))
	// A non-secret key keeps the text it was given.
	require.JSONEq(t, `{"remark":"********"}`, KeepNodeSecretsJSON(`{"remark":"********"}`, `{"remark":"x"}`))
	// The whole value sent as the placeholder keeps the whole stored value.
	require.Equal(t, "not json", KeepNodeSecretsJSON(NodeSecretPlaceholder, "not json"))
	// Without a placeholder the text is returned as it is.
	require.Equal(t, `{ "private_key": "x" }`, KeepNodeSecretsJSON(`{ "private_key": "x" }`, stored))
}

func TestRedactNodeMasksProtocolsAndRawConfig(t *testing.T) {
	reality := `{"private_key":"reality-private","public_key":"reality-public","short_id":"ab"}`
	raw := `{"server_private_key":"raw-private","server_port":51820}`
	node := &model.Node{
		RawConfig: &raw,
		Protocols: []model.NodeProtocol{{RealitySettings: &reality}},
	}
	RedactNode(node)
	require.NotContains(t, *node.RawConfig, "raw-private")
	require.Contains(t, *node.RawConfig, "51820")
	require.NotContains(t, *node.Protocols[0].RealitySettings, "reality-private")
	require.Contains(t, *node.Protocols[0].RealitySettings, "reality-public")
	require.Nil(t, node.Protocols[0].Settings)

	keys := []model.AuthorizedKey{{Key: "registration-key"}, {}}
	MaskAuthorizedKeys(keys)
	require.Equal(t, NodeSecretPlaceholder, keys[0].Key)
	require.Equal(t, "", keys[1].Key)
}
