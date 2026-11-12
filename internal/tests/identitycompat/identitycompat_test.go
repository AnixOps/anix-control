// Package identitycompat proves that identity-platform's native routes
// answer exactly as the kernel's legacy handlers: each case runs against
// both on identically seeded databases (internal/tests/packagecompat), on
// SQLite and PostgreSQL. The native side gets its accounts through the real
// import path and reaches Control through the real KernelIdentity and
// KernelSubscriber servers, the latter authorized for exactly the families
// the package declares.
package identitycompat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/AnixOps/anix-control/identity/settings"
	"github.com/AnixOps/anix-control/identity/signingkey"
	"github.com/AnixOps/anix-control/identity/throttle"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/identityimport"
	"github.com/AnixOps/anix-control/v5/internal/kernelidentity"
	"github.com/AnixOps/anix-control/v5/internal/kernelsubscriber"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/AnixOps/anix-control/v5/internal/packagestore"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/AnixOps/anix-control/v5/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v5/packages/identity-platform/native"
	"google.golang.org/grpc"
	"gorm.io/gorm"
)

// Models every identity case needs in both databases.
var Models = []any{
	&model.User{}, &model.UserSS2022Key{}, &model.UserMFA{}, &model.SystemConfig{}, &model.InviteCode{},
	&model.IdentityAccountLink{}, &model.IdentityAuthority{}, &model.IdentityRevocation{}, &model.Plan{}, &model.SubscriberRequest{}, &model.SubscriberChange{},
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

// identityHost is the identity-platform host the kernel serves.
var identityHost = packagebridge.HostIdentity{PackageID: "identity-platform", Version: "4.1.0", Generation: 1}

// declaredSubscriberFamilies authorizes what identity-platform's signed
// release declares of KernelSubscriber: traffic and credential resets.
type declaredSubscriberFamilies struct{}

func (declaredSubscriberFamilies) AuthorizeCapability(_ context.Context, host packagebridge.HostIdentity, capability string) error {
	if host == identityHost && (capability == service.CapabilitySubscriberTraffic || capability == service.CapabilitySubscriberCredentials) {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

// subscriberKernel calls the KernelSubscriber server in process.
type subscriberKernel struct {
	server kernelsubscriberv1.KernelSubscriberServer
}

func (k subscriberKernel) ResetTraffic(ctx context.Context, in *kernelsubscriberv1.ResetTrafficRequest, _ ...grpc.CallOption) (*kernelsubscriberv1.ResetTrafficResponse, error) {
	return k.server.ResetTraffic(ctx, in)
}

func (k subscriberKernel) ResetCredentials(ctx context.Context, in *kernelsubscriberv1.ResetCredentialsRequest, _ ...grpc.CallOption) (*kernelsubscriberv1.ResetCredentialsResponse, error) {
	return k.server.ResetCredentials(ctx, in)
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

// importAccounts copies the seeded v2 rows, their MFA and backup codes into
// identity and links them, as the 5.0.3 cutover did.
func importAccounts(db *gorm.DB, store *account.Store) {
	var users []model.User
	must(db.Order("id").Find(&users).Error)
	var mfaRows []model.UserMFA
	must(db.Find(&mfaRows).Error)
	mfas := map[uint]model.UserMFA{}
	for _, mfa := range mfaRows {
		mfas[mfa.UserID] = mfa
	}
	var batch account.ImportBatch
	for _, user := range users {
		accountUUID := identityimport.AccountUUID(user.ID)
		must(db.Create(&model.IdentityAccountLink{UserID: user.ID, AccountUUID: accountUUID}).Error)
		imported := account.Imported{Account: account.Account{
			UserID: uint64(user.ID), AccountUUID: accountUUID, Email: user.Email, PasswordHash: user.Password,
			IsAdmin: user.IsAdmin == 1, IsStaff: user.IsStaff == 1, Banned: user.Banned == 1, CreatedAt: user.CreatedAt,
		}}
		if user.PasswordAlgo != nil {
			imported.Account.PasswordAlgo = *user.PasswordAlgo
		}
		if user.PasswordSalt != nil {
			imported.Account.PasswordSalt = *user.PasswordSalt
		}
		if user.InviteUserID != nil {
			imported.Account.InviteUserID = uint64(*user.InviteUserID)
		}
		if mfa, ok := mfas[user.ID]; ok {
			var codes []string
			if mfa.BackupCodes != "" {
				must(json.Unmarshal([]byte(mfa.BackupCodes), &codes))
			}
			hashes := make([]string, 0, len(codes))
			for _, code := range codes {
				sum := sha256.Sum256([]byte(code))
				hashes = append(hashes, hex.EncodeToString(sum[:]))
			}
			imported.MFA = &account.MFA{Enabled: mfa.Enabled, TOTPSecret: mfa.TOTPSecret, BackupCodeHashes: hashes, LastMethod: mfa.LastMethod}
			if mfa.LastUsed != nil {
				imported.MFA.LastUsed = *mfa.LastUsed
			}
			if mfa.Enabled {
				imported.MFA.EnabledAt = mfa.UpdatedAt
			}
		}
		batch.Accounts = append(batch.Accounts, imported)
	}
	_, err := store.Import(context.Background(), "compat-seed", "users", batch)
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
			Open:       func(context.Context) (*native.Stores, error) { return stores, nil },
			Kernel:     kernel{server: (&kernelidentity.Server{DB: db, Authorizer: allowAll{}, Config: config.Get}).For(identityHost)},
			Subscriber: subscriberKernel{server: (&kernelsubscriber.Server{DB: db, Authorizer: declaredSubscriberFamilies{}}).For(identityHost)},
			Directory:  native.ViewDirectory{DB: func(context.Context) (*gorm.DB, error) { return db, nil }},
			SigningKey: func(context.Context) (signingkey.Key, error) {
				return key, nil
			},
		}
		return service
	}
}

// clockTexts are the seeded instants that move with the clock (expiredAt,
// later, earlyToday): a recorded answer holds 0 in their place.
func clockTexts() []string {
	texts := []string{strconv.FormatInt(expiredAt, 10), strconv.FormatInt(later, 10), strconv.FormatInt(earlyToday.Unix(), 10)}
	for _, at := range []time.Time{time.Unix(expiredAt, 0), time.Unix(later, 0), earlyToday} {
		texts = append(texts, at.UTC().Format(time.RFC3339), at.Format(time.RFC3339))
	}
	return texts
}

func readCase(t *testing.T, route packagecompat.Route, c packagecompat.Case) {
	t.Helper()
	c.Volatile = append(c.Volatile, clockTexts()...)
	packagecompat.RunRead(t, route, c)
}

func writeCase(t *testing.T, route packagecompat.Route, c packagecompat.Case) {
	t.Helper()
	c.Volatile = append(c.Volatile, clockTexts()...)
	packagecompat.RunWrite(t, route, c)
}
