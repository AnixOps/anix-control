package kernelsettings

import (
	"context"
	"strings"
	"testing"
	"time"

	kernelsettingsv1 "github.com/AnixOps/anix-control/sdk/api/kernelsettings/v1"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
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

var host = packagebridge.HostIdentity{PackageID: "gost-mesh", Version: "4.1.0", Generation: 1}

func fixture(t *testing.T, authorizer Authorizer) (*gorm.DB, kernelsettingsv1.KernelSettingsServer) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.SystemConfig{}, &model.BackupConfig{}, &model.OperationLog{}, &model.SettingsRequest{}))
	require.NoError(t, db.Create(&[]model.SystemConfig{
		{Key: "forward.runtime.nodex.base_url", Value: "http://nodex.test", Type: "string", Group: "forward_runtime"},
		{Key: "forward.runtime.nodex.token", Value: "nodex-secret", Type: "string", Group: "forward_runtime"},
		{Key: "forward.runtime.nodex.timeout_seconds", Value: "  ", Type: "int", Group: "forward_runtime"},
		{Key: "forward.runtime.nodex_mode", Value: "true", Type: "bool", Group: "forward_runtime"},
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
	_, readOnly := fixture(t, capabilities(service.SettingsNamespaceNodeX, service.SettingsAccessRead))
	_, err := readOnly.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex"})
	require.NoError(t, err)
	_, err = readOnly.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "nodex", RequestId: "r",
		Entries: []*kernelsettingsv1.SettingEntry{{Key: "forward.runtime.nodex.base_url", Value: "http://x"}}})
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
	_, err = fenced.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex"})
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
	for _, key := range []string{"security.mfa.config", "invite.frontend.config", "forward.runtime.nodex_mode", "forward.runtime.nodex.", "", "forward.runtime.nodex.a b"} {
		_, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex", Keys: []string{key}})
		require.Equal(t, codes.InvalidArgument, status.Code(err), key)
		_, err = server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "nodex", RequestId: "p:" + key,
			Entries: []*kernelsettingsv1.SettingEntry{{Key: key, Value: "x"}}})
		require.Equal(t, codes.InvalidArgument, status.Code(err), key)
		_, err = server.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{Namespace: "nodex", RequestId: "d:" + key, Keys: []string{key}})
		require.Equal(t, codes.InvalidArgument, status.Code(err), key)
	}
	_, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "backup", Keys: []string{"backup.password"}})
	require.Equal(t, codes.InvalidArgument, status.Code(err), "a backup key must name a field")
	var mfa model.SystemConfig
	require.NoError(t, db.Where("key = ?", "security.mfa.config").Take(&mfa).Error)
	require.JSONEq(t, `{"enforce":true}`, mfa.Value)

	// Listing a namespace answers its own stored keys only, by key.
	listed, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex"})
	require.NoError(t, err)
	var keys []string
	for _, setting := range listed.GetSettings() {
		keys = append(keys, setting.GetKey())
	}
	require.Equal(t, []string{"forward.runtime.nodex.base_url", "forward.runtime.nodex.timeout_seconds", "forward.runtime.nodex.token"}, keys)
	listed, err = server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "invite"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{"invite.frontend.config": `{"code_prefix":"AFF"}`}, values(listed.GetSettings()))
}

func TestSecretsAreMaskedWithoutTheSecretsCapability(t *testing.T) {
	ctx := context.Background()
	keys := []string{"forward.runtime.nodex.token", "forward.runtime.nodex.base_url", "forward.runtime.nodex.timeout_seconds", "forward.runtime.nodex.missing_token"}

	_, masked := fixture(t, capabilities("nodex", service.SettingsAccessRead).with(capabilities("mail", service.SettingsAccessRead)))
	got, err := masked.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex", Keys: keys})
	require.NoError(t, err)
	settings := got.GetSettings()
	require.Len(t, settings, 4)
	require.Equal(t, "forward.runtime.nodex.token", settings[0].GetKey(), "answered in the requested order")
	require.Equal(t, "********", settings[0].GetValue())
	require.True(t, settings[0].GetSecret() && settings[0].GetMasked() && settings[0].GetHasValue() && settings[0].GetStored())
	require.Equal(t, "http://nodex.test", settings[1].GetValue())
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

	_, clear := fixture(t, capabilities("nodex", service.SettingsAccessRead, service.SettingsAccessSecrets))
	got, err = clear.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex", Keys: keys[:1]})
	require.NoError(t, err)
	require.Equal(t, "nodex-secret", got.GetSettings()[0].GetValue())
	require.True(t, got.GetSettings()[0].GetSecret())
	require.False(t, got.GetSettings()[0].GetMasked())
}

