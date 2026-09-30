// Package identitycompat proves that identity-platform's native group A
// routes answer exactly as the kernel's legacy handlers: each case runs
// against both on identically seeded databases (internal/tests/packagecompat).
// The native side gets its accounts through the real import path and reaches
// Control through the real KernelIdentity server.
package identitycompat

import (
	"context"
	"net"
	"os"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/AnixOps/anix-control/identity/server"
	"github.com/AnixOps/anix-control/identity/settings"
	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/AnixOps/anix-control/identity/throttle"
	identityv1 "github.com/AnixOps/anix-control/sdk/api/identity/v1"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/identityimport"
	"github.com/AnixOps/anix-control/v4/internal/kernelidentity"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"github.com/AnixOps/anix-control/v4/packages/identity-platform/native"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"gorm.io/gorm"
)

// Models every identity case needs in both databases.
var Models = []any{
	&model.User{}, &model.UserMFA{}, &model.MFALoginAttempt{}, &model.SystemConfig{}, &model.InviteCode{},
	&model.IdentityAccountLink{}, &model.IdentityAuthority{}, &model.IdentityRevocation{}, &model.Plan{},
}

const tablePrefix = "idp_"

type allowAll struct{}

func (allowAll) AuthorizeIdentity(context.Context, packagebridge.HostIdentity) error { return nil }

// kernel calls the KernelIdentity server in process.
type kernel struct {
	server kernelidentityv1.KernelIdentityServer
}

func (k kernel) CreateSubscriber(ctx context.Context, in *kernelidentityv1.CreateSubscriberRequest, _ ...grpc.CallOption) (*kernelidentityv1.CreateSubscriberResponse, error) {
	return k.server.CreateSubscriber(ctx, in)
}

func (k kernel) ResolveActorAccess(ctx context.Context, in *kernelidentityv1.ResolveActorAccessRequest, _ ...grpc.CallOption) (*kernelidentityv1.ResolveActorAccessResponse, error) {
	return k.server.ResolveActorAccess(ctx, in)
}

func (k kernel) UpdateSubscriber(ctx context.Context, in *kernelidentityv1.UpdateSubscriberRequest, _ ...grpc.CallOption) (*kernelidentityv1.UpdateSubscriberResponse, error) {
	return k.server.UpdateSubscriber(ctx, in)
}

func (k kernel) ApplyAccountProjection(ctx context.Context, in *kernelidentityv1.ApplyAccountProjectionRequest, _ ...grpc.CallOption) (*kernelidentityv1.ApplyAccountProjectionResponse, error) {
	return k.server.ApplyAccountProjection(ctx, in)
}

func (k kernel) DeleteSubscriber(ctx context.Context, in *kernelidentityv1.DeleteSubscriberRequest, _ ...grpc.CallOption) (*kernelidentityv1.DeleteSubscriberResponse, error) {
	return k.server.DeleteSubscriber(ctx, in)
}

func (k kernel) GetSubscriber(ctx context.Context, in *kernelidentityv1.GetSubscriberRequest, _ ...grpc.CallOption) (*kernelidentityv1.GetSubscriberResponse, error) {
	return k.server.GetSubscriber(ctx, in)
}

func (k kernel) GetIdentitySettings(ctx context.Context, in *kernelidentityv1.GetIdentitySettingsRequest, _ ...grpc.CallOption) (*kernelidentityv1.GetIdentitySettingsResponse, error) {
	return k.server.GetIdentitySettings(ctx, in)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

// applyMigration runs one of identity-platform's migration scripts.
func applyMigration(db *gorm.DB, name string) {
	script, err := os.ReadFile("../../../packages/identity-platform/migrations/" + name)
	must(err)
	prefixed := &packagestoresdk.Store{Lease: packagebridgesdk.StorageLease{TablePrefix: tablePrefix}}
	for _, statement := range strings.Split(prefixed.ExpandScript(string(script)), ";") {
		if strings.TrimSpace(statement) != "" {
			must(db.Exec(statement).Error)
		}
	}
}

// importAccounts copies the seeded v2 rows into identity with the kernel's
// importer, as the cutover does.
func importAccounts(db *gorm.DB, store *account.Store) {
	listener := bufconn.Listen(1 << 20)
	grpcServer := grpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, &server.Server{Accounts: store})
	go func() { _ = grpcServer.Serve(listener) }()
	defer grpcServer.Stop()
	conn, err := grpc.NewClient("passthrough:///identity", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	must(err)
	defer func() { _ = conn.Close() }()
	importer := &identityimport.Importer{DB: db, Connect: func() (grpc.ClientConnInterface, error) { return conn, nil }}
	_, err = importer.Run(context.Background(), false)
	must(err)
}

// nativeRoute builds a native route on the native database.
func nativeRoute(routeID string) func(db *gorm.DB) pluginhostsdk.NativeHandler {
	return func(db *gorm.DB) pluginhostsdk.NativeHandler {
		return nativeService(db).Handlers()[routeID]
	}
}

// nativeService builds identity on db: its tables, the imported accounts, a
// signing key and Control through the real KernelIdentity server.
func nativeService(db *gorm.DB) *native.Service {
	{
		for _, migration := range []string{"003_accounts.sql", "004_login.sql"} {
			applyMigration(db, migration)
		}
		must(packagestore.EnsureKernelAPIViews(db))
		box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
		must(err)
		accounts := &account.Store{DB: db, Secrets: box, Tables: account.Tables{
			Account: tablePrefix + "account", MFA: tablePrefix + "mfa", ImportRun: tablePrefix + "import_run", MFAAttempt: tablePrefix + "mfa_attempt",
		}}
		importAccounts(db, accounts)
		key, err := signingkey.Generate(time.Now())
		must(err)
		key.State, key.ActivatedAt = signingkey.StateActive, time.Now()
		stores := &native.Stores{
			Accounts: accounts,
			Throttle: &throttle.Limiter{DB: db, Table: tablePrefix + "throttle"},
			Settings: &settings.Store{DB: db, Table: tablePrefix + "setting"},
		}
		service := &native.Service{
			Open:      func(context.Context) (*native.Stores, error) { return stores, nil },
			Kernel:    kernel{server: (&kernelidentity.Server{DB: db, Authorizer: allowAll{}, Config: config.Get}).For(packagebridge.HostIdentity{PackageID: "identity-platform", Version: "4.1.0", Generation: 1})},
			Directory: native.ViewDirectory{DB: func(context.Context) (*gorm.DB, error) { return db, nil }},
			SigningKey: func(context.Context) (signingkey.Key, error) {
				return key, nil
			},
		}
		return service
	}
}
