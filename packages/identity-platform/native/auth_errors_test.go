package native

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The unauthenticated login and registration do not hand the text of an
// infrastructure failure (here a missing table of the identity database) to
// the client; a refusal meant for the caller still reads as it did.
func TestLoginAndRegisterHideInfrastructureErrors(t *testing.T) {
	f := newResetFixture(t)
	f.service.Directory = noExpiry{}
	f.limitLogins(t, 100)

	refused := f.login(t, "member@example.test", "wrong-password")
	require.Contains(t, refused, "用户不存在或密码错误", "a refusal is read as before: %s", refused)

	require.NoError(t, f.stores.Accounts.DB.Exec("DROP TABLE idp_throttle").Error)
	answer := f.login(t, "member@example.test", "wrong-password")
	require.Contains(t, answer, internalFailure)
	for _, leaked := range []string{"idp_throttle", "no such table", "SQL"} {
		require.False(t, strings.Contains(answer, leaked), "the answer leaks %q: %s", leaked, answer)
	}
}
