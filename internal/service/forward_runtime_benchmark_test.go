package service

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	forwardRuntimeBenchmarkHistoryJobs = 6000
	forwardRuntimeBenchmarkActiveJobs  = 600
	forwardRuntimeBenchmarkClaimBatch  = 10
)

func BenchmarkForwardRuntimeJobCleanAgentClaiming(b *testing.B) {
	db := setupForwardRuntimeBenchmarkDB(
		b,
		&model.User{},
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.Forward{},
		&model.ForwardRuntimeJob{},
		&model.ForwardCleanAgent{},
	)
	agentID, token, jobIDs := seedForwardRuntimeJobClaimBenchmark(b, db)
	svc := NewForwardCleanAgentService(db)
	input := ForwardCleanAgentHeartbeatInput{
		AgentID: agentID,
		Token:   token,
		Version: "bench",
		Limit:   forwardRuntimeBenchmarkClaimBatch,
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		resetForwardRuntimeClaimBenchmarkJobs(b, db, jobIDs)
		b.StartTimer()

		actions, err := svc.Heartbeat(input)

		b.StopTimer()
		if err != nil {
			b.Fatal(err)
		}
		if len(actions) != forwardRuntimeBenchmarkClaimBatch {
			b.Fatalf("claimed actions = %d, want %d", len(actions), forwardRuntimeBenchmarkClaimBatch)
		}
	}
}

func setupForwardRuntimeBenchmarkDB(b *testing.B, models ...any) *gorm.DB {
	b.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(b.TempDir(), "forward-runtime-benchmark.db")), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		b.Fatal(err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if len(models) > 0 {
		if err := db.AutoMigrate(models...); err != nil {
			b.Fatal(err)
		}
	}
	if err := EnsureForwardRuntimeJobSchema(db); err != nil {
		b.Fatal(err)
	}

	return db
}

func seedForwardRuntimeJobClaimBenchmark(b *testing.B, db *gorm.DB) (uint, string, []uint) {
	b.Helper()

	user := &model.User{
		Email:          "forward-runtime-claim-bench@example.com",
		Token:          "forward-runtime-claim-bench-token",
		UUID:           "forward-runtime-claim-bench-uuid",
		TransferEnable: 1 << 40,
	}
	if err := db.Create(user).Error; err != nil {
		b.Fatal(err)
	}

	node := &model.ForwardNode{
		Name:    "forward-runtime-claim-bench-node",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "203.0.113.10",
		Port:    22,
		Enabled: true,
	}
	if err := db.Create(node).Error; err != nil {
		b.Fatal(err)
	}

	tunnel := &model.ForwardTunnel{
		Name:     "forward-runtime-claim-bench-tunnel",
		Type:     1,
		InNodeID: node.ID,
		OutNodeID: func() *uint {
			id := node.ID
			return &id
		}(),
		InIP:   node.Host,
		OutIP:  node.Host,
		Status: model.ForwardTunnelStatusActive,
	}
	if err := db.Create(tunnel).Error; err != nil {
		b.Fatal(err)
	}

	token := "forward-runtime-claim-bench-token"
	agent := &model.ForwardCleanAgent{
		NodeID: &node.ID,
		Name:   "forward-runtime-claim-bench-agent",
		Token:  token,
		Status: model.ForwardCleanAgentStatusOnline,
	}
	if err := db.Create(agent).Error; err != nil {
		b.Fatal(err)
	}

	jobIDs := make([]uint, 0, forwardRuntimeBenchmarkClaimBatch)
	for i := 0; i < forwardRuntimeBenchmarkClaimBatch; i++ {
		forward := &model.Forward{
			UserID:         user.ID,
			UserName:       user.Email,
			Name:           fmt.Sprintf("forward-runtime-claim-bench-%02d", i),
			TunnelID:       tunnel.ID,
			InPort:         21000 + i,
			RemoteAddr:     "127.0.0.1:8080",
			Status:         model.ForwardStatusActive,
			RuntimeBackend: model.ForwardRuntimeBackendCleanAgent,
		}
		if err := db.Create(forward).Error; err != nil {
			b.Fatal(err)
		}

		job := &model.ForwardRuntimeJob{
			Backend:      model.ForwardRuntimeBackendCleanAgent,
			Action:       model.ForwardRuntimeJobActionUpdate,
			ResourceType: nodeXForwardResourceTypePanelForward,
			ResourceID:   uintPtr(forward.ID),
			ForwardID:    uintPtr(forward.ID),
			TunnelID:     uintPtr(tunnel.ID),
			NodeID:       uintPtr(node.ID),
			Status:       model.ForwardRuntimeJobStatusPending,
			Payload:      fmt.Sprintf(`{"forwardId":%d}`, forward.ID),
		}
		if err := db.Create(job).Error; err != nil {
			b.Fatal(err)
		}
		jobIDs = append(jobIDs, job.ID)
	}

	return agent.ID, token, jobIDs
}

func resetForwardRuntimeClaimBenchmarkJobs(b *testing.B, db *gorm.DB, jobIDs []uint) {
	b.Helper()

	if err := db.Model(&model.ForwardRuntimeJob{}).
		Where("id IN ?", jobIDs).
		Updates(map[string]any{
			"status":       model.ForwardRuntimeJobStatusPending,
			"agent_id":     nil,
			"started_at":   nil,
			"claimed_at":   nil,
			"completed_at": nil,
			"result":       "",
			"error":        "",
		}).Error; err != nil {
		b.Fatal(err)
	}
}
