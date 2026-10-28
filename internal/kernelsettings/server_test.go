package kernelsettings

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/packagebridge"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// grants authorizes the capabilities it holds; fenced refuses every call
// as a fenced generation.
type grants map[string]bool

func (g grants) AuthorizeCapability(_ context.Context, _ packagebridge.HostIdentity, capability string) error {
	if g["fenced"] {
		return packagebridge.ErrHostFenced
	}
	if g[capability] {
		return nil
	}
	return service.ErrCapabilityNotAuthorized
}

func capabilities(namespace string, accesses ...string) grants {
	granted := grants{}
	for _, access := range accesses {
		granted[service.SettingsCapability(namespace, access)] = true
	}
	return granted
}

func (g grants) with(other grants) grants {
	for capability := range other {
		g[capability] = true
	}
	return g
}

// testCleanup runs the cleanups TestMain registers after every test.
type testCleanup struct{ cleanups []func() }

func (c *testCleanup) Cleanup(f func()) { c.cleanups = append(c.cleanups, f) }

// TestMain adds the demo namespace, a remote service's address, token and
// timeout (demo.remote.*), which the tests read and write.
func TestMain(m *testing.M) {
	cleanup := &testCleanup{}
	service.AddSettingsNamespacesForTest(cleanup, service.SettingsNamespace{Name: "demo", Prefixes: []string{"demo.remote."}, Audit: service.SettingsAuditSystemConfig})
	code := m.Run()
	for _, f := range cleanup.cleanups {
		f()
	}
	os.Exit(code)
}

var host = packagebridge.HostIdentity{PackageID: "demo-package", Version: "4.1.0", Generation: 1}

func fixture(t *testing.T, authorizer Authorizer) (*gorm.DB, kernelsettingsv1.KernelSettingsServer) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.OperationLog{}, &model.SettingsRequest{}))
	require.NoError(t, db.Create(&[]model.SystemConfig{
		{Key: "demo.remote.base_url", Value: "http://remote.test", Type: "string", Group: "demo"},
		{Key: "demo.remote.token", Value: "remote-secret", Type: "string", Group: "demo"},
		{Key: "demo.remote.timeout_seconds", Value: "  ", Type: "int", Group: "demo"},
		{Key: "demo.remote_mode", Value: "true", Type: "bool", Group: "demo"},
		{Key: "notification.email.config", Value: `{"host":"smtp.test","password":"smtp-secret"}`, Type: "json", Group: "notification"},
		{Key: "invite.frontend.config", Value: `{"code_prefix":"AFF"}`, Type: "json", Group: "invite"},
		{Key: "invite.", Value: "the bare prefix is no key"},
		{Key: "security.mfa.config", Value: `{"enforce":true}`, Type: "json", Group: "security"},
	}).Error)
	server := &Server{DB: db, Authorizer: authorizer}
	return db, server.For(host)
}

func values(settings []*kernelsettingsv1.Setting) map[string]string {
	result := map[string]string{}
	for _, setting := range settings {
		result[setting.GetKey()] = setting.GetValue()
	}
	return result
}

func audits(t *testing.T, db *gorm.DB) []model.OperationLog {
	t.Helper()
	var rows []model.OperationLog
	require.NoError(t, db.Order("id").Find(&rows).Error)
	return rows
}

func TestEachCallNeedsTheNamespaceCapability(t *testing.T) {
	ctx := context.Background()
	_, readOnly := fixture(t, capabilities("demo", service.SettingsAccessRead))
	_, err := readOnly.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo"})
	require.NoError(t, err)
	_, err = readOnly.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "demo", RequestId: "r",
		Entries: []*kernelsettingsv1.SettingEntry{{Key: "demo.remote.base_url", Value: "http://x"}}})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "reading is not writing")
	_, err = readOnly.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "mail"})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "one namespace's capability reaches no other")

	_, writeOnly := fixture(t, capabilities(service.SettingsNamespaceInvite, service.SettingsAccessWrite))
	_, err = writeOnly.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "invite"})
	require.Equal(t, codes.PermissionDenied, status.Code(err), "writing is not reading")
	_, err = writeOnly.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{Namespace: "invite", RequestId: "d", Keys: []string{"invite.frontend.config"}})
	require.NoError(t, err)

	_, err = readOnly.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "everything"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, fenced := fixture(t, grants{"fenced": true})
	_, err = fenced.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo"})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

