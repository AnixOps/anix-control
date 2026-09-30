package identitycompat

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/gin-gonic/gin"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

const (
	totpSecret = "JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP"
	password   = "correct-horse"
)

// expiredAt is one past instant for both databases of a case.
var expiredAt = time.Now().Add(-time.Hour).Unix()

func loginRoute() packagecompat.Route {
	return packagecompat.Route{
		Method: "POST", Pattern: "/api/v2/login", RouteID: "identity.auth.login", Models: Models,
		Legacy: func(c *gin.Context) { handler.NewAuthHandler(config.Get()).Login(c) },
		Native: nativeRoute("identity.auth.login"),
	}
}

func registerRoute() packagecompat.Route {
	return packagecompat.Route{
		Method: "POST", Pattern: "/api/v2/register", RouteID: "identity.auth.register", Models: Models,
		Legacy: func(c *gin.Context) { handler.NewAuthHandler(config.Get()).Register(c) },
		Native: nativeRoute("identity.auth.register"),
	}
}

// configure installs the configuration both sides read: the legacy handler
// directly, identity through KernelIdentity.GetIdentitySettings.
func configure(change func(*config.Config)) {
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "identity-compat-secret", Expire: 3600}}
	if change != nil {
		change(cfg)
	}
	config.Set(cfg)
	service.ResetLoginRateLimiterForTest()
}

func hash(t testing.TB, plain string) string {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.MinCost)
	require.NoError(t, err)
	return string(hashed)
}

// seedUsers writes the same members into either database.
func seedUsers(change func(*config.Config), mutate func(t testing.TB, db *gorm.DB)) func(t testing.TB, db *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		configure(change)
		past := expiredAt
		users := []model.User{
			{ID: 1, Email: "admin@example.test", Password: hash(t, password), UUID: "u1", Token: "t1", IsAdmin: 1},
			{ID: 2, Email: "member@example.test", Password: hash(t, password), UUID: "u2", Token: "t2"},
			{ID: 3, Email: "banned@example.test", Password: hash(t, password), UUID: "u3", Token: "t3", Banned: 1},
			{ID: 4, Email: "expired@example.test", Password: hash(t, password), UUID: "u4", Token: "t4", ExpiredAt: &past},
			{ID: 5, Email: "mfa@example.test", Password: hash(t, password), UUID: "u5", Token: "t5"},
		}
		require.NoError(t, db.Create(&users).Error)
		if db.Name() == "postgres" {
			// Explicit ids do not advance the sequence new rows draw from.
			require.NoError(t, db.Exec("SELECT setval(pg_get_serial_sequence('v2_user', 'id'), (SELECT MAX(id) FROM v2_user))").Error)
		}
		require.NoError(t, db.Create(&model.UserMFA{UserID: 5, Enabled: true, TOTPSecret: totpSecret, BackupCodes: `["AAAA-BBBB","CCCC-DDDD"]`}).Error)
		if mutate != nil {
			mutate(t, db)
		}
	}
}

func login(email, secret string, extra map[string]string) []byte {
	body := map[string]string{"email": email, "password": secret}
	for key, value := range extra {
		body[key] = value
	}
	encoded, _ := json.Marshal(body)
	return encoded
}

func requireMFAConfig(settings string) func(t testing.TB, db *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		require.NoError(t, db.Create(&model.SystemConfig{Key: "security.mfa.config", Value: settings, Type: "json", Group: "security"}).Error)
	}
}

func TestLoginParity(t *testing.T) {
	route := loginRoute()
	token := []string{"data.token"}
	for _, c := range []packagecompat.Case{
		{Name: "member", Body: login("member@example.test", password, nil), Mask: token},
		{Name: "admin has unrestricted permissions", Body: login("Admin@Example.test", password, nil), Mask: token},
		{Name: "wrong password", Body: login("member@example.test", "wrong", nil)},
		{Name: "unknown email", Body: login("nobody@example.test", password, nil)},
		{Name: "banned", Body: login("banned@example.test", password, nil)},
		{Name: "expired", Body: login("expired@example.test", password, nil)},
		{Name: "missing password", Body: []byte(`{"email":"member@example.test"}`)},
		{Name: "not an email", Body: []byte(`{"email":"member","password":"x"}`)},
		{Name: "empty body"},
		{Name: "mfa challenge", Body: login("mfa@example.test", password, nil)},
		{Name: "mfa backup code", Body: login("mfa@example.test", password, map[string]string{"mfa_code": "CCCC-DDDD", "mfa_method": "backup"}), Mask: token},
		{Name: "mfa backup code without method", Body: login("mfa@example.test", password, map[string]string{"mfa_code": "AAAA-BBBB"}), Mask: token},
		{Name: "mfa wrong code", Body: login("mfa@example.test", password, map[string]string{"mfa_code": "000000"})},
		{Name: "mfa email method is not implemented", Body: login("mfa@example.test", password, map[string]string{"mfa_code": "123456", "mfa_method": "email"})},
		{
			Name: "rate limited after repeated failures", Body: login("member@example.test", password, nil), Headers: []string{"Retry-After"},
			Warmup: [][]byte{
				login("member@example.test", "w1", nil), login("member@example.test", "w2", nil), login("member@example.test", "w3", nil),
				login("member@example.test", "w4", nil), login("member@example.test", "w5", nil), login("member@example.test", "w6", nil),
			},
		},
	} {
		c.Path = "/api/v2/login"
		if c.Seed == nil {
			c.Seed = seedUsers(nil, nil)
		}
		packagecompat.RunRead(t, route, c)
	}
}

