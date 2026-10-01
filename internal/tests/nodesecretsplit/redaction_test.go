package nodesecretsplit

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/stretchr/testify/require"
)

// TestSecretPositionsMatchRedaction: the positions the split stores are
// exactly the values the administrators' answers mask. Putting the stored
// values back into the masked document gives the original, so the redacted
// document a finalized column will hold (section 4.4) loses nothing.
func TestSecretPositionsMatchRedaction(t *testing.T) {
	for _, document := range []string{
		`{"private_key":"fake-reality-private","public_key":"reality-public","short_id":"6ba85179","dest":"www.example.test:443"}`,
		`{"server_port":51820,"wireguard":{"private_key":"fake-raw-private","peers":[{"preshared_key":"fake-psk","public_key":"peer-public"}]}}`,
		`{"users":[{"password":"fake-u1","email":"a@example.test"},{"password":"","email":"b@example.test"}],"psk":["fake-k1"],"auth":null,"password_required":true}`,
		`{"a/b":{"x~y_token":"fake-escaped"},"list":[[{"secret":"fake-deep"}]],"large":12345678901234567890}`,
		`{"flow":"xtls-rprx-vision"}`,
		`fake-not-json`,
		``,
	} {
		masked := service.RedactNodeSecretsJSON(document)
		positions := nodesecrets.SecretPositions(document)
		if len(positions) == 0 {
			require.Equal(t, document, masked, document)
			continue
		}
		if positions[0].Pointer == "" {
			require.Equal(t, service.NodeSecretPlaceholder, masked)
			require.Equal(t, document, positions[0].Value)
			continue
		}
		var maskedValue, original any
		require.NoError(t, json.Unmarshal([]byte(masked), &maskedValue))
		require.NoError(t, json.Unmarshal([]byte(document), &original))
		for _, position := range positions {
			require.Equal(t, service.NodeSecretPlaceholder, valueAt(t, maskedValue, position.Pointer), "%s at %s", document, position.Pointer)
			var value any
			require.NoError(t, json.Unmarshal([]byte(position.Value), &value))
			setAt(t, maskedValue, position.Pointer, value)
		}
		require.Equal(t, original, maskedValue, document)
		require.NotContains(t, masked, "fake-", "no secret survives the mask")
	}
}

func pointerTokens(pointer string) []string {
	tokens := strings.Split(strings.TrimPrefix(pointer, "/"), "/")
	for i, token := range tokens {
		tokens[i] = strings.ReplaceAll(strings.ReplaceAll(token, "~1", "/"), "~0", "~")
	}
	return tokens
}

func step(t *testing.T, value any, token string) any {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		return typed[token]
	case []any:
		index, err := strconv.Atoi(token)
		require.NoError(t, err)
		return typed[index]
	}
	t.Fatalf("no %q in a scalar", token)
	return nil
}

func valueAt(t *testing.T, value any, pointer string) any {
	t.Helper()
	for _, token := range pointerTokens(pointer) {
		value = step(t, value, token)
	}
	return value
}

func setAt(t *testing.T, value any, pointer string, replacement any) {
	t.Helper()
	tokens := pointerTokens(pointer)
	parent := value
	for _, token := range tokens[:len(tokens)-1] {
		parent = step(t, parent, token)
	}
	last := tokens[len(tokens)-1]
	switch typed := parent.(type) {
	case map[string]any:
		typed[last] = replacement
	case []any:
		index, err := strconv.Atoi(last)
		require.NoError(t, err)
		typed[index] = replacement
	default:
		t.Fatalf("cannot set %s", pointer)
	}
}
