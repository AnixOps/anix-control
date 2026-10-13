package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/backup"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/rootkey"
	"github.com/AnixOps/anix-control/v5/internal/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// backupManagerForTest backs up a migrated SQLite file of its own and keeps
// the rows in the router's database.
func backupManagerForTest(t *testing.T) *backup.Manager {
	t.Helper()
	root := t.TempDir()
	dataDir, dbPath := filepath.Join(root, "data"), filepath.Join(root, "control.db")
	require.NoError(t, os.MkdirAll(dataDir, 0o700))
	key, err := rootkey.Generate()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(rootkey.Path(dataDir), rootkey.File{Key: key}.Marshal(), 0o600))
	source, err := database.Open(&config.DatabaseConfig{Driver: "sqlite", Database: dbPath, LogLevel: "silent"})
	require.NoError(t, err)
	_, err = migrations.Migrate(context.Background(), source, migrations.Options{ProgramVersion: "4.2.1"})
	require.NoError(t, err)
	require.NoError(t, database.CloseHandle(source))
	return &backup.Manager{
		DataDir: dataDir, Driver: "sqlite", DatabasePath: dbPath, Domain: "panel.example.com", ProgramVersion: "4.2.1",
		RootKey: func() ([]byte, error) { return os.ReadFile(rootkey.Path(dataDir)) },
		DB:      database.GetDB,
	}
}