func TestPutKeepsSecretsRecordsAuditAndAppliesOnce(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("nodex", service.SettingsAccessRead, service.SettingsAccessWrite, service.SettingsAccessSecrets))
	actor := &kernelsettingsv1.Actor{UserId: ptr(uint64(7)), ClientIp: "192.0.2.4", UserAgent: "ua/1"}
	before := service.SettingsGeneration("nodex")
	put := &kernelsettingsv1.PutSettingsRequest{Namespace: "nodex", RequestId: "gost.nodex:1", Actor: actor, Entries: []*kernelsettingsv1.SettingEntry{
		{Key: "forward.runtime.nodex.token", Value: "********", Remark: "kept"},
		{Key: "forward.runtime.nodex.base_url", Value: "http://nodex.example", Group: "nodex"},
		{Key: "forward.runtime.nodex.api_token", Keep: true},
		{Key: "forward.runtime.nodex.timeout_seconds", Value: "9"},
		{Key: "forward.runtime.nodex.region", Value: "eu", Type: "string"},
	}}
	response, err := server.PutSettings(ctx, put)
	require.NoError(t, err)
	require.True(t, response.GetApplied())
	require.Equal(t, before+1, service.SettingsGeneration("nodex"), "the write bumps the namespace once")
	changes := response.GetChanges()
	require.Len(t, changes, 5)
	require.True(t, changes[0].GetKept())
	require.False(t, changes[2].GetKept() || changes[2].GetCreated(), "nothing to keep: the key is not created")
	require.True(t, changes[4].GetCreated())

	got, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex"})
	require.NoError(t, err)
	require.Equal(t, map[string]string{
		"forward.runtime.nodex.token": "nodex-secret", "forward.runtime.nodex.base_url": "http://nodex.example",
		"forward.runtime.nodex.timeout_seconds": "9", "forward.runtime.nodex.region": "eu",
	}, values(got.GetSettings()))
	var token model.SystemConfig
	require.NoError(t, db.Where("key = ?", "forward.runtime.nodex.token").Take(&token).Error)
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
	require.JSONEq(t, `{"key":"forward.runtime.nodex.token","group":"forward_runtime","type":"string","sensitive":true,"has_value":true,"preserve_existing":true}`, entries[0].Content)
	require.NotContains(t, entries[0].Content, "nodex-secret")
	require.Equal(t, "create", entries[3].Action)
	require.JSONEq(t, `{"key":"forward.runtime.nodex.region","group":"","type":"string","sensitive":false,"has_value":true,"preserve_existing":false}`, entries[3].Content)

	// A repeat answers the first result and changes nothing.
	put.Entries[1].Value = "http://elsewhere"
	again, err := server.PutSettings(ctx, put)
	require.NoError(t, err)
	require.False(t, again.GetApplied())
	require.Len(t, again.GetChanges(), 5)
	require.True(t, again.GetChanges()[4].GetCreated())
	require.Len(t, audits(t, db), 4)
	require.Equal(t, before+1, service.SettingsGeneration("nodex"))
	got, err = server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "nodex", Keys: []string{"forward.runtime.nodex.base_url"}})
	require.NoError(t, err)
	require.Equal(t, "http://nodex.example", got.GetSettings()[0].GetValue())

	// The id names that write: as a delete it is refused.
	_, err = server.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{Namespace: "nodex", RequestId: "gost.nodex:1", Keys: []string{"forward.runtime.nodex.region"}})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