func TestLoginMFAPolicyParity(t *testing.T) {
	route := loginRoute()
	required := `{"enabled":true,"required":true}`
	adminOnly := `{"enabled":true,"enforce_for_admin":true}`
	for _, c := range []packagecompat.Case{
		{Name: "enrollment required", Body: login("member@example.test", password, nil), Seed: seedUsers(nil, requireMFAConfig(required))},
		{Name: "enforced for admins only", Body: login("admin@example.test", password, nil), Seed: seedUsers(nil, requireMFAConfig(adminOnly))},
		{Name: "members pass when only admins are enforced", Body: login("member@example.test", password, nil), Seed: seedUsers(nil, requireMFAConfig(adminOnly)), Mask: []string{"data.token"}},
	} {
		c.Path = "/api/v2/login"
		packagecompat.RunRead(t, route, c)
	}
}

func TestLoginWithTOTPParity(t *testing.T) {
	code, err := totp.GenerateCode(totpSecret, time.Now())
	require.NoError(t, err)
	packagecompat.RunRead(t, loginRoute(), packagecompat.Case{
		Name: "totp", Path: "/api/v2/login", Seed: seedUsers(nil, nil), Mask: []string{"data.token"},
		Body: login("mfa@example.test", password, map[string]string{"mfa_code": code, "mfa_method": "totp"}),
	})
}

func register(email, secret, invite string) []byte {
	encoded, _ := json.Marshal(map[string]string{"email": email, "password": secret, "invite_code": invite})
	return encoded
}

// subscribers is the kernel state registration leaves.
func subscribers(t testing.TB, db *gorm.DB) any {
	var users []struct {
		ID           uint
		Email        string
		IsAdmin      int
		InviteUserID *uint
	}
	require.NoError(t, db.Model(&model.User{}).Order("id").Find(&users).Error)
	var invites []struct {
		Code   string
		Status int
		UsedBy *uint
	}
	require.NoError(t, db.Model(&model.InviteCode{}).Order("code").Find(&invites).Error)
	return map[string]any{"users": users, "invites": invites}
}

func invites(t testing.TB, db *gorm.DB) {
	owner := uint(2)
	expired := time.Now().Add(-time.Hour)
	require.NoError(t, db.Create(&[]model.InviteCode{{Code: "OPEN", UserID: &owner}, {Code: "OLD", ExpiredAt: &expired}, {Code: "USED", Status: 1}}).Error)
}

func TestRegisterParity(t *testing.T) {
	route := registerRoute()
	token := []string{"data.token"}
	requireInvite := func(cfg *config.Config) { cfg.Auth.Registration.RequireInvite = true }
	disabled := func(cfg *config.Config) {
		off := false
		cfg.Auth.Registration.Enabled = &off
	}
	blocked := func(cfg *config.Config) { cfg.Auth.Registration.BlockedEmailDomains = []string{"@Spam.test"} }
	allowlist := func(cfg *config.Config) { cfg.Auth.Registration.AllowedEmailDomains = []string{"example.test"} }
	for _, c := range []packagecompat.Case{
		{Name: "new member", Body: register("New@Example.test", "secret1", ""), Mask: token},
		{Name: "with invite code", Body: register("invited@example.test", "secret1", " OPEN "), Mask: token},
		{Name: "used invite code", Body: register("invited@example.test", "secret1", "USED")},
		{Name: "expired invite code", Body: register("invited@example.test", "secret1", "OLD")},
		{Name: "existing email", Body: register("member@example.test", "secret1", "")},
		{Name: "short password", Body: register("new@example.test", "12345", "")},
		{Name: "missing body"},
		{Name: "invite required", Body: register("new@example.test", "secret1", ""), Seed: seedUsers(requireInvite, invites)},
		{Name: "registration disabled", Body: register("new@example.test", "secret1", ""), Seed: seedUsers(disabled, invites)},
		{Name: "blocked domain", Body: register("new@sub.spam.test", "secret1", ""), Seed: seedUsers(blocked, invites)},
		{Name: "outside the allowlist", Body: register("new@other.test", "secret1", ""), Seed: seedUsers(allowlist, invites)},
		{
			Name: "rate limited", Body: register("last@example.test", "secret1", ""), Headers: []string{"Retry-After"},
			Warmup: [][]byte{register("a@example.test", "secret1", ""), register("b@example.test", "secret1", ""),
				register("c@example.test", "secret1", ""), register("d@example.test", "secret1", ""), register("e@example.test", "secret1", "")},
		},
	} {
		c.Path = "/api/v2/register"
		if c.Seed == nil {
			c.Seed = seedUsers(nil, invites)
		}
		c.Snapshot = subscribers
		packagecompat.RunWrite(t, route, c)
	}
}
