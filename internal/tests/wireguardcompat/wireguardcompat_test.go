// Package wireguardcompat proves the wireguard package's native keypair
// route answers as the kernel's legacy handler: the same envelope, with the
// random keys masked (packagecompat), and on both sides a fresh X25519
// keypair whose public key derives from its private key, in standard
// base64. The route reads no table; both backends run for uniformity.
package wireguardcompat

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/wireguard/native"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const (
	path    = "/api/v2/admin/wireguard/keypair"
	routeID = "wireguard.admin.wireguard.keypair.post"
)

var admin = pluginhostsdk.Principal{ActorID: 1, Admin: true}

func legacy(c *gin.Context) { handler.NewNodeHandler().GenerateWireGuardKeypair(c) }

func TestWireGuardKeypairRouteParity(t *testing.T) {
	route := packagecompat.Route{
		Method: "POST", Pattern: path, RouteID: routeID, Legacy: legacy,
		Native: func(*gorm.DB) pluginhostsdk.NativeHandler { return (&native.Service{}).Handlers()[routeID] },
	}
	for _, c := range []packagecompat.Case{
		{Name: "no body"},
		{Name: "a body, ignored", Body: []byte(`{"private_key":"chosen","public_key":"chosen"}`)},
		{Name: "a body that is not JSON, ignored", Body: []byte(`not json`)},
	} {
		c.Path, c.Principal = path, admin
		c.Mask = []string{"data.private_key", "data.public_key"}
		packagecompat.RunRead(t, route, c)
	}
}

// keypair decodes an answer's keys and checks the pair: 32 bytes each, in
// standard base64, the public key derived from the private one.
func keypair(t *testing.T, body []byte) (string, string) {
	t.Helper()
	var answer struct {
		Code int `json:"code"`
		Data struct {
			PrivateKey string `json:"private_key"`
			PublicKey  string `json:"public_key"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(body, &answer))
	require.Zero(t, answer.Code)
	private, err := base64.StdEncoding.DecodeString(answer.Data.PrivateKey)
	require.NoError(t, err)
	require.Len(t, private, 32)
	key, err := ecdh.X25519().NewPrivateKey(private)
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), answer.Data.PublicKey)
	return answer.Data.PrivateKey, answer.Data.PublicKey
}

// Both sides generate a valid, fresh keypair on every request.
func TestWireGuardKeypairsAreValidAndFresh(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST(path, legacy)
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, bytes.NewReader(nil)))
		require.Equal(t, http.StatusOK, recorder.Code)
		private, _ := keypair(t, recorder.Body.Bytes())
		require.False(t, seen[private])
		seen[private] = true

		response, err := (&native.Service{}).Keypair(context.Background(), pluginhostsdk.NativeRequest{})
		require.NoError(t, err)
		require.EqualValues(t, http.StatusOK, response.StatusCode)
		private, _ = keypair(t, response.Body)
		require.False(t, seen[private])
		seen[private] = true
	}
}