func TestPutValidatesBeforeWriting(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("nodex", service.SettingsAccessWrite).with(capabilities("backup", service.SettingsAccessWrite)))
	entry := func(key, value string) []*kernelsettingsv1.SettingEntry {
		return []*kernelsettingsv1.SettingEntry{{Key: key, Value: value}}
	}
	for name, request := range map[string]*kernelsettingsv1.PutSettingsRequest{
		"no request id":    {Namespace: "nodex", Entries: entry("forward.runtime.nodex.base_url", "x")},
		"long request id":  {Namespace: "nodex", RequestId: strings.Repeat("r", 129), Entries: entry("forward.runtime.nodex.base_url", "x")},
		"no entries":       {Namespace: "nodex", RequestId: "r"},
		"repeated key":     {Namespace: "nodex", RequestId: "r", Entries: append(entry("forward.runtime.nodex.base_url", "x"), entry("forward.runtime.nodex.base_url", "y")...)},
		"unknown type":     {Namespace: "nodex", RequestId: "r", Entries: []*kernelsettingsv1.SettingEntry{{Key: "forward.runtime.nodex.base_url", Type: "yaml"}}},
		"NUL value":        {Namespace: "nodex", RequestId: "r", Entries: entry("forward.runtime.nodex.base_url", "a\x00b")},
		"bad client ip":    {Namespace: "nodex", RequestId: "r", Entries: entry("forward.runtime.nodex.base_url", "x"), Actor: &kernelsettingsv1.Actor{ClientIp: "nowhere"}},
		"backup metadata":  {Namespace: "backup", RequestId: "r", Entries: []*kernelsettingsv1.SettingEntry{{Key: "backup.schedule", Value: "x", Group: "g"}}},
		"backup not bool":  {Namespace: "backup", RequestId: "r", Entries: entry("backup.enabled", "maybe")},
		"backup not int":   {Namespace: "backup", RequestId: "r", Entries: entry("backup.retention_days", "7d")},
		"backup not field": {Namespace: "backup", RequestId: "r", Entries: entry("backup.config", "x")},
	} {
		_, err := server.PutSettings(ctx, request)
		require.Equal(t, codes.InvalidArgument, status.Code(err), name)
	}
	var count int64
	require.NoError(t, db.Model(&model.BackupConfig{}).Count(&count).Error)
	require.Zero(t, count, "a refused backup write creates no row")
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
	db, server := fixture(t, capabilities("nodex", service.SettingsAccessWrite).with(capabilities("backup", service.SettingsAccessWrite)))
	response, err := server.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{
		Namespace: "nodex", RequestId: "d", Keys: []string{"forward.runtime.nodex.token", "forward.runtime.nodex.absent"},
		Actor: &kernelsettingsv1.Actor{UserId: ptr(uint64(1))},
	})
	require.NoError(t, err)
	require.True(t, response.GetApplied())
	require.True(t, response.GetChanges()[0].GetDeleted())
	require.False(t, response.GetChanges()[1].GetDeleted())
	var count int64
	require.NoError(t, db.Model(&model.SystemConfig{}).Where("key = ?", "forward.runtime.nodex.token").Count(&count).Error)
	require.Zero(t, count)
	entries := audits(t, db)
	require.Len(t, entries, 1)
	require.Equal(t, "delete", entries[0].Action)
	require.JSONEq(t, `{"key":"forward.runtime.nodex.token","group":"forward_runtime","type":"string","sensitive":true,"has_value":true,"preserve_existing":false}`, entries[0].Content)

	_, err = server.DeleteSettings(ctx, &kernelsettingsv1.DeleteSettingsRequest{Namespace: "backup", RequestId: "b", Keys: []string{"backup.enabled"}})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}

