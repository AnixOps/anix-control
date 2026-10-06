package native

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

const loginRouteID = "identity.auth.login"

// noExpiry is a Directory whose subscribers never expire.
type noExpiry struct{}

func (noExpiry) ExpiredAt(context.Context, uint64) (*int64, error) { return nil, nil }

func (f *resetFixture) limitLogins(t *testing.T, maxAttempts int) {
	t.Helper()
	document, err := json.Marshal(map[string]any{
		"login_rate_limit": map[string]any{"enabled": true, "max_attempts": maxAttempts, "window_seconds": 300, "lockout_seconds": 600},
	})
	require.NoError(t, err)
	require.NoError(t, f.stores.Settings.Put(context.Background(), SettingsKey, document))
}

func (f *resetFixture) login(t *testing.T, email, password string) string {
	t.Helper()
	handler, ok := f.service.Handlers()[loginRouteID]
	require.True(t, ok)
	body, err := json.Marshal(map[string]string{"email": email, "password": password})
	require.NoError(t, err)
	response, err := handler(context.Background(), pluginhostsdk.NativeRequest{
		RouteID: loginRouteID, Method: "POST", Body: body,
		Metadata: pluginhostsdk.RequestMetadata{ClientIP: "203.0.113.9"},
	})
	require.NoError(t, err)
	return string(response.Body)
}

// Parallel guesses at one account are counted before they run: the limit used
// to be checked first and recorded after the password check, so a burst all
// passed the check and every guess was tried.
func TestLoginBurstOfWrongPasswordsStopsAtTheLimit(t *testing.T) {
	f := newResetFixture(t)
	f.service.Directory = noExpiry{}
	f.limitLogins(t, 3)
	// A password check that takes real time, as production's bcrypt does: the
	// burst overlaps in it.
	slow, err := bcrypt.GenerateFromPassword([]byte(resetPassword), 10)
	require.NoError(t, err)
	require.NoError(t, f.stores.Accounts.DB.Exec("UPDATE idp_account SET password_hash = ? WHERE user_id = 2", string(slow)).Error)

	const burst = 20
	answers := make([]string, burst)
	var wait sync.WaitGroup
	start := make(chan struct{})
	for i := range burst {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			answers[i] = f.login(t, "member@example.test", "wrong-password")
		}()
	}
	close(start)
	wait.Wait()

	limited, tried := 0, 0
	for _, answer := range answers {
		switch {
		case strings.Contains(answer, "too many login attempts"):
			limited++
		case strings.Contains(answer, "用户不存在或密码错误") || strings.Contains(answer, "\\u7528\\u6237"):
			tried++
		default:
			t.Fatalf("unexpected answer %s", answer)
		}
	}
	require.Equal(t, 3, tried, "only the limit's worth of passwords is checked")
	require.Equal(t, burst-3, limited)
}

// A correct password that waits for its second factor is not a failed attempt:
// the prompt gives the attempt back.
func TestLoginSecondFactorPromptDoesNotUseUpAttempts(t *testing.T) {
	f := newResetFixture(t)
	f.service.Directory = noExpiry{}
	f.limitLogins(t, 3)
	for range 10 {
		answer := f.login(t, "mfa@example.test", resetPassword)
		require.Contains(t, answer, "mfa_required", "%s", answer)
	}
}

func (f *resetFixture) configure(t *testing.T, document map[string]any) {
	t.Helper()
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	require.NoError(t, f.stores.Settings.Put(context.Background(), SettingsKey, encoded))
}

func (f *resetFixture) call(t *testing.T, routeID string, userID uint, ip, body string) string {
	t.Helper()
	handler, ok := f.service.Handlers()[routeID]
	require.True(t, ok)
	response, err := handler(context.Background(), pluginhostsdk.NativeRequest{
		RouteID: routeID, Method: "POST", Body: []byte(body), Principal: pluginhostsdk.Principal{ActorID: userID},
		Metadata: pluginhostsdk.RequestMetadata{ClientIP: ip},
	})
	require.NoError(t, err)
	return string(response.Body)
}

// The second-factor guesses of an account are limited by the admin MFA
// configuration whichever address they come from: the login key is per
// address, so guesses spread over addresses were not limited at all.
func TestLoginSecondFactorGuessesAreLimitedPerAccount(t *testing.T) {
	f := newResetFixture(t)
	f.service.Directory = noExpiry{}
	f.configure(t, map[string]any{
		"login_rate_limit": map[string]any{"enabled": true, "max_attempts": 100, "window_seconds": 300, "lockout_seconds": 600},
		"admin_mfa":        map[string]any{"enabled": true, "max_attempts": 3, "lockout_duration": 15},
	})
	guess := func(ip, code string) string {
		body, err := json.Marshal(map[string]string{"email": "mfa@example.test", "password": resetPassword, "mfa_code": code})
		require.NoError(t, err)
		return f.call(t, loginRouteID, 0, ip, string(body))
	}
	for i := range 3 {
		answer := guess(fmt.Sprintf("203.0.113.%d", i+1), "000000")
		require.Contains(t, answer, "invalid mfa code", "%s", answer)
	}
	// A fourth address, and the right code: the account is locked.
	right, err := totp.GenerateCode(f.secret, f.now)
	require.NoError(t, err)
	answer := guess("198.51.100.4", right)
	require.Contains(t, answer, "too many mfa attempts", "%s", answer)
	require.NotContains(t, answer, `"token"`)
}

// The password re-check of the sign-in's own session is not a guessing oracle
// either: it counts per account, like the login does per address.
func TestDisableMFAPasswordGuessesAreLimitedPerAccount(t *testing.T) {
	f := newResetFixture(t)
	f.configure(t, map[string]any{
		"login_rate_limit": map[string]any{"enabled": true, "max_attempts": 3, "window_seconds": 300, "lockout_seconds": 600},
	})
	const route = "identity.user.mfa.disable.post"
	for range 3 {
		require.Contains(t, f.call(t, route, 5, "", `{"password":"wrong"}`), "invalid password")
	}
	answer := f.call(t, route, 5, "", `{"password":"`+resetPassword+`"}`)
	require.Contains(t, answer, "too many attempts", "the right password is not tried while locked: %s", answer)
	st, err := f.stores.Accounts.Status(context.Background(), 5)
	require.NoError(t, err)
	require.True(t, st.Enabled, "MFA stays enabled")
}
