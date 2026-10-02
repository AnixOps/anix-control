package native

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/account"
	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/AnixOps/anix-control/identity/settings"
	"github.com/AnixOps/anix-control/identity/throttle"
	kernelidentityv1 "github.com/AnixOps/anix-control/sdk/api/kernelidentity/v1"
	kernelsubscriberv1 "github.com/AnixOps/anix-control/sdk/api/kernelsubscriber/v1"
	"github.com/AnixOps/anix-control/sdk/packagebridgesdk"
	"github.com/AnixOps/anix-control/sdk/packagestoresdk"
	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/glebarez/sqlite"
	"github.com/pquerna/otp/totp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const resetPassword = "correct-horse"

// resetKernel is Control for a reset: KernelIdentity.GetSubscriber and
// KernelSubscriber.ResetCredentials for the subscribers it knows.
type resetKernel struct {
	Kernel
	tokens map[uint64]string
	resets []*kernelsubscriberv1.ResetCredentialsRequest
	fail   error
}

func (k *resetKernel) GetSubscriber(_ context.Context, in *kernelidentityv1.GetSubscriberRequest, _ ...grpc.CallOption) (*kernelidentityv1.GetSubscriberResponse, error) {
	token, ok := k.tokens[in.GetUserId()]
	if !ok {
		return nil, status.Error(codes.NotFound, "subscriber is not linked to an identity account")
	}
	encoded, _ := json.Marshal(map[string]any{"id": in.GetUserId(), "token": token, "uuid": "kept"})
	return &kernelidentityv1.GetSubscriberResponse{SubscriberJson: encoded}, nil
}

func (k *resetKernel) ResetTraffic(context.Context, *kernelsubscriberv1.ResetTrafficRequest, ...grpc.CallOption) (*kernelsubscriberv1.ResetTrafficResponse, error) {
	return nil, errors.New("not expected")
}

func (k *resetKernel) ResetCredentials(_ context.Context, in *kernelsubscriberv1.ResetCredentialsRequest, _ ...grpc.CallOption) (*kernelsubscriberv1.ResetCredentialsResponse, error) {
	k.resets = append(k.resets, in)
	if k.fail != nil {
		return nil, k.fail
	}
	if _, ok := k.tokens[in.GetUserId()]; !ok {
		return nil, status.Error(codes.NotFound, "subscriber not found")
	}
	for _, seen := range k.resets[:len(k.resets)-1] {
		if seen.GetRequestId() == in.GetRequestId() {
			return &kernelsubscriberv1.ResetCredentialsResponse{}, nil
		}
	}
	k.tokens[in.GetUserId()] = "issued-" + in.GetRequestId()[len(in.GetRequestId())-4:]
	return &kernelsubscriberv1.ResetCredentialsResponse{Applied: true}, nil
}

type resetFixture struct {
	service *Service
	kernel  *resetKernel
	stores  *Stores
	now     time.Time
	secret  string
	backups []string
}

// newResetFixture is identity with two accounts on SQLite: 2 without a
// second factor, 5 with TOTP and recovery codes.
func newResetFixture(t *testing.T) *resetFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	prefixed := &packagestoresdk.Store{Lease: packagebridgesdk.StorageLease{TablePrefix: "idp_"}}
	for _, name := range []string{"003_accounts.sql", "004_login.sql"} {
		script, err := os.ReadFile("../migrations/" + name)
		require.NoError(t, err)
		for _, statement := range strings.Split(prefixed.ExpandScript(string(script)), ";") {
			if strings.TrimSpace(statement) != "" {
				require.NoError(t, db.Exec(statement).Error)
			}
		}
	}
	box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	f := &resetFixture{now: time.Unix(1_790_000_000, 0)}
	clock := func() time.Time { return f.now }
	accounts := &account.Store{DB: db, Secrets: box, Now: clock, Tables: account.Tables{
		Account: "idp_account", MFA: "idp_mfa", ImportRun: "idp_import_run", MFAAttempt: "idp_mfa_attempt",
	}}
	hash, err := bcrypt.GenerateFromPassword([]byte(resetPassword), bcrypt.MinCost)
	require.NoError(t, err)
	ctx := context.Background()
	for _, id := range []uint64{2, 5} {
		require.NoError(t, accounts.Create(ctx, account.Account{
			UserID: id, AccountUUID: fmt.Sprintf("00000000-0000-4000-8000-%012d", id), Email: map[uint64]string{2: "member@example.test", 5: "mfa@example.test"}[id],
			PasswordHash: string(hash), PasswordAlgo: "bcrypt", TokenVersion: 1, Version: 1,
		}))
	}
	setup, err := accounts.SetupTOTP(ctx, 5, "AnixOps Control", "mfa@example.test", 2)
	require.NoError(t, err)
	code, err := totp.GenerateCode(setup.Secret, f.now)
	require.NoError(t, err)
	require.NoError(t, accounts.EnableTOTP(ctx, 5, code))
	f.secret, f.backups = setup.Secret, setup.BackupCodes
	f.stores = &Stores{
		Accounts: accounts,
		Throttle: &throttle.Limiter{DB: db, Table: "idp_throttle", Now: clock},
		Settings: &settings.Store{DB: db, Table: "idp_setting"},
	}
	f.kernel = &resetKernel{tokens: map[uint64]string{2: "t2", 5: "t5"}}
	counter := 0
	f.service = &Service{
		Open:       func(context.Context) (*Stores, error) { return f.stores, nil },
		Kernel:     f.kernel,
		Subscriber: f.kernel,
		Now:        clock,
		NewToken: func() string {
			counter++
			return "request-" + string(rune('a'+counter))
		},
	}
	return f
}