// A key outside the named namespace, or in no namespace, is reachable
// through no call.
func TestKeysOutsideTheNamespaceAreRefused(t *testing.T) {
	ctx := context.Background()
	everything := grants{}
	for _, namespace := range service.SettingsNamespaces() {
		everything.with(capabilities(namespace.Name, service.SettingsAccessRead, service.SettingsAccessWrite, service.SettingsAccessSecrets))
	}
	db, server := fixture(t, everything)
	for _, key := range []string{"security.mfa.config", "invite.frontend.config", "demo.remote_mode", "demo.remote.", "", "demo.remote.a b"} {
		_, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo", Keys: []string{key}})
		require.Equal(t, codes.InvalidArgument, status.Code(err), key)
		_, err = server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "demo", RequestId: "p:" + key,
			Entries: []*kernelsettingsv1.SettingEntry{{Key: key, Value: "x"}}})
		require.Equal(t, codes.InvalidArgument, status.Code(err), key)
		_, err = server.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{Namespace: "demo", RequestId: "d:" + key, Keys: []string{key}})
		require.Equal(t, codes.InvalidArgument, status.Code(err), key)
	}
	// The backup configuration namespace went with the backup settings.
	_, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "backup"})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "the backup namespace is gone")
	require.Contains(t, status.Convert(err).Message(), `unknown settings namespace "backup"`)
	var mfa model.SystemConfig
	require.NoError(t, db.Where("key = ?", "security.mfa.config").Take(&mfa).Error)
	require.JSONEq(t, `{"enforce":true}`, mfa.Value)

	// Listing a namespace answers its own stored keys only, by key.
	listed, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo"})
	require.NoError(t, err)
	var keys []string
	for _, setting := range listed.GetSettings() {
		keys = append(keys, setting.GetKey())
	}
	require.Equal(t, []string{"demo.remote.base_url", "demo.remote.timeout_seconds", "demo.remote.token"}, keys)
	listed, err = server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "invite"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"invite.frontend.config": `{"code_prefix":"AFF"}`}, values(listed.GetSettings()))
}

func TestSecretsAreMaskedWithoutTheSecretsCapability(t *testing.T) {
	ctx := context.Background()
	keys := []string{"demo.remote.token", "demo.remote.base_url", "demo.remote.timeout_seconds", "demo.remote.missing_token"}

	_, masked := fixture(t, capabilities("demo", service.SettingsAccessRead).with(capabilities("mail", service.SettingsAccessRead)))
	got, err := masked.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo", Keys: keys})
	require.NoError(t, err)
	settings := got.GetSettings()
	require.Len(t, settings, 4)
	require.Equal(t, "demo.remote.token", settings[0].GetKey(), "answered in the requested order")
	require.Equal(t, "********", settings[0].GetValue())
	require.True(t, settings[0].GetSecret() && settings[0].GetMasked() && settings[0].GetHasValue() && settings[0].GetStored())
	require.Equal(t, "http://remote.test", settings[1].GetValue())
	require.False(t, settings[1].GetSecret() || settings[1].GetMasked())
	require.Equal(t, "string", settings[1].GetType())
	require.Equal(t, "  ", settings[2].GetValue())
	require.False(t, settings[2].GetHasValue())
	require.Equal(t, "", settings[3].GetValue(), "a secret with no value reads empty")
	require.True(t, settings[3].GetMasked() && !settings[3].GetStored() && !settings[3].GetHasValue())

	// The e-mail configuration is one JSON value holding the password: the
	// mail namespace declares it secret.
	mail, err := masked.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "mail"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"notification.email.config": "********"}, values(mail.GetSettings()))

	_, clear := fixture(t, capabilities("demo", service.SettingsAccessRead, service.SettingsAccessSecrets))
	got, err = clear.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo", Keys: keys[:1]})
	require.NoError(t, err)
	require.Equal(t, "remote-secret", got.GetSettings()[0].GetValue())
	require.True(t, got.GetSettings()[0].GetSecret())
	require.False(t, got.GetSettings()[0].GetMasked())
}