func TestBackupRoutes(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	// One connection: each connection of an in-memory database is its own
	// database, and the verification after a backup runs in the background.
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&model.Backup{}))
	// A super administrator (1), an administrator who is staff (2).
	require.NoError(t, db.Create(&model.User{ID: 1, Email: "root@example.com", IsAdmin: 1, Token: "t-1", UUID: "u-1"}).Error)
	require.NoError(t, db.Create(&model.User{ID: 2, Email: "staff@example.com", IsAdmin: 1, IsStaff: 1, Token: "t-2", UUID: "u-2"}).Error)
	manager := backupManagerForTest(t)
	saved := backup.Default()
	backup.SetDefault(manager)
	t.Cleanup(func() { backup.SetDefault(saved) })

	call := func(method, path, token string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	token := func(id uint, email string, admin bool) string {
		value, err := utils.GenerateToken(id, email, admin, cfg.JWT.Secret, cfg.JWT.Expire)
		require.NoError(t, err)
		return value
	}
	root, staff, user := token(1, "root@example.com", true), token(2, "staff@example.com", true), token(3, "user@example.com", false)

	for _, route := range [][2]string{{http.MethodGet, "/api/v5/backups"}, {http.MethodPost, "/api/v5/backups"}, {http.MethodGet, "/api/v5/backups/1/download"}} {
		assert.Equal(t, http.StatusUnauthorized, call(route[0], route[1], "").Code, route[1])
		assert.Equal(t, http.StatusForbidden, call(route[0], route[1], user).Code, route[1])
	}

	listed := call(http.MethodGet, "/api/v5/backups", staff)
	require.Equal(t, http.StatusOK, listed.Code, listed.Body.String())
	var answer struct {
		Data struct {
			Supported bool `json:"supported"`
			Backups   []struct {
				ID           uint   `json:"id"`
				File         string `json:"file"`
				VerifyStatus string `json:"verify_status"`
			} `json:"backups"`
			Summary struct {
				LastVerified    *struct{ File string } `json:"last_verified"`
				Stale           bool                   `json:"stale"`
				StaleAfterHours int                    `json:"stale_after_hours"`
				NextDailyAt     *time.Time             `json:"next_daily_at"`
			} `json:"summary"`
			Retention struct {
				Daily int `json:"daily"`
			} `json:"retention"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &answer))
	assert.True(t, answer.Data.Supported)
	assert.Empty(t, answer.Data.Backups)
	assert.Nil(t, answer.Data.Summary.LastVerified)
	assert.True(t, answer.Data.Summary.Stale, "no verified backup, and no recorded creation time")
	assert.Equal(t, 36, answer.Data.Summary.StaleAfterHours)
	require.NotNil(t, answer.Data.Summary.NextDailyAt)
	assert.Equal(t, 7, answer.Data.Retention.Daily)

	// Any administrator may back up; the verification follows.
	created := call(http.MethodPost, "/api/v5/backups", staff)
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	assert.Contains(t, created.Body.String(), `"type":"manual"`)
	assert.Contains(t, created.Body.String(), `"verify_status":"pending"`)
	require.Eventually(t, func() bool {
		var rows []model.Backup
		return db.Find(&rows).Error == nil && len(rows) == 1 && rows[0].VerifyStatus == model.BackupVerifyPassed
	}, 30*time.Second, 50*time.Millisecond, "the background verification passes")

	listed = call(http.MethodGet, "/api/v5/backups", staff)
	require.NoError(t, json.Unmarshal(listed.Body.Bytes(), &answer))
	require.Len(t, answer.Data.Backups, 1)
	require.NotNil(t, answer.Data.Summary.LastVerified)
	assert.False(t, answer.Data.Summary.Stale)
	id := answer.Data.Backups[0].ID
	file := answer.Data.Backups[0].File

	// The download holds the root key: a super administrator only.
	download := fmt.Sprintf("/api/v5/backups/%d/download", id)
	refused := call(http.MethodGet, download, staff)
	assert.Equal(t, http.StatusForbidden, refused.Code)
	assert.Contains(t, refused.Body.String(), "super_admin_required")
	downloaded := call(http.MethodGet, download, root)
	require.Equal(t, http.StatusOK, downloaded.Code, downloaded.Body.String())
	assert.Equal(t, "application/x-tar", downloaded.Header().Get("Content-Type"))
	assert.Equal(t, `attachment; filename="`+file+`"`, downloaded.Header().Get("Content-Disposition"))
	assert.Equal(t, "no-store", downloaded.Header().Get("Cache-Control"))
	onDisk, err := os.ReadFile(filepath.Join(manager.Dir(), file))
	require.NoError(t, err)
	assert.Equal(t, onDisk, downloaded.Body.Bytes())
	assert.Equal(t, http.StatusNotFound, call(http.MethodGet, "/api/v5/backups/999/download", root).Code)

	// There is no restore route.
	assert.Equal(t, http.StatusNotFound, call(http.MethodPost, fmt.Sprintf("/api/v5/backups/%d/restore", id), root).Code)
}

// The old backup routes are gone: their schedule, retention and S3 settings
// ran nothing, and the online restore overwrote the open database. They
// answer 404, to an administrator too.
func TestRemovedBackupRoutesAnswer404(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, database.GetDB().Create(&model.User{ID: 1, Email: "root@example.com", IsAdmin: 1}).Error)
	admin, err := utils.GenerateToken(1, "root@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	for _, route := range [][2]string{
		{http.MethodGet, "/api/v2/admin/system/backup/config"},
		{http.MethodPut, "/api/v2/admin/system/backup/config"},
		{http.MethodPost, "/api/v2/admin/system/backup"},
		{http.MethodGet, "/api/v2/admin/system/backups"},
		{http.MethodGet, "/api/v2/admin/system/backup/stats"},
		{http.MethodDelete, "/api/v2/admin/system/backups/1"},
		{http.MethodPost, "/api/v2/admin/system/backups/1/restore"},
	} {
		request := httptest.NewRequest(route[0], route[1], nil)
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		assert.Equal(t, http.StatusNotFound, recorder.Code, "%s %s", route[0], route[1])
	}
}

// On PostgreSQL the list says why there are no backups and creating one is
// refused; nothing breaks.
func TestBackupRoutesOnPostgreSQL(t *testing.T) {
	r, cfg := setupTestRouter(t)
	defer teardownTestRouter(t)
	require.NoError(t, database.GetDB().Create(&model.User{ID: 1, Email: "root@example.com", IsAdmin: 1}).Error)
	saved := backup.Default()
	backup.SetDefault(&backup.Manager{Driver: "postgres", DataDir: t.TempDir(), DB: func() *gorm.DB { return database.GetDB() }})
	t.Cleanup(func() { backup.SetDefault(saved) })
	admin, err := utils.GenerateToken(1, "root@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)
	call := func(method, path string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(method, path, nil)
		request.Header.Set("Authorization", "Bearer "+admin)
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, request)
		return recorder
	}
	listed := call(http.MethodGet, "/api/v5/backups")
	require.Equal(t, http.StatusOK, listed.Code)
	assert.Contains(t, listed.Body.String(), `"supported":false`)
	assert.Contains(t, listed.Body.String(), "backups need SQLite")
	created := call(http.MethodPost, "/api/v5/backups")
	assert.Equal(t, http.StatusConflict, created.Code)
	assert.Contains(t, created.Body.String(), "backups_need_sqlite")
	assert.Equal(t, http.StatusConflict, call(http.MethodGet, "/api/v5/backups/1/download").Code)
}
