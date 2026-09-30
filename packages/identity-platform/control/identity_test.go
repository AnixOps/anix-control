package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// storageBridge leases a SQLite file, as the kernel does for a local host on
// SQLite.
type storageBridge struct {
	bridgeStub
	path string
}

func (b *storageBridge) LeaseStorage(context.Context) (packagebridgesdk.StorageLease, error) {
	return packagebridgesdk.StorageLease{Driver: "sqlite", DSN: b.path, TablePrefix: "pkg_identity_platform_", LeaseGeneration: 1}, nil
}

func environment(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

const testKEK = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestIdentityHostMigratesStorageAndPublishesItsKeys(t *testing.T) {
	bridge := &storageBridge{path: filepath.Join(t.TempDir(), "kernel.db")}
	host, err := newIdentityHost(bridge, "lease-1", environment(map[string]string{envKEK: testKEK}), t.Logf)
	require.NoError(t, err)
	ctx := context.Background()

	migration, err := host.Migrate(ctx, pluginhostsdk.MigrationRequest{MigrationID: "index.0123456789abcdef0123456789abcdef"})
	require.NoError(t, err)
	require.True(t, migration.Complete)
	require.NotEmpty(t, migration.ValidationDigest)

	runCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		host.keys.maintain(runCtx, time.Hour, t.Logf)
		close(done)
	}()
	var response *identityv1.GetTokenKeysResponse
	require.Eventually(t, func() bool {
		response, err = host.identity.GetTokenKeys(ctx, &identityv1.GetTokenKeysRequest{})
		return err == nil && len(response.GetKeys()) == 1
	}, 5*time.Second, 20*time.Millisecond)
	stop()
	<-done
	require.Equal(t, tokenIssuer, response.GetIssuer())
	require.Equal(t, tokenAudience, response.GetAudience())
	require.Equal(t, identityv1.TokenKeyState_TOKEN_KEY_STATE_ACTIVE, response.GetKeys()[0].GetState())

	// Another replica with the same KEK and storage serves the same keys.
	replica, err := newIdentityHost(bridge, "lease-2", environment(map[string]string{envKEK: testKEK}), t.Logf)
	require.NoError(t, err)
	again, err := replica.identity.GetTokenKeys(ctx, &identityv1.GetTokenKeysRequest{})
	require.NoError(t, err)
	require.Equal(t, response.GetKeys()[0].GetKid(), again.GetKeys()[0].GetKid())
}

func TestIdentityHostWithoutKEKPublishesNothing(t *testing.T) {
	host, err := newIdentityHost(&bridgeStub{}, "lease-1", environment(nil), t.Logf)
	require.NoError(t, err)
	require.Nil(t, host.keys)
	_, err = host.identity.GetTokenKeys(context.Background(), &identityv1.GetTokenKeysRequest{})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))

	_, err = newIdentityHost(&bridgeStub{}, "lease-1", environment(map[string]string{envKEK: "short"}), t.Logf)
	require.Error(t, err, "a malformed KEK stops the host")
	_, err = newIdentityHost(&bridgeStub{}, "lease-1", environment(map[string]string{envTokenLifetime: "-1h"}), t.Logf)
	require.Error(t, err)
}
