package nodesecrets

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecretPositions(t *testing.T) {
	document := `{"private_key":"fake-reality-private","public_key":"fake-public","short_id":"6ba85179",` +
		`"nested":{"password":"fake-hy2","port":443},"users":[{"password":"fake-user"},{"email":"a@example.test"}],` +
		`"psk":["fake-k1","fake-k2"],"password_required":true,"server_key":"","token":null,` +
		`"a/b":{"x~y_key":"fake-escaped"},"large":12345678901234567890}`
	require.Equal(t, []Position{
		{Pointer: "/a~1b/x~0y_key", Value: `"fake-escaped"`},
		{Pointer: "/nested/password", Value: `"fake-hy2"`},
		{Pointer: "/private_key", Value: `"fake-reality-private"`},
		{Pointer: "/psk", Value: `["fake-k1","fake-k2"]`},
		{Pointer: "/users/0/password", Value: `"fake-user"`},
	}, SecretPositions(document))

	// Nothing secret, blank and empty: no positions.
	require.Empty(t, SecretPositions(`{"flow":"xtls-rprx-vision","server_key":""}`))
	require.Empty(t, SecretPositions(""))
	require.Empty(t, SecretPositions("   "))
	// A JSON value that is not an object holds no keys.
	require.Empty(t, SecretPositions(`"fake-string"`))
	// Text that is not JSON is one secret, the whole column.
	require.Equal(t, []Position{{Value: "private_key=fake"}}, SecretPositions("private_key=fake"))
	// The placeholder marks a secret kept in the new table only.
	require.Equal(t, []Position{{Pointer: "/password", Value: `"********"`, Moved: true}},
		SecretPositions(`{"password":"********"}`))
	require.Equal(t, []Position{{Value: Placeholder, Moved: true}}, SecretPositions(Placeholder))
	// HTML characters are not escaped, numbers keep their digits.
	require.Equal(t, []Position{{Pointer: "/auth", Value: `["<fake>&",1.50]`}}, SecretPositions(`{"auth":["<fake>&",1.50]}`))
}

func TestIsSecretKey(t *testing.T) {
	for _, key := range []string{"private_key", "privateKey", "preshared_key", "obfs-password", "psk", "auth", "seed", "api_token", "credentials"} {
		require.True(t, IsSecretKey(key), key)
	}
	for _, key := range []string{"public_key", "short_id", "key_file", "private_key_path", "obfs", "flow", ""} {
		require.False(t, IsSecretKey(key), key)
	}
}

func TestTombstone(t *testing.T) {
	require.Equal(t, "!moved:12", Tombstone(12))
	require.NotEqual(t, Tombstone(1), Tombstone(2))
	require.True(t, IsTombstone(Tombstone(7)))
	require.False(t, IsTombstone(""))
	require.False(t, IsTombstone("0123abcdef"))
}
