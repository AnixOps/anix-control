package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	statsBenchmarkActiveUsers = 250
	statsBenchmarkZeroUsers   = 750
	statsBenchmarkHours       = 168
)

func BenchmarkStatsServiceTrafficQueries(b *testing.B) {
	svc, firstUserID := setupStatsBenchmarkService(b)

	b.Run("hourly_168h_all_users", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rows, err := svc.GetHourlyTraffic(statsBenchmarkHours, 0)
			if err != nil {
				b.Fatal(err)
			}
			if len(rows) != statsBenchmarkHours {
				b.Fatalf("hourly rows = %d, want %d", len(rows), statsBenchmarkHours)
			}
		}
	})

	b.Run("hourly_168h_single_user", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rows, err := svc.GetHourlyTraffic(statsBenchmarkHours, firstUserID)
			if err != nil {
				b.Fatal(err)
			}
			if len(rows) != statsBenchmarkHours {
				b.Fatalf("hourly rows = %d, want %d", len(rows), statsBenchmarkHours)
			}
		}
	})

	b.Run("ranking_168h_top200", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rows, err := svc.GetUserTrafficRanking(statsBenchmarkHours, 500, false)
			if err != nil {
				b.Fatal(err)
			}
			if len(rows) != 200 {
				b.Fatalf("ranking rows = %d, want 200", len(rows))
			}
		}
	})

	b.Run("ranking_168h_include_zero_top1000", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			rows, err := svc.GetUserTrafficRanking(statsBenchmarkHours, 5000, true)
			if err != nil {
				b.Fatal(err)
			}
			if len(rows) != statsBenchmarkActiveUsers+statsBenchmarkZeroUsers {
				b.Fatalf("ranking rows = %d, want %d", len(rows), statsBenchmarkActiveUsers+statsBenchmarkZeroUsers)
			}
		}
	})
}

func setupStatsBenchmarkService(b *testing.B) (*StatsService, uint) {
	b.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(b.TempDir(), "stats-benchmark.db")), &gorm.Config{
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

	if err := db.AutoMigrate(&model.User{}, &model.TrafficLog{}); err != nil {
		b.Fatal(err)
	}

	users := make([]model.User, 0, statsBenchmarkActiveUsers+statsBenchmarkZeroUsers)
	for i := 0; i < statsBenchmarkActiveUsers; i++ {
		users = append(users, model.User{
			Email: fmt.Sprintf("stats-bench-active-%03d@example.com", i),
			Token: fmt.Sprintf("stats-bench-token-a-%03d", i),
			UUID:  fmt.Sprintf("stats-bench-uuid-a-%03d", i),
		})
	}
	for i := 0; i < statsBenchmarkZeroUsers; i++ {
		users = append(users, model.User{
			Email: fmt.Sprintf("stats-bench-zero-%03d@example.com", i),
			Token: fmt.Sprintf("stats-bench-token-z-%03d", i),
			UUID:  fmt.Sprintf("stats-bench-uuid-z-%03d", i),
		})
	}
	if err := db.CreateInBatches(&users, 200).Error; err != nil {
		b.Fatal(err)
	}

	now := time.Now()
	currentHour := time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.Local).Unix()
	logs := make([]model.TrafficLog, 0, statsBenchmarkActiveUsers*statsBenchmarkHours)
	for userIndex := 0; userIndex < statsBenchmarkActiveUsers; userIndex++ {
		userID := users[userIndex].ID
		for hourOffset := 0; hourOffset < statsBenchmarkHours; hourOffset++ {
			hour := currentHour - int64(hourOffset)*hourSeconds
			logs = append(logs, model.TrafficLog{
				UserID:     userID,
				ServerID:   uint(userIndex%10 + 1),
				ServerType: "node",
				U:          int64(1024 + userIndex),
				D:          int64(2048 + hourOffset),
				Rate:       1,
				LogAt:      hour + int64(userIndex%3000),
			})
		}
	}
	if err := db.CreateInBatches(&logs, 1000).Error; err != nil {
		b.Fatal(err)
	}

	return &StatsService{db: db}, users[0].ID
}