// The backup namespace is the backup configuration row: a write keeps the
// S3 keys it is told to, records the backup handler's audit entry, and the
// kernel's backup service reloads its copy before the call returns.
func TestBackupNamespaceWritesTheRowAndRefreshesTheBackupService(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("backup", service.SettingsAccessRead, service.SettingsAccessWrite))

	listed, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "backup"})
	require.NoError(t, err)
	require.Len(t, listed.GetSettings(), 13)
	require.Equal(t, "backup.auto_backup", listed.GetSettings()[0].GetKey())
	require.False(t, listed.GetSettings()[0].GetStored(), "with no row the defaults read, unstored")
	var count int64
	require.NoError(t, db.Model(&model.BackupConfig{}).Count(&count).Error)
	require.Zero(t, count, "a read creates nothing")

	backups := service.NewBackupService(db)
	require.NoError(t, db.Create(&model.BackupConfig{StorageType: "s3", StoragePath: "/old", S3AccessKey: "AKIA", S3SecretKey: "s3cret", RetentionDays: 3}).Error)
	cached, err := backups.GetConfig()
	require.NoError(t, err)
	require.Equal(t, "/old", cached.StoragePath)

	_, err = server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{
		Namespace: "backup", RequestId: "platform.backup_config:1", Actor: &kernelsettingsv1.Actor{UserId: ptr(uint64(1)), ClientIp: "192.0.2.1"},
		Entries: []*kernelsettingsv1.SettingEntry{
			{Key: "backup.enabled", Value: "true"}, {Key: "backup.retention_days", Value: "14"},
			{Key: "backup.storage_path", Value: "/new"}, {Key: "backup.s3_access_key", Keep: true},
			{Key: "backup.s3_secret_key", Value: "********"}, {Key: "backup.schedule", Value: "interval:6"},
		},
	})
	require.NoError(t, err)
	refreshed, err := backups.GetConfig()
	require.NoError(t, err)
	require.Equal(t, "/new", refreshed.StoragePath, "the backup service's copy reloaded")
	require.True(t, refreshed.Enabled)
	require.Equal(t, 14, refreshed.RetentionDays)
	require.Equal(t, "AKIA", refreshed.S3AccessKey)
	require.Equal(t, "s3cret", refreshed.S3SecretKey)

	entries := audits(t, db)
	require.Len(t, entries, 1)
	require.Equal(t, "backup_config", entries[0].TargetType)
	require.Equal(t, "update", entries[0].Action)
	require.Equal(t, refreshed.ID, *entries[0].TargetID)
	require.JSONEq(t, `{"enabled":true,"auto_backup":false,"schedule":"interval:6","retention_days":14,"backup_database":true,
		"backup_files":false,"storage_type":"s3","storage_path":"/new","s3_bucket":"","s3_region":"","s3_endpoint":"",
		"s3_access_key_has_value":true,"s3_secret_key_has_value":true,"preserved_sensitive_fields":["s3_access_key","s3_secret_key"]}`, entries[0].Content)

	got, err := server.GetSettings(ctx, &kernelsettingsv1.GetSettingsRequest{Namespace: "backup", Keys: []string{"backup.s3_secret_key", "backup.enabled"}})
	require.NoError(t, err)
	require.Equal(t, "********", got.GetSettings()[0].GetValue())
	require.Equal(t, "true", got.GetSettings()[1].GetValue())
	require.True(t, got.GetSettings()[1].GetStored())
}

// A backup write with no row creates the defaults first, as the kernel's
// handler does.
func TestBackupWriteCreatesTheDefaults(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("backup", service.SettingsAccessWrite))
	_, err := server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "backup", RequestId: "b",
		Entries: []*kernelsettingsv1.SettingEntry{{Key: "backup.s3_bucket", Value: "bucket"}}})
	require.NoError(t, err)
	var rows []model.BackupConfig
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, "bucket", rows[0].S3Bucket)
	require.Equal(t, "backups", rows[0].StoragePath)
	require.Equal(t, 7, rows[0].RetentionDays)
	require.True(t, rows[0].BackupDatabase)
	require.Nil(t, audits(t, db)[0].UserID, "no actor, no user")
}

func TestUserAgentIsCutToTheAuditColumn(t *testing.T) {
	ctx := context.Background()
	db, server := fixture(t, capabilities("nodex", service.SettingsAccessWrite))
	_, err := server.PutSettings(ctx, &kernelsettingsv1.PutSettingsRequest{Namespace: "nodex", RequestId: "u",
		Entries: []*kernelsettingsv1.SettingEntry{{Key: "forward.runtime.nodex.base_url", Value: "x"}},
		Actor:   &kernelsettingsv1.Actor{UserAgent: strings.Repeat("é", 300)}})
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("é", 255), audits(t, db)[0].UserAgent)
}

func TestRequestLedgerPrune(t *testing.T) {
	db, _ := fixture(t, grants{})
	now := time.Now()
	require.NoError(t, db.Create(&[]model.SettingsRequest{
		{RequestID: "old", Namespace: "nodex", Method: "put", CreatedAt: now.Add(-RequestRetention - time.Hour)},
		{RequestID: "new", Namespace: "nodex", Method: "put", CreatedAt: now},
	}).Error)
	pruned, err := PruneRequests(db, now)
	require.NoError(t, err)
	require.Equal(t, int64(1), pruned)
}

func ptr[T any](value T) *T { return &value }
