package forwardv4

import (
	"net/http"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// Entry HA through DNS (L2) over the real ForwardControl: provider
// credentials go in, are sealed, and never come back out.
func TestForwardV4DNS(t *testing.T)         { runForwardV4DNS(t, openSQLite(t)) }
func TestPostgresForwardV4DNS(t *testing.T) { runForwardV4DNS(t, openPostgres(t)) }

func runForwardV4DNS(t *testing.T, db *gorm.DB) {
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	e := newEnv(t, db)
	const secret = "cf-api-token-0123456789"

	// Without a key-encryption key no credential is stored.
	refused := e.call("POST", "/dns/providers", `{"provider":{"name":"cf","kind":"DNS_PROVIDER_KIND_CLOUDFLARE"},"credentials":{"api_token":"`+secret+`"}}`, "")
	require.Equal(t, http.StatusConflict, refused.status, refused.raw)
	require.Equal(t, []string{"secret_store_unavailable"}, refused.violationCodes())

	e.kernel.DNSKEK = func() string { return "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=" }
	created := e.call("POST", "/dns/providers", `{"provider":{"name":"cf","kind":"DNS_PROVIDER_KIND_CLOUDFLARE"},"credentials":{"api_token":"`+secret+`"}}`, "p-1")
	require.Equal(t, http.StatusCreated, created.status, created.raw)
	require.NotContains(t, created.raw, secret)
	provider := created.data()["provider"].(map[string]any)
	providerID := provider["id"].(string)
	require.Equal(t, []any{"api_token"}, provider["credential_names"])
	var row model.KernelForwardDNSProvider
	require.NoError(t, db.First(&row).Error)
	require.NotContains(t, row.SealedCredentials+row.ConfigJSON, secret)

	invalid := e.call("POST", "/dns/providers", `{"provider":{"name":"hook","kind":"DNS_PROVIDER_KIND_WEBHOOK","config":{"url":"http://example.com"}},"credentials":{}}`, "")
	require.Equal(t, http.StatusBadRequest, invalid.status, invalid.raw)
	require.Equal(t, "invalid_request", invalid.errorCode())
	require.ElementsMatch(t, []string{"invalid_format", "required"}, invalid.violationCodes())

	for _, path := range []string{"/dns/providers", "/dns/providers/" + providerID} {
		read := e.call("GET", path, "", "")
		require.Equal(t, http.StatusOK, read.status, read.raw)
		require.NotContains(t, read.raw, secret)
	}
	updated := e.call("PUT", "/dns/providers/"+providerID, `{"provider":{"name":"cf-main"},"credentials":{"api_token":"********"}}`, "")
	require.Equal(t, http.StatusOK, updated.status, updated.raw)
	require.NotContains(t, updated.raw, secret)

	// A route with an entry name, bound in CNAME mode.
	routeBody := strings.Replace(route("31000", "LINK_SECURITY_RAW"), `"protocol":"L4_PROTOCOL_TCP"`, `"protocol":"L4_PROTOCOL_TCP","entry_hostname":"hk.example.com"`, 1)
	createdRoute := e.call("POST", "/routes", routeBody, "")
	require.Equal(t, http.StatusCreated, createdRoute.status, createdRoute.raw)
	routeID := createdRoute.data()["route"].(map[string]any)["id"].(string)
	unbound := e.call("GET", "/routes/"+routeID+"/dns", "", "")
	require.Equal(t, http.StatusOK, unbound.status, unbound.raw)
	require.Equal(t, "unbound", unbound.data()["status"].(map[string]any)["state"])

	binding := e.call("POST", "/dns/bindings", `{"route_id":"`+routeID+`","provider_id":"`+providerID+`","zone":"ha.example.net","mode":"DNS_BINDING_MODE_CNAME","record_name":"r1.ha.example.net"}`, "")
	require.Equal(t, http.StatusCreated, binding.status, binding.raw)
	bindingID := binding.data()["binding"].(map[string]any)["id"].(string)
	status := e.call("GET", "/routes/"+routeID+"/dns", "", "").data()["status"].(map[string]any)
	require.Equal(t, "pending", status["state"])
	require.Equal(t, "r1.ha.example.net", status["cname_target"])
	require.Equal(t, "hk.example.com", status["entry_hostname"])

	mismatch := e.call("POST", "/dns/bindings", `{"route_id":"`+routeID+`","provider_id":"`+providerID+`","zone":"example.com","record_name":"other.example.com"}`, "")
	require.Equal(t, http.StatusBadRequest, mismatch.status, mismatch.raw)
	require.Contains(t, mismatch.violationCodes(), "hostname_mismatch")

	inUse := e.call("DELETE", "/dns/providers/"+providerID, "", "")
	require.Equal(t, http.StatusConflict, inUse.status, inUse.raw)
	require.Equal(t, []string{"provider_in_use"}, inUse.violationCodes())

	list := e.call("GET", "/dns/bindings?route_id="+routeID, "", "")
	require.Len(t, list.data()["bindings"], 1)
	require.Equal(t, http.StatusOK, e.call("DELETE", "/dns/bindings/"+bindingID+"?purge=true", "", "").status)
	require.Equal(t, http.StatusOK, e.call("DELETE", "/dns/providers/"+providerID, "", "").status)
	require.Equal(t, http.StatusNotFound, e.call("GET", "/dns/providers/"+providerID, "", "").status)
}
