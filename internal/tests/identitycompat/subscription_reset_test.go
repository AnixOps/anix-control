package identitycompat

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	kernelservice "github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
)

const subscriptionResetPath = "/api/v2/user/subscription/reset"

func subscriptionResetRoute() packagecompat.Route {
	return packagecompat.Route{
		Method: "POST", Pattern: subscriptionResetPath, RouteID: native.UserSubscriptionResetRouteID, Models: Models,
		Legacy: func(c *gin.Context) { handler.NewUserHandler().ResetSubscription(c) },
		Native: nativeRoute(native.UserSubscriptionResetRouteID),
	}
}

func resetBody(fields map[string]string) []byte {
	encoded, _ := json.Marshal(fields)
	return encoded
}

// A user's reset of their own subscription link answers, refuses and
// changes Control exactly as the legacy handler does: the same new-token
// rotation and request ledger entry as the administrator's reset, the same
// re-authentication (password, or a TOTP or recovery code with a second
// factor) and the same per-user limit of three attempts an hour.
func TestUserSubscriptionResetParity(t *testing.T) {
	code, err := totp.GenerateCode(totpSecret, time.Now())
	require.NoError(t, err)
	issued := []string{"data.token"}
	withPassword := resetBody(map[string]string{"password": password})
	wrongPassword := resetBody(map[string]string{"password": "wrong"})
	for _, c := range []packagecompat.Case{
		{Name: "password", Principal: member, Body: withPassword, Mask: issued},
		{Name: "retried request applies once", Principal: member, Body: withPassword, Mask: issued, Warmup: [][]byte{withPassword}},
		{Name: "wrong password", Principal: member, Body: wrongPassword},
		{Name: "missing password", Principal: member, Body: []byte(`{}`)},
		{Name: "a code does not replace the password", Principal: member, Body: resetBody(map[string]string{"code": code})},
		{Name: "empty body", Principal: member},
		{Name: "malformed body", Principal: member, Body: []byte(`{"password":`)},
		{Name: "unknown user", Principal: nobody, Body: withPassword},
		{Name: "mfa requires a code", Principal: mfa, Body: withPassword},
		{Name: "mfa wrong code", Principal: mfa, Body: resetBody(map[string]string{"code": "000000", "method": "totp"})},
		{Name: "mfa totp", Principal: mfa, Body: resetBody(map[string]string{"code": code, "method": "totp"}), Mask: issued},
		{Name: "mfa recovery code", Principal: mfa, Body: resetBody(map[string]string{"code": "CCCC-DDDD", "method": "backup"}), Mask: issued},
		{Name: "mfa recovery code without method", Principal: mfa, Body: resetBody(map[string]string{"code": " AAAA-BBBB "}), Mask: issued},
		{
			Name: "rate limited after three wrong passwords", Principal: member, Body: withPassword, Headers: []string{"Retry-After"},
			Warmup: [][]byte{wrongPassword, wrongPassword, wrongPassword},
		},
		{
			Name: "successful resets count toward the limit", Principal: member, Body: withPassword, Headers: []string{"Retry-After"},
			Warmup: [][]byte{withPassword, withPassword, withPassword},
		},
		{
			Name: "missing credentials do not count", Principal: member, Body: withPassword, Mask: issued,
			Warmup: [][]byte{[]byte(`{}`), []byte(`{}`), []byte(`{}`), wrongPassword, wrongPassword},
		},
	} {
		c.Path = subscriptionResetPath
		c.Seed = seedUsers(nil, nil)
		c.Snapshot = resetState
		c.RequestHeaders = map[string]string{"Idempotency-Key": "self-reset-1"}
		packagecompat.RunWrite(t, subscriptionResetRoute(), c)
	}
}

// Natively, through the real KernelSubscriber, the reset changes Control's
// subscription token, keeps the proxy uuid and records the user's request.
func TestUserSubscriptionResetChangesControlsToken(t *testing.T) {
	db, identity, _ := controlWithIdentity(t)
	response, err := identity.Handlers()[native.UserSubscriptionResetRouteID](context.Background(), pluginhostsdk.NativeRequest{
		RouteID: native.UserSubscriptionResetRouteID, Principal: member, Body: resetBody(map[string]string{"password": password}),
		Metadata: pluginhostsdk.RequestMetadata{Headers: map[string][]string{"Idempotency-Key": {"self-1"}}},
	})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &decoded), "%s", response.Body)
	token, ok := data(t, decoded)["token"].(string)
	require.True(t, ok, "%v", decoded)
	var stored model.User
	require.NoError(t, db.Take(&stored, 2).Error)
	require.Equal(t, token, stored.Token)
	require.NotEqual(t, "t2", stored.Token)
	require.Equal(t, "u2", stored.UUID)
	var request model.SubscriberRequest
	require.NoError(t, db.Take(&request).Error)
	require.Equal(t, kernelservice.UserSubscriptionResetRequestID(2, "self-1"), request.RequestID)
}

// An installation finalized before the route joined group A has no stored
// mode for it: the kernel resolves it native, and the native handler resets
// with identity's credentials although Control no longer holds any.
func TestUserSubscriptionResetWorksOnAFinalizedInstallWithoutItsStoredMode(t *testing.T) {
	db, identity, _ := controlWithIdentity(t)
	require.NoError(t, db.Save(&model.IdentityAuthority{ID: 1, State: model.IdentityAuthorityFinalized, UpdatedAt: time.Now()}).Error)
	require.NoError(t, db.Model(&model.User{}).Where("1 = 1").Update("password", model.UnusableLegacyPassword).Error)
	require.NoError(t, db.Where("1 = 1").Delete(&model.UserMFA{}).Error)

	stored := map[string]string{}
	for _, route := range kernelservice.IdentityGroupARoutes {
		if route != native.UserSubscriptionResetRouteID {
			stored[route] = "native"
		}
	}
	modes, err := kernelservice.ResolvePackageRouteModes(db, kernelservice.IdentityPlatformPackageID, stored)
	require.NoError(t, err)
	require.Equal(t, "native", modes[native.UserSubscriptionResetRouteID])

	code, err := totp.GenerateCode(totpSecret, time.Now())
	require.NoError(t, err)
	for _, c := range []struct {
		principal pluginhostsdk.Principal
		body      []byte
	}{
		{member, resetBody(map[string]string{"password": password})},
		{mfa, resetBody(map[string]string{"code": code, "method": "totp"})},
	} {
		response, err := identity.Handlers()[native.UserSubscriptionResetRouteID](context.Background(), pluginhostsdk.NativeRequest{
			RouteID: native.UserSubscriptionResetRouteID, Principal: c.principal, Body: c.body,
		})
		require.NoError(t, err)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(response.Body, &decoded), "%s", response.Body)
		token, ok := data(t, decoded)["token"].(string)
		require.True(t, ok, "%v", decoded)
		var stored model.User
		require.NoError(t, db.Take(&stored, c.principal.ActorID).Error)
		require.Equal(t, token, stored.Token)
	}
}