func TestPutKeepsSecretsRecordsAuditAndAppliesOnce(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("demo", service.SettingsAccessRead, service.SettingsAccessWrite, service.SettingsAccessSecrets))
	actor := &kernelsettingsv1.Actor{UserId: ptr(uint64(7)), ClientIp: "192.0.2.4", UserAgent: "ua/1"}
	before := service.SettingsGeneration("demo")
	put := &kernelsettingsv1.PutSettingsRequest{Namespace: "demo", RequestId: "demo.remote:1", Actor: actor, Entries: []*kernelsettingsv1.SettingEntry{
		{Key: "demo.remote.token", Value: "********", Remark: "kept"},
		{Key: "demo.remote.base_url", Value: "http://remote.example", Group: "demo"},
		{Key: "demo.remote.api_token", Keep: true},
		{Key: "demo.remote.timeout_seconds", Value: "9"},
		{Key: "demo.remote.region", Value: "eu", Type: "string"},
	}}
	response, err := server.PutSettings(ctx, put)
	require.NoError(t, err)
	require.True(t, response.GetApplied())
	require.Equal(t, before+1, service.SettingsGeneration("demo"), "the write bumps the namespace once")
	changes := response.GetChanges()
	require.Len(t, changes, 5)
	require.True(t, changes[0].GetKept())
	require.False(t, changes[2].GetKept() || changes[2].GetCreated(), "nothing to keep: the key is not created")
	require.True(t, changes[4].GetCreated())

	got, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"demo.remote.token": "remote-secret", "demo.remote.base_url": "http://remote.example",
		"demo.remote.timeout_seconds": "9", "demo.remote.region": "eu",
	}, values(got.GetSettings()))
	var token model.SystemConfig
	require.NoError(t, db.Where("key = ?", "demo.remote.token").Take(&token).Error)
	require.Equal(t, "kept", token.Remark, "a kept secret still takes its metadata")

	entries := audits(t, db)
	require.Len(t, entries, 4, "the system configuration handlers' entry per written key")
	require.Equal(t, "update", entries[0].Action)
	require.Equal(t, "system", entries[0].Module)
	require.Equal(t, "system_config", entries[0].TargetType)
	require.Equal(t, uint(7), *entries[0].UserID)
	require.Equal(t, "192.0.2.4", entries[0].IP)
	require.Equal(t, "ua/1", entries[0].UserAgent)
	require.Empty(t, entries[0].Username)
	require.JSONEq(t, `{"key":"demo.remote.token","group":"demo","type":"string","sensitive":true,"has_value":true,"preserve_existing":true}`, entries[0].Content)
	require.NotContains(t, entries[0].Content, "remote-secret")
	require.Equal(t, "create", entries[3].Action)
	require.JSONEq(t, `{"key":"demo.remote.region","group":"","type":"string","sensitive":false,"has_value":true,"preserve_existing":false}`, entries[3].Content)

	// A repeat answers the first result and changes nothing.
	put.Entries[1].Value = "http://elsewhere"
	again, err := server.PutSettings(ctx, put)
	require.NoError(t, err)
	require.False(t, again.GetApplied())
	require.Len(t, again.GetChanges(), 5)
	require.True(t, again.GetChanges()[4].GetCreated())
	require.Len(t, audits(t, db), 4)
	require.Equal(t, before+1, service.SettingsGeneration("demo"))
	got, err = server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "demo", Keys: []string{"demo.remote.base_url"}})
	require.NoError(t, err)
	require.Equal(t, "http://remote.example", got.GetSettings()[0].GetValue())

	// The id names that write: as a delete it is refused.
	_, err = server.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{Namespace: "demo", RequestId: "demo.remote:1", Keys: []string{"demo.remote.region"}})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestPutValidatesBeforeWriting(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("demo", service.SettingsAccessWrite))
	entry := func(key, value string) []*kernelsettingsv1.SettingEntry {
		return []*kernelsettingsv1.SettingEntry{{Key: key, Value: value}}
	}
	for name, request := range map[string]*kernelsettingsv1.PutSettingsRequest{
		"no request id":   {Namespace: "demo", Entries: entry("demo.remote.base_url", "x")},
		"long request id": {Namespace: "demo", RequestId: strings.Repeat("r", 129), Entries: entry("demo.remote.base_url", "x")},
		"no entries":      {Namespace: "demo", RequestId: "r"},
		"repeated key":    {Namespace: "demo", RequestId: "r", Entries: append(entry("demo.remote.base_url", "x"), entry("demo.remote.base_url", "y")...)},
		"unknown type":    {Namespace: "demo", RequestId: "r", Entries: []*kernelsettingsv1.SettingEntry{{Key: "demo.remote.base_url", Type: "yaml"}}},
		"NUL value":       {Namespace: "demo", RequestId: "r", Entries: entry("demo.remote.base_url", "a\x00b")},
		"bad client ip":   {Namespace: "demo", RequestId: "r", Entries: entry("demo.remote.base_url", "x"), Actor: &kernelsettingsv1.Actor{ClientIp: "nowhere"}},
	} {
		_, err := server.PutSettings(ctx, request)
		require.Equal(t, codes.InvalidArgument, status.Code(err), name)
	}
	var count int64
	require.NoError(t, db.Model(&model.SettingsRequest{}).Count(&count).Error)
	require.Zero(t, count)
	require.Empty(t, audits(t, db))
}

