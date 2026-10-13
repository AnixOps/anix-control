package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openPostgresSchema opens a throwaway schema of ANIX_TEST_POSTGRES_DSN, or
// skips.
func openPostgresSchema(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	config := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	admin, err := gorm.Open(postgres.Open(base), config)
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "traffic_buckets_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), config)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func postgresTrafficDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openPostgresSchema(t)
	require.NoError(t, db.AutoMigrate(&model.Node{}, &model.User{}, &model.TrafficLog{}, &model.StatServer{},
		&model.SubscriberRequest{}, &model.SubscriberChange{}))
	for _, id := range []uint{7, 8} {
		require.NoError(t, db.Create(&model.Node{ID: id, Name: fmt.Sprintf("node%d", id), Host: "node.example.com", APIKey: fmt.Sprintf("key%d", id)}).Error)
	}
	for _, id := range []uint{1, 2, 3} {
		require.NoError(t, db.Create(&model.User{ID: id, Email: fmt.Sprintf("pg%d@example.test", id), Token: fmt.Sprintf("tok%d", id), UUID: fmt.Sprintf("uuid%d", id)}).Error)
	}
	return db
}

// The bucket upsert on PostgreSQL: the conflict target is the unique index,
// the old value is qualified, and a batch updates each row once.
func TestPostgresTrafficLogBucketsEqualPerReportRows(t *testing.T) {
	requireBucketsEqualPerReportRows(t, postgresTrafficDB, 2)
}
