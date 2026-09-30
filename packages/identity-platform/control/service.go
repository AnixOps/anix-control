package main

import (
	"context"
	"log"

	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/AnixOps/anix-control/identity/server"
	"github.com/AnixOps/anix-control/identity/signingkey"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	identityplatform "github.com/AnixOps/anix-control/v4/packages/identity-platform"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

const identityMigrationRoute = "migration.identity-platform.001_identity_platform"

var identityRoutes = map[string]struct{}{
	"identity.admin.mfa.config.get":                  {},
	"identity.admin.mfa.config.put":                  {},
	"identity.admin.users.get":                       {},
	"identity.admin.users.id.ban.post":               {},
	"identity.admin.users.id.delete":                 {},
	"identity.admin.users.id.get":                    {},
	"identity.admin.users.id.put":                    {},
	"identity.admin.users.id.reset_subscribe.post":   {},
	"identity.admin.users.id.reset_traffic.post":     {},
	"identity.admin.users.id.unban.post":             {},
	"identity.admin.users.post":                      {},
	"identity.admin.users.stats.get":                 {},
	"identity.auth.login":                            {},
	"identity.auth.register":                         {},
	"identity.user.dashboard.get":                    {},
	"identity.user.invite.generate.post":             {},
	"identity.user.invite.get":                       {},
	"identity.user.mfa.backup_codes.regenerate.post": {},
	"identity.user.mfa.disable.post":                 {},
	"identity.user.mfa.status.get":                   {},
	"identity.user.mfa.totp.enable.post":             {},
	"identity.user.mfa.totp.setup.post":              {},
	"identity.user.mfa.verify.post":                  {},
	"identity.user.profile.get":                      {},
}

// identityBridge is what the identity host needs from the package bridge.
type identityBridge interface {
	pluginhostsdk.RouterBridge
	packagestoresdk.Leaser
}

// newIdentityService returns the identity host's router: only the declared
// identity routes and the 001_identity_platform migration are accepted, and
// every route passes through the bridge. The package's own storage runs the
// embedded migration index when the kernel starts the host.
func newIdentityService(bridge identityBridge, leaseID string, storage packagestoresdk.Opener, handlers ...map[string]pluginhostsdk.NativeHandler) (*pluginhostsdk.Router, error) {
	var nativeHandlers map[string]pluginhostsdk.NativeHandler
	if len(handlers) > 0 {
		nativeHandlers = handlers[0]
	}
	return pluginhostsdk.NewRouter(pluginhostsdk.RouterConfig{
		PackageID: "identity-platform", LeaseID: leaseID, Bridge: bridge, Logf: log.Printf,
		AllowRoute: func(routeID string) bool {
			_, allowed := identityRoutes[routeID]
			return allowed
		},
		MigrationOperation: func(migrationID string) (string, bool) {
			return identityMigrationRoute, migrationID == "001_identity_platform"
		},
		IndexMigration: packagestoresdk.IndexMigrator(storage, identityplatform.Migrations, identityplatform.MigrationIndex),
		// Native routes serve only once the kernel sets their mode (the
		// identity cutover); until then every route stays legacy.
		Native: nativeHandlers,
	})
}

// newIdentityHost builds the identity-platform package. Without a KEK the
// host serves the routes and IdentityService, but publishes no keys.
func newIdentityHost(bridge identityBridge, leaseID string, getenv func(string) string, logf func(string, ...any)) (*identityHost, error) {
	storage := packagestoresdk.SharedOpener(bridge)
	policy, err := keyPolicy(getenv)
	if err != nil {
		return nil, err
	}
	host := &identityHost{
		identity: &server.Server{Policy: policy, Issuer: tokenIssuer, Audience: tokenAudience},
		logf:     logf,
	}
	kek, err := loadKEK(getenv)
	if err != nil {
		return nil, err
	}
	if kek == nil {
		logf("identity signing keys: no %s; identity publishes no keys and signs no tokens", envKEK)
		if host.Router, err = newIdentityService(bridge, leaseID, storage); err != nil {
			return nil, err
		}
		return host, nil
	}
	sealer, err := signingkey.NewSealer(kek)
	if err != nil {
		return nil, err
	}
	host.keys = &storedKeys{open: storage, sealer: sealer, policy: policy}
	host.identity.Keys = host.keys
	box, err := secretbox.New(kek)
	if err != nil {
		return nil, err
	}
	accounts := &storedAccounts{open: storage, box: box}
	host.identity.Accounts = accounts
	service := &native.Service{
		Open:       nativeStores(storage, accounts),
		Directory:  native.ViewDirectory{DB: func(ctx context.Context) (*gorm.DB, error) { return openDB(ctx, storage) }},
		SigningKey: host.keys.signingKey,
	}
	if conn, ok := bridge.(interface {
		Conn() grpc.ClientConnInterface
	}); ok && conn.Conn() != nil {
		service.Kernel = kernelidentityv1.NewKernelIdentityClient(conn.Conn())
	}
	handlers := map[string]pluginhostsdk.NativeHandler(nil)
	if service.Kernel != nil {
		handlers = service.Handlers()
	}
	if host.Router, err = newIdentityService(bridge, leaseID, storage, handlers); err != nil {
		return nil, err
	}
	return host, nil
}