// E-mail and invite configuration writes record the system configuration
// handler's entry per key, as their kernel handlers now do: the masked
// password is named, never its value. The username is the actor's e-mail.
func TestMailAndInviteWritesRecordTheSystemConfigAudit(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("mail", service.SettingsAccessWrite).with(capabilities("invite", service.SettingsAccessWrite)))
	require.NoError(t, db.AutoMigrate(&model.User{}))
	require.NoError(t, db.Create(&model.User{ID: 7, Email: "admin@example.test", Password: "x", UUID: "u-7", Token: "t-7"}).Error)
	actor := &kernelsettingsv1.Actor{UserId: ptr(uint64(7)), ClientIp: "192.0.2.4", UserAgent: "ua/1"}
	_, err := server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "mail", RequestId: "m", Actor: actor, Entries: []*kernelsettingsv1.SettingEntry{
		{Key: "notification.email.config", Value: `{"host":"smtp.example","password":"********"}`, Type: "json", Group: "notification", Remark: "Email notification config"},
	}})
	require.NoError(t, err)
	_, err = server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "invite", RequestId: "i", Actor: actor, Entries: []*kernelsettingsv1.SettingEntry{
		{Key: "invite.frontend.config", Value: `{"code_prefix":"NEW"}`, Type: "json", Group: "invite", Remark: "Invite frontend configuration fields"},
	}})
	require.NoError(t, err)
	_, err = server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "mail", RequestId: "m2", Entries: []*kernelsettingsv1.SettingEntry{
		{Key: "notification.email.sender", Value: "x"},
	}})
	require.NoError(t, err)

	var stored model.SystemConfig
	require.NoError(t, db.Where("key = ?", "notification.email.config").Take(&stored).Error)
	require.Equal(t, `{"host":"smtp.example","password":"smtp-secret"}`, stored.Value)
	require.Equal(t, "Email notification config", stored.Remark)

	entries := audits(t, db)
	require.Len(t, entries, 3)
	for _, entry := range entries[:2] {
		require.Equal(t, "system", entry.Module)
		require.Equal(t, "system_config", entry.TargetType)
		require.Equal(t, "update", entry.Action)
		require.Equal(t, uint(7), *entry.UserID)
		require.Equal(t, "admin@example.test", entry.Username)
		require.Equal(t, "192.0.2.4", entry.IP)
		require.Equal(t, "ua/1", entry.UserAgent)
		require.NotContains(t, entry.Content, "smtp-secret")
	}
	require.Equal(t, stored.ID, *entries[0].TargetID)
	require.JSONEq(t, `{"key":"notification.email.config","group":"notification","type":"json","sensitive":false,"has_value":true,
		"preserve_existing":true,"masked_fields":["password"],"masked_fields_with_value":["password"]}`, entries[0].Content)
	require.JSONEq(t, `{"key":"invite.frontend.config","group":"invite","type":"json","sensitive":false,"has_value":true,"preserve_existing":false}`, entries[1].Content)
	require.Equal(t, "create", entries[2].Action)
	require.Nil(t, entries[2].UserID)
	require.Empty(t, entries[2].Username, "no actor, no user")
}

