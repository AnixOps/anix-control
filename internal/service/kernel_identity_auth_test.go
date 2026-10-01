package service

import (
	"context"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
)

func TestAuthorizeIdentityNeedsTheSignedCapabilityAndTheCurrentGeneration(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, _ := seedKnowledgeRelease(t, db, "", []string{CapabilityIdentity})
	operations := PackageHostOperations{DB: db, FallbackPublicKey: publicKey}
	host := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}

	require.NoError(t, operations.AuthorizeIdentity(context.Background(), host))

	stale := host
	stale.Generation = 6
	require.ErrorIs(t, operations.AuthorizeIdentity(context.Background(), stale), packagebridge.ErrHostFenced)

	require.NoError(t, db.Model(&model.Plugin{}).Where("id = ?", "knowledge").Update("official", false).Error)
	require.ErrorIs(t, operations.AuthorizeIdentity(context.Background(), host), ErrIdentityNotAuthorized,
		"only an official package may hold kernel identity")
}

func TestAuthorizeIdentityRefusesPackagesWithoutTheCapability(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, _ := seedKnowledgeRelease(t, db, "", []string{CapabilityStorage})
	operations := PackageHostOperations{DB: db, FallbackPublicKey: publicKey}
	host := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}

	require.ErrorIs(t, operations.AuthorizeIdentity(context.Background(), host), ErrIdentityNotAuthorized)
}

// Subscriber method families are authorized one capability at a time, on the
// same terms as KernelIdentity.
func TestAuthorizeCapabilityGrantsOnlyTheSignedSubscriberFamilies(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, _ := seedKnowledgeRelease(t, db, "", []string{CapabilitySubscriberTraffic, CapabilitySubscriberDirectory})
	operations := PackageHostOperations{DB: db, FallbackPublicKey: publicKey}
	host := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}
	ctx := context.Background()

	require.NoError(t, operations.AuthorizeCapability(ctx, host, CapabilitySubscriberTraffic))
	require.NoError(t, operations.AuthorizeCapability(ctx, host, CapabilitySubscriberDirectory))
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, host, CapabilitySubscriberEntitlements), ErrCapabilityNotAuthorized)
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, host, CapabilitySubscriberGroups), ErrCapabilityNotAuthorized)
	require.ErrorIs(t, operations.AuthorizeIdentity(ctx, host), ErrIdentityNotAuthorized)
	stale := host
	stale.Generation = 6
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, stale, CapabilitySubscriberTraffic), packagebridge.ErrHostFenced)

	require.Error(t, validateManifestCapabilities([]string{"kernel.subscriber.everything.v1"}), "unknown families are refused")
	require.NoError(t, validateManifestCapabilities([]string{CapabilitySubscriberGroups}), "the membership family is in the grammar")
}

// KernelOrder's completion is authorized as the subscriber families are: on
// every call, for an official package whose signed release declares it.
func TestAuthorizeCapabilityGrantsOrderCompletionToItsHoldersOnly(t *testing.T) {
	db := newKernelTestDB(t)
	publicKey, _ := seedKnowledgeRelease(t, db, "", []string{CapabilityOrderComplete})
	operations := PackageHostOperations{DB: db, FallbackPublicKey: publicKey}
	host := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}
	ctx := context.Background()

	require.NoError(t, operations.AuthorizeCapability(ctx, host, CapabilityOrderComplete))
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, host, CapabilitySubscriberEntitlements), ErrCapabilityNotAuthorized)
	stale := host
	stale.Generation = 6
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, stale, CapabilityOrderComplete), packagebridge.ErrHostFenced)
	require.NoError(t, db.Model(&model.Plugin{}).Where("id = ?", "knowledge").Update("official", false).Error)
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, host, CapabilityOrderComplete), ErrCapabilityNotAuthorized,
		"only an official package may complete orders")

	require.NoError(t, validateManifestCapabilities([]string{CapabilityOrderComplete}))
	require.Error(t, validateManifestCapabilities([]string{"kernel.order.everything.v1"}), "unknown order capabilities are refused")
}