// reset sends the request as the kernel forwards it and returns the panel
// envelope and the Retry-After header.
func (f *resetFixture) reset(t *testing.T, userID uint, body string, headers map[string][]string) (map[string]any, string) {
	t.Helper()
	handler, ok := f.service.Handlers()[UserSubscriptionResetRouteID]
	require.True(t, ok)
	response, err := handler(context.Background(), pluginhostsdk.NativeRequest{
		RouteID: UserSubscriptionResetRouteID, Method: "POST", Body: []byte(body),
		Principal: pluginhostsdk.Principal{ActorID: userID},
		Metadata:  pluginhostsdk.RequestMetadata{Headers: headers},
	})
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(response.Body, &decoded), "%s", response.Body)
	retry := ""
	for _, header := range response.Headers {
		if header.Name == "Retry-After" {
			retry = header.Value
		}
	}
	return decoded, retry
}

func requireRefused(t *testing.T, answer map[string]any, message string) {
	t.Helper()
	require.NotEqual(t, float64(0), answer["code"], "%v", answer)
	require.Equal(t, message, answer["msg"], "%v", answer)
}

func requireIssued(t *testing.T, answer map[string]any) string {
	t.Helper()
	require.Equal(t, float64(0), answer["code"], "%v", answer)
	data, ok := answer["data"].(map[string]any)
	require.True(t, ok, "%v", answer)
	token, _ := data["token"].(string)
	require.NotEmpty(t, token)
	return token
}

func TestResetOwnSubscriptionWithThePassword(t *testing.T) {
	f := newResetFixture(t)
	answer, _ := f.reset(t, 2, `{"password":"`+resetPassword+`"}`, map[string][]string{"Idempotency-Key": {"key-1"}})
	token := requireIssued(t, answer)
	require.Equal(t, f.kernel.tokens[2], token, "the answer is the subscriber's new token")
	require.NotEqual(t, "t2", token)
	require.Len(t, f.kernel.resets, 1)
	reset := f.kernel.resets[0]
	require.Equal(t, ResetRequestID("user_reset_subscribe", 2, "key-1"), reset.GetRequestId())
	require.True(t, strings.HasPrefix(reset.GetRequestId(), "identity.user_reset_subscribe:2:"))
	require.Equal(t, uint64(2), reset.GetUserId())
	require.True(t, reset.GetSubscriptionToken(), "the subscription token rotates")
	require.False(t, reset.GetProxyUuid(), "the proxy uuid is kept, as the administrator's reset keeps it")

	again, _ := f.reset(t, 2, `{"password":"`+resetPassword+`"}`, map[string][]string{"Idempotency-Key": {"key-1"}})
	require.Equal(t, token, requireIssued(t, again), "a retried request answers the token the first one issued")
}

