package service

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// openPostgresTestDB opens POSTGRES_TEST_DSN, refusing databases whose name
// does not contain "test", and recreates the given tables.
func openPostgresTestDB(t *testing.T, models ...any) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("POSTGRES_TEST_DSN"))
	if dsn == "" {
		t.Skip("POSTGRES_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	var databaseName string
	require.NoError(t, db.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("POSTGRES_TEST_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	require.NoError(t, db.Migrator().DropTable(models...))
	require.NoError(t, db.AutoMigrate(models...))
	t.Cleanup(func() { _ = db.Migrator().DropTable(models...) })
	return db
}

// Two processes applying the same traffic snapshot must record its delta once.
// The in-process lock cannot help across processes, so this calls the
// transactional part directly from concurrent goroutines.
func TestPostgresConcurrentTrafficSnapshotsCountOnce(t *testing.T) {
	db := openPostgresTestDB(t, &model.User{}, &model.ForwardTunnel{}, &model.ForwardUserTunnel{}, &model.Forward{}, &model.ForwardTrafficCursor{})

	user := &model.User{Email: "pg-cursor@example.com", Token: "pg-cursor-token", UUID: "pg-cursor-uuid"}
	require.NoError(t, db.Create(user).Error)
	tunnel := &model.ForwardTunnel{Name: "pg-cursor-tunnel", InIP: "10.42.0.1", Status: model.ForwardTunnelStatusActive, Flow: 1, TrafficRatio: 1}
	require.NoError(t, db.Create(tunnel).Error)
	forward := &model.Forward{UserID: user.ID, UserName: user.Email, Name: "pg-cursor", TunnelID: tunnel.ID, InPort: 16001, RemoteAddr: "example.test:443", Status: model.ForwardStatusActive}
	require.NoError(t, db.Create(forward).Error)

	service := &PanelForwardService{db: db}
	snapshot := PanelForwardTrafficSnapshot{ForwardID: forward.ID, Backend: "gost", UploadTotal: 1000, DownloadTotal: 2000}
	const workers = 8
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	start := make(chan struct{})
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := service.advanceForwardTrafficCursor(snapshot, "gost")
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var stored model.Forward
	require.NoError(t, db.First(&stored, forward.ID).Error)
	require.EqualValues(t, 2000, stored.InFlow, "download delta must be counted once")
	require.EqualValues(t, 1000, stored.OutFlow, "upload delta must be counted once")
	var cursors int64
	require.NoError(t, db.Model(&model.ForwardTrafficCursor{}).Where("forward_id = ?", forward.ID).Count(&cursors).Error)
	require.EqualValues(t, 1, cursors)
}

// Two processes starting on a reset day must reset once.
func TestPostgresConcurrentDailyClaimsRunOnce(t *testing.T) {
	db := openPostgresTestDB(t, &model.SystemConfig{})
	now := time.Date(2026, 3, 6, 0, 0, 5, 0, time.Local)

	const workers = 8
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		claims  int
		results []error
	)
	start := make(chan struct{})
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := db.Transaction(func(tx *gorm.DB) error {
				claimed, err := claimDailyRun(tx, forwardFlowResetRunKey, now)
				if err == nil && claimed {
					mu.Lock()
					claims++
					mu.Unlock()
				}
				return err
			})
			mu.Lock()
			results = append(results, err)
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()
	for _, err := range results {
		require.NoError(t, err)
	}
	require.Equal(t, 1, claims)
}
