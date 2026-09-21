package service

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type maintenanceProcessSender struct{ path string }

func (s maintenanceProcessSender) Send(_ context.Context, d model.MaintenanceDelivery) error {
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(d.Channel + "\n")
	return err
}
func TestMaintenanceProcessWorkerHelper(t *testing.T) {
	path := os.Getenv("ANIXOPS_MAINTENANCE_PROCESS_DB")
	if path == "" {
		t.Skip("subprocess helper")
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec("PRAGMA busy_timeout = 5000").Error)
	_, err = ProcessMaintenanceEvents(db, 50, time.Now())
	require.NoError(t, err)
	_, err = DeliverMaintenanceNotifications(context.Background(), db, maintenanceProcessSender{path: os.Getenv("ANIXOPS_MAINTENANCE_PROCESS_SENT")}, time.Now(), 50)
	require.NoError(t, err)
}
func TestMaintenanceCrossProcessWorkerClaims(t *testing.T) {
	db := maintenanceDB(t)
	now := time.Now().UTC()
	seedMaintenanceSettings(t, db, now, false)
	event := maintenanceEvent("process-fault", now)
	require.True(t, ReceiveMaintenanceEvents(db, 1, maintenanceBatch(t, event), now).Events[0].Persisted)
	type databaseInfo struct{ File string }
	var info databaseInfo
	require.NoError(t, db.Raw("PRAGMA database_list").Scan(&info).Error)
	sent := filepath.Join(t.TempDir(), "sent.log")
	var wg sync.WaitGroup
	outputs := make([][]byte, 2)
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestMaintenanceProcessWorkerHelper$")
			cmd.Env = append(os.Environ(), "ANIXOPS_MAINTENANCE_PROCESS_DB="+info.File, "ANIXOPS_MAINTENANCE_PROCESS_SENT="+sent)
			outputs[index], errs[index] = cmd.CombinedOutput()
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "helper: %s", outputs[i])
	}
	require.EqualValues(t, 1, ticketCount(t, db))
	data, err := os.ReadFile(sent)
	require.NoError(t, err)
	require.Equal(t, 1, strings.Count(string(data), "email\n"))
	require.Equal(t, 1, strings.Count(string(data), "telegram\n"))
}
