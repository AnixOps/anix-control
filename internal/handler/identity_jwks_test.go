package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/AnixOps/anix-control/sdk/identitytoken"
	"github.com/AnixOps/anix-control/v4/internal/authn"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type jwksKeys struct{ set identitytoken.KeySet }

func (k jwksKeys) KeySet() identitytoken.KeySet { return k.set }
func (jwksKeys) Kick()                          {}

func TestIdentityJWKSPublishesUnrevokedKeys(t *testing.T) {
	gin.SetMode(gin.TestMode)
	live, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	revoked, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	authn.SetDefaultIdentityKeys(jwksKeys{set: identitytoken.KeySet{
		"idk-live": {ID: "idk-live", PublicKey: live}, "idk-revoked": {ID: "idk-revoked", PublicKey: revoked, Revoked: true},
	}})
	t.Cleanup(func() { authn.SetDefaultIdentityKeys(nil) })

	router := gin.New()
	router.GET("/jwks.json", IdentityJWKS)
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/jwks.json", nil))

	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "public, max-age=300", recorder.Header().Get("Cache-Control"))
	keys, err := identitytoken.ParseJWKS(recorder.Body.Bytes())
	require.NoError(t, err)
	require.Equal(t, identitytoken.KeySet{"idk-live": {ID: "idk-live", PublicKey: live}}, keys)
}
