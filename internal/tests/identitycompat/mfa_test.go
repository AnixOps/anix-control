package identitycompat

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func mfaRoute(method, pattern, routeID string, legacy func(*handler.MFAHandler, *gin.Context)) packagecompat.Route {
	return packagecompat.Route{
		Method: method, Pattern: pattern, RouteID: routeID, Models: Models,
		Legacy: func(c *gin.Context) { legacy(handler.NewMFAHandler(), c) },
		Native: nativeRoute(routeID),
	}
}

var (
	member = pluginhostsdk.Principal{ActorID: 2}
	mfa    = pluginhostsdk.Principal{ActorID: 5}
	nobody = pluginhostsdk.Principal{ActorID: 99}
	admin  = pluginhostsdk.Principal{ActorID: 1, Admin: true}
)

func run(t *testing.T, route packagecompat.Route, cases []packagecompat.Case, path string) {
	t.Helper()
	for _, c := range cases {
		c.Path = path
		if c.Seed == nil {
			c.Seed = seedUsers(nil, nil)
		}
		packagecompat.RunRead(t, route, c)
	}
}

// unfinishedMFA leaves the member's second factor set up but not enabled.
func unfinishedMFA(t testing.TB, db *gorm.DB) {
	require.NoError(t, db.Model(&model.UserMFA{}).Where("user_id = ?", 5).Update("enabled", false).Error)
}

func usedMFA(t testing.TB, db *gorm.DB) {
	used := time.Now().Add(-time.Hour)
	require.NoError(t, db.Model(&model.UserMFA{}).Where("user_id = ?", 5).Updates(map[string]any{"last_used": used, "last_method": "totp"}).Error)
}

func TestMFAStatusParity(t *testing.T) {
	run(t, mfaRoute("GET", "/api/v2/user/mfa/status", "identity.user.mfa.status.get", (*handler.MFAHandler).GetStatus), []packagecompat.Case{
		{Name: "no second factor", Principal: member},
		{Name: "enabled, never used", Principal: mfa},
		{Name: "used before", Principal: mfa, Seed: seedUsers(nil, usedMFA), Mask: []string{"data.last_used"}},
	}, "/api/v2/user/mfa/status")
}

func TestMFASetupAndEnableParity(t *testing.T) {
	secrets := []string{"data.secret", "data.url", "data.qr_code", "data.backup_codes"}
	run(t, mfaRoute("POST", "/api/v2/user/mfa/totp/setup", "identity.user.mfa.totp.setup.post", (*handler.MFAHandler).SetupTOTP), []packagecompat.Case{
		{Name: "new second factor", Principal: member, Mask: secrets},
		{Name: "an enabled second factor is not replaced by a session alone", Principal: mfa},
		{Name: "an unfinished setup is replaced", Principal: mfa, Mask: secrets, Seed: seedUsers(nil, unfinishedMFA)},
		{Name: "unknown user", Principal: nobody},
	}, "/api/v2/user/mfa/totp/setup")

	code, err := totp.GenerateCode(totpSecret, time.Now())
	require.NoError(t, err)
	run(t, mfaRoute("POST", "/api/v2/user/mfa/totp/enable", "identity.user.mfa.totp.enable.post", (*handler.MFAHandler).EnableTOTP), []packagecompat.Case{
		{Name: "missing code", Principal: mfa, Body: []byte(`{}`)},
		{Name: "not set up", Principal: member, Body: []byte(`{"code":"123456"}`)},
		{Name: "wrong code", Principal: mfa, Body: []byte(`{"code":"000000"}`)},
		{Name: "valid code", Principal: mfa, Body: []byte(`{"code":"` + code + `"}`)},
	}, "/api/v2/user/mfa/totp/enable")
}

func TestMFAVerifyParity(t *testing.T) {
	code, err := totp.GenerateCode(totpSecret, time.Now())
	require.NoError(t, err)
	run(t, mfaRoute("POST", "/api/v2/user/mfa/verify", "identity.user.mfa.verify.post", (*handler.MFAHandler).VerifyMFA), []packagecompat.Case{
		{Name: "missing code", Principal: mfa, Body: []byte(`{"method":"totp"}`)},
		{Name: "totp", Principal: mfa, Body: []byte(`{"code":"` + code + `","method":"TOTP"}`)},
		{Name: "backup code", Principal: mfa, Body: []byte(`{"code":"AAAA-BBBB"}`)},
		{Name: "wrong code", Principal: mfa, Body: []byte(`{"code":"000000"}`)},
		{Name: "sms is not implemented", Principal: mfa, Body: []byte(`{"code":"123456","method":"sms"}`)},
		{Name: "no second factor passes", Principal: member, Body: []byte(`{"code":"000000"}`)},
	}, "/api/v2/user/mfa/verify")
}

func TestMFADisableAndRegenerateParity(t *testing.T) {
	run(t, mfaRoute("POST", "/api/v2/user/mfa/disable", "identity.user.mfa.disable.post", (*handler.MFAHandler).DisableMFA), []packagecompat.Case{
		{Name: "missing password", Principal: mfa, Body: []byte(`{}`)},
		{Name: "wrong password", Principal: mfa, Body: []byte(`{"password":"wrong"}`)},
		{Name: "correct password", Principal: mfa, Body: []byte(`{"password":"` + password + `"}`)},
		{Name: "no second factor", Principal: member, Body: []byte(`{"password":"` + password + `"}`)},
		{Name: "unknown user", Principal: nobody, Body: []byte(`{"password":"x"}`)},
	}, "/api/v2/user/mfa/disable")
	run(t, mfaRoute("POST", "/api/v2/user/mfa/backup-codes/regenerate", "identity.user.mfa.backup_codes.regenerate.post", (*handler.MFAHandler).RegenerateBackupCodes), []packagecompat.Case{
		{Name: "not enabled", Principal: member},
		{Name: "enabled", Principal: mfa, Mask: []string{"data.backup_codes"}},
	}, "/api/v2/user/mfa/backup-codes/regenerate")
}

func TestAdminMFAConfigParity(t *testing.T) {
	stored := seedUsers(nil, requireMFAConfig(`{"enabled":"1","enforce_for_all":true,"backup_codes_count":"7","methods":{"sms":true,"email":false},"totp_issuer":" Panel "}`))
	run(t, mfaRoute("GET", "/api/v2/admin/mfa/config", "identity.admin.mfa.config.get", (*handler.MFAHandler).GetAdminConfig), []packagecompat.Case{
		{Name: "defaults", Principal: admin},
		{Name: "stored configuration", Principal: admin, Seed: stored},
		{Name: "methods from allowed_methods", Principal: admin, Seed: seedUsers(nil, requireMFAConfig(`{"allowed_methods":"sms"}`))},
	}, "/api/v2/admin/mfa/config")
	run(t, mfaRoute("PUT", "/api/v2/admin/mfa/config", "identity.admin.mfa.config.put", (*handler.MFAHandler).UpdateAdminConfig), []packagecompat.Case{
		{Name: "update", Principal: admin, Body: []byte(`{"enabled":true,"required":"yes","allowed_methods":"totp,sms","totp_issuer":" Ops ","backup_code_count":"12","max_attempts":0}`)},
		{Name: "methods then allowed_methods", Principal: admin, Seed: stored, Body: []byte(`{"methods":{"totp":false},"allowed_methods":["email"]}`)},
		{Name: "not an object", Principal: admin, Body: []byte(`[1]`)},
		{Name: "empty body", Principal: admin},
	}, "/api/v2/admin/mfa/config")
}
