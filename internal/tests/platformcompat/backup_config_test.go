package platformcompat

import (
	"context"
	"sync"
	"testing"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/v4/internal/handler"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/AnixOps/anix-control/v4/internal/tests/packagecompat"
	"github.com/AnixOps/anix-control/v4/packages/platform/native"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// platformHost is the identity the kernel serves the platform host as; its
// signed release declares kernel.settings.backup.write.v1.
var platformHost = packagebridge.HostIdentity{PackageID: "platform", Version: "4.1.0", Generation: 1}

// backupConfigUpdate is the backup configuration update: the legacy handler
// against the native one, which writes through the real KernelSettings
// server on the native side's database.
func backupConfigUpdate(t *testing.T) packagecompat.Route {
	return packagecompat.Route{
		Method: "PUT", Pattern: "/api/v2/admin/system/backup/config", RouteID: native.BackupConfigUpdateRouteID,
		Legacy: system((*handler.SystemHandler).UpdateBackupConfig),
		Models: []any{&model.OperationLog{}, &model.BackupConfig{}, &model.BackupRecord{}, &model.SettingsRequest{}},
		Native: func(db *gorm.DB) pluginhostsdk.NativeHandler {
			service := &native.Service{
				Open: func(ctx context.Context) (*gorm.DB, error) { return db.WithContext(ctx), nil },
				Settings: packagecompat.KernelSettings(t, db, packagecompat.SettingsGrants{
					Host: platformHost, Capabilities: []string{service.SettingsCapability(service.SettingsNamespaceBackup, service.SettingsAccessWrite)},
				}),
			}
			return service.Handlers()[native.BackupConfigUpdateRouteID]
		},
	}
}

// kernelCopies are the kernel's backup services by database: each loaded
// the configuration into memory before the request, as the kernel's
// long-lived one has, and is read again after it.
var kernelCopies sync.Map

// warm seeds, then loads the configuration into a kernel backup service.
func warm(seed func(testing.TB, *gorm.DB)) func(testing.TB, *gorm.DB) {
	return func(t testing.TB, db *gorm.DB) {
		seed(t, db)
		backups := service.NewBackupService(db)
		_, err := backups.GetConfig()
		require.NoError(t, err)
		kernelCopies.Store(db, backups)
	}
}

// backupConfigState is the backup configuration rows, the system audit
// entries and the configuration the kernel's backup service holds in
// memory after the request, without the times taken from the clock.
func backupConfigState(t testing.TB, db *gorm.DB) any {
	var audit []struct {
		ID         uint
		UserID     *uint
		Username   string
		Action     string
		Module     string
		TargetType string
		TargetID   *uint
		Content    string
		IP         string
		UserAgent  string
		Status     int
	}
	require.NoError(t, db.Model(&model.OperationLog{}).Order("id").Find(&audit).Error)
	backups := service.NewBackupService(db)
	if warmed, ok := kernelCopies.Load(db); ok {
		backups = warmed.(*service.BackupService)
	}
	memory, err := backups.GetConfig()
	require.NoError(t, err)
	copied := *memory
	copied.CreatedAt, copied.UpdatedAt = seeded, seeded
	return map[string]any{"rows": backupConfigs(t, db), "audit": audit, "memory": copied}
}

var storedS3 = model.BackupConfig{
	ID: 1, Enabled: true, AutoBackup: true, Schedule: "interval:6", RetentionDays: 14, BackupDatabase: true, BackupFiles: true,
	StorageType: "s3", StoragePath: "/srv/backups", S3Bucket: "bucket", S3Region: "eu-west-1", S3Endpoint: "https://s3.example.test",
	S3AccessKey: "AKIAEXAMPLE", S3SecretKey: "wJalrXUtnFEMI",
}

func TestBackupConfigUpdateRouteParity(t *testing.T) {
	stored := warm(seedBackupConfig(storedS3))
	clock := []string{"data.updated_at"}
	created := []string{"data.created_at", "data.updated_at"}
	for _, c := range []packagecompat.Case{
		{Name: "every field, new S3 keys", Body: []byte(`{"enabled":false,"auto_backup":false,"schedule":"0 3 * * *","retention_days":30,
			"backup_database":false,"backup_files":false,"storage_type":"local","storage_path":"/var/backups","s3_bucket":"b2","s3_region":"us-east-1",
			"s3_endpoint":"https://s3.other.test","s3_access_key":"AKIANEW","s3_secret_key":"newsecret"}`), Seed: stored, Mask: clock},
		{Name: "the placeholder keeps both S3 keys", Body: []byte(`{"storage_path":"/new","s3_access_key":"********","s3_secret_key":"********"}`), Seed: stored, Mask: clock},
		{Name: "null and blank keep the S3 keys when asked", Body: []byte(`{"preserve_existing_sensitive":true,"s3_access_key":null,"s3_secret_key":"  "}`), Seed: stored, Mask: clock},
		{Name: "null and blank clear the S3 keys otherwise", Body: []byte(`{"s3_access_key":null,"s3_secret_key":""}`), Seed: stored, Mask: clock},
		{Name: "keep_count and a positive interval win", Body: []byte(`{"retention_days":3,"keep_count":9.7,"schedule":"0 1 * * *","interval":12.9}`), Seed: stored, Mask: clock},
		{Name: "an interval below one leaves the schedule", Body: []byte(`{"schedule":"0 1 * * *","interval":0.5}`), Seed: stored, Mask: clock},
		{Name: "fields of another type are ignored", Body: []byte(`{"enabled":"yes","retention_days":"5","s3_access_key":5,"storage_type":7,"preserve_existing_sensitive":"true","s3_secret_key":""}`), Seed: stored, Mask: clock},
		{Name: "an empty update saves and audits", Body: []byte(`{}`), Seed: stored, Mask: clock},
		{Name: "a null body", Body: []byte(`null`), Seed: stored, Mask: clock},
		{Name: "no row: the defaults first", Body: []byte(`{"storage_path":"/first","s3_secret_key":"********"}`), Mask: created},
		{Name: "the first of two rows", Body: []byte(`{"enabled":true}`), Seed: warm(seedTwoBackupConfigs), Mask: clock},
		{Name: "the request's user agent in the audit", Body: []byte(`{"enabled":false}`), Seed: stored, Mask: clock,
			RequestHeaders: map[string]string{"User-Agent": "parity-agent/1.0"}},
		{Name: "a body that is not an object", Body: []byte(`[1]`), Seed: stored},
		{Name: "a body that does not parse, no row yet", Body: []byte(`{"enabled":`)},
	} {
		c.Path = "/api/v2/admin/system/backup/config"
		c.Principal = admin
		c.Snapshot = backupConfigState
		packagecompat.RunWrite(t, backupConfigUpdate(t), c)
	}
}
