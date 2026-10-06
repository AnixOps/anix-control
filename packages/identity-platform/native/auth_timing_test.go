package native

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
)

// neverExpires is a Directory whose subscribers never expire.
type neverExpires struct{}

func (neverExpires) ExpiredAt(context.Context, uint64) (*int64, error) { return nil, nil }

func (f *resetFixture) wrongLogin(t *testing.T, email string) string {
	t.Helper()
	handler, ok := f.service.Handlers()["identity.auth.login"]
	require.True(t, ok)
	body, err := json.Marshal(map[string]string{"email": email, "password": "wrong-password"})
	require.NoError(t, err)
	response, err := handler(context.Background(), pluginhostsdk.NativeRequest{
		RouteID: "identity.auth.login", Method: "POST", Body: body,
		Metadata: pluginhostsdk.RequestMetadata{ClientIP: "203.0.113.9"},
	})
	require.NoError(t, err)
	return string(response.Body)
}

// An unknown e-mail costs the same password work as a wrong password, so the
// time of the answer does not tell which e-mails have an account.
func TestLoginOfAnUnknownEmailDoesTheSamePasswordWork(t *testing.T) {
	f := newResetFixture(t)
	f.service.Directory = neverExpires{}
	require.NoError(t, f.stores.Settings.Put(context.Background(), SettingsKey, []byte(`{"login_rate_limit":{"enabled":false}}`)))
	checks := 0
	original := compareHash
	compareHash = func(hash, password []byte) error {
		checks++
		return original(hash, password)
	}
	t.Cleanup(func() { compareHash = original })

	known := f.wrongLogin(t, "member@example.test")
	knownChecks := checks
	checks = 0
	unknown := f.wrongLogin(t, "nobody@example.test")
	require.Equal(t, 1, knownChecks)
	require.Equal(t, knownChecks, checks, "a bcrypt comparison for an unknown e-mail too")
	require.Equal(t, known, unknown, "and the same answer")
}