func TestResetOwnSubscriptionRefusesWithoutTheRightPassword(t *testing.T) {
	f := newResetFixture(t)
	for _, test := range []struct{ body, message string }{
		{`{"password":"wrong"}`, "invalid password"},
		{`{}`, "password required"},
		{`{"code":"123456"}`, "password required"},
		{``, "参数错误"},
		{`{"password":`, "参数错误"},
	} {
		answer, _ := f.reset(t, 2, test.body, nil)
		requireRefused(t, answer, test.message)
	}
	require.Empty(t, f.kernel.resets, "nothing is reset")
	require.Equal(t, "t2", f.kernel.tokens[2])

	answer, _ := f.reset(t, 99, `{"password":"`+resetPassword+`"}`, nil)
	requireRefused(t, answer, "用户不存在")
}

func TestResetOwnSubscriptionWithASecondFactor(t *testing.T) {
	f := newResetFixture(t)
	answer, _ := f.reset(t, 5, `{"password":"`+resetPassword+`"}`, nil)
	requireRefused(t, answer, "mfa code required")
	answer, _ = f.reset(t, 5, `{"code":"000000","method":"totp"}`, nil)
	requireRefused(t, answer, "invalid mfa code")
	require.Empty(t, f.kernel.resets)

	code, err := totp.GenerateCode(f.secret, f.now)
	require.NoError(t, err)
	answer, _ = f.reset(t, 5, `{"code":"`+code+`","method":"totp"}`, nil)
	requireIssued(t, answer)
	require.Len(t, f.kernel.resets, 1)

	// A recovery code works once.
	f.now = f.now.Add(2 * time.Hour)
	answer, _ = f.reset(t, 5, `{"code":"`+f.backups[0]+`","method":"backup"}`, nil)
	requireIssued(t, answer)
	answer, _ = f.reset(t, 5, `{"code":"`+f.backups[0]+`","method":"backup"}`, nil)
	requireRefused(t, answer, "invalid mfa code")
	answer, _ = f.reset(t, 5, `{"code":" `+f.backups[1]+` "}`, nil)
	requireIssued(t, answer)
	require.Len(t, f.kernel.resets, 3)
}

func TestResetOwnSubscriptionIsLimitedToThreeAttemptsAnHour(t *testing.T) {
	f := newResetFixture(t)
	right := `{"password":"` + resetPassword + `"}`
	for range 3 {
		answer, _ := f.reset(t, 2, `{"password":"wrong"}`, nil)
		requireRefused(t, answer, "invalid password")
	}
	answer, retry := f.reset(t, 2, right, nil)
	requireRefused(t, answer, "too many subscription reset attempts, please try again later")
	require.Equal(t, "3600", retry)
	require.Empty(t, f.kernel.resets, "a locked user is not reset")

	// The limit is per user.
	code, err := totp.GenerateCode(f.secret, f.now)
	require.NoError(t, err)
	answer, _ = f.reset(t, 5, `{"code":"`+code+`"}`, nil)
	requireIssued(t, answer)

	// Successful resets count too; missing credentials do not.
	f.now = f.now.Add(time.Hour + time.Second)
	for range 3 {
		answer, _ = f.reset(t, 2, `{}`, nil)
		requireRefused(t, answer, "password required")
	}
	for range 3 {
		answer, _ = f.reset(t, 2, right, nil)
		requireIssued(t, answer)
	}
	answer, retry = f.reset(t, 2, right, nil)
	requireRefused(t, answer, "too many subscription reset attempts, please try again later")
	require.NotEmpty(t, retry)
}

func TestResetOwnSubscriptionReportsControlFailures(t *testing.T) {
	f := newResetFixture(t)
	f.kernel.fail = status.Error(codes.Unavailable, "kernel is draining")
	answer, _ := f.reset(t, 2, `{"password":"`+resetPassword+`"}`, nil)
	requireRefused(t, answer, "重置订阅失败: kernel is draining")
	f.kernel.fail = status.Error(codes.NotFound, "subscriber not found")
	f.now = f.now.Add(2 * time.Hour)
	answer, _ = f.reset(t, 2, `{"password":"`+resetPassword+`"}`, nil)
	requireRefused(t, answer, "用户不存在")
}

// Without KernelSubscriber the route has no native handler and stays with
// the kernel's legacy handler.
func TestResetOwnSubscriptionNeedsKernelSubscriber(t *testing.T) {
	require.NotContains(t, (&Service{}).Handlers(), UserSubscriptionResetRouteID)
}
