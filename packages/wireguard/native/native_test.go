package native

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

// Every request answers a valid, fresh X25519 keypair in the panel envelope.
func TestKeypairsAreValidAndFresh(t *testing.T) {
	seen := map[string]bool{}
	for range 3 {
		response, err := (&Service{}).Keypair(context.Background(), pluginhostsdk.NativeRequest{})
		require.NoError(t, err)
		require.EqualValues(t, http.StatusOK, response.StatusCode)
		var answer struct {
			Code int `json:"code"`
			Data struct {
				PrivateKey string `json:"private_key"`
				PublicKey  string `json:"public_key"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(response.Body, &answer))
		require.Zero(t, answer.Code)
		private, err := base64.StdEncoding.DecodeString(answer.Data.PrivateKey)
		require.NoError(t, err)
		key, err := ecdh.X25519().NewPrivateKey(private)
		require.NoError(t, err)
		require.Equal(t, base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), answer.Data.PublicKey)
		require.False(t, seen[answer.Data.PrivateKey])
		seen[answer.Data.PrivateKey] = true
	}
}