// The e-mail configuration's password is a masked field: a write that
// sends it as the placeholder keeps the stored password, and a new one
// replaces it.
func TestMailWriteKeepsThePasswordSentAsThePlaceholder(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("mail", service.SettingsAccessWrite))
	put := func(id, value string) {
		_, err := server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "mail", RequestId: id, Entries: []*kernelsettingsv1.SettingEntry{
			{Key: "notification.email.config", Value: value, Type: "json"},
		}})
		require.NoError(t, err)
	}
	stored := func() string {
		var row model.SystemConfig
		require.NoError(t, db.Where("key = ?", "notification.email.config").Take(&row).Error)
		return row.Value
	}
	put("keep", `{"host":"smtp.new","password":"********"}`)
	require.Equal(t, `{"host":"smtp.new","password":"smtp-secret"}`, stored())
	put("rotate", `{"host":"smtp.new","password":"rotated"}`)
	require.Equal(t, `{"host":"smtp.new","password":"rotated"}`, stored())
	put("whole", "********")
	require.Equal(t, `{"host":"smtp.new","password":"rotated"}`, stored(), "the placeholder for the whole secret keeps it")
}

func TestDeleteRemovesNamespaceKeysWithTheirAudit(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("demo", service.SettingsAccessWrite))
	response, err := server.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{
		Namespace: "demo", RequestId: "d", Keys: []string{"demo.remote.token", "demo.remote.absent"},
		Actor: &kernelsettingsv1.Actor{UserId: ptr(uint64(1))},
	})
	require.NoError(t, err)
	require.True(t, response.GetApplied())
	require.True(t, response.GetChanges()[0].GetDeleted())
	require.False(t, response.GetChanges()[1].GetDeleted())
	var count int64
	require.NoError(t, db.Model(&model.SystemConfig{}).Where("key = ?", "demo.remote.token").Count(&count).Error)
	require.Zero(t, count)
	entries := audits(t, db)
	require.Len(t, entries, 1)
	require.Equal(t, "delete", entries[0].Action)
	require.JSONEq(t, `{"key":"demo.remote.token","group":"demo","type":"string","sensitive":true,"has_value":true,"preserve_existing":false}`, entries[0].Content)
}

func TestUserAgentIsCutToTheAuditColumn(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("demo", service.SettingsAccessWrite))
	_, err := server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "demo", RequestId: "u",
		Entries: []*kernelsettingsv1.SettingEntry{{Key: "demo.remote.base_url", Value: "x"}},
		Actor:   &kernelsettingsv1.Actor{UserAgent: strings.Repeat("é", 300)}})
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("é", 255), audits(t, db)[0].UserAgent)
}

func TestRequestLedgerPrune(t *testing.T) {
	db, _ := fixture(t, grants{})
	now := time.Now()
	require.NoError(t, db.Create(&[]model.SettingsRequest{
		{RequestID: "old", Namespace: "demo", Method: "put", CreatedAt: now.Add(-RequestRetention - time.Hour)},
		{RequestID: "new", Namespace: "demo", Method: "put", CreatedAt: now},
	}).Error)
	pruned, err := PruneRequests(db, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), pruned)
}

func ptr[T any](value T) *T { return &value }
