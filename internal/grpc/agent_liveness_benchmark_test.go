package grpc

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/cache"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"gorm.io/gorm"
)

// BenchmarkAgentHeartbeatTouchSQLite measures the liveness write of the
// Agent Control stream for N nodes that heartbeat every 20 seconds (simulated
// time, the clock of the throttle moves 20 s a round): throttled is the
// stream's touchNode (at most one write per node and
// AgentLivenessWriteInterval), every_heartbeat writes each one as before.
// writes/heartbeat is the number of UPDATE statements per heartbeat and
// p99-us the time of a touch. docs/design/single-binary Appendix B models the
// heartbeat as one write per node and minute.
func BenchmarkAgentHeartbeatTouchSQLite(b *testing.B) {
	for _, nodes := range []int{10, 50, 200} {
		for _, mode := range []struct {
			name     string
			interval time.Duration
		}{
			{"throttled", AgentLivenessWriteInterval},
			{"every_heartbeat", 0},
		} {
			b.Run(fmt.Sprintf("N%d/%s", nodes, mode.name), func(b *testing.B) {
				cache.InitMemory()
				database.Reset()
				if err := database.Init(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(b.TempDir(), "heartbeat.db"), LogLevel: "silent"}); err != nil {
					b.Fatal(err)
				}
				b.Cleanup(func() { _ = database.Close() })
				db := database.GetDB()
				if err := db.AutoMigrate(&model.Node{}, &model.ForwardNode{}); err != nil {
					b.Fatal(err)
				}
				rows := make([]model.Node, nodes)
				for i := range rows {
					rows[i] = model.Node{Name: fmt.Sprintf("node-%d", i+1), Host: "node.example.test"}
				}
				if err := db.CreateInBatches(&rows, 100).Error; err != nil {
					b.Fatal(err)
				}
				var writes atomic.Int64
				if err := db.Callback().Update().After("gorm:update").Register("bench:count_updates", func(*gorm.DB) { writes.Add(1) }); err != nil {
					b.Fatal(err)
				}

				now := time.Unix(1_800_000_000, 0)
				server := &AgentControlGRPCServer{nodeService: service.NewNodeService(), liveness: newLivenessThrottle(mode.interval)}
				server.liveness.now = func() time.Time { return now }
				latencies := make([]time.Duration, 0, b.N)

				b.ReportAllocs()
				b.ResetTimer()
				started := time.Now()
				for i := 0; i < b.N; i++ {
					if i%nodes == 0 {
						now = now.Add(20 * time.Second) // a heartbeat round
					}
					node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(rows[i%nodes].ID)}
					at := time.Now()
					if err := server.touchNode(node); err != nil {
						b.Fatal(err)
					}
					latencies = append(latencies, time.Since(at))
				}
				elapsed := time.Since(started)
				b.StopTimer()
				sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
				b.ReportMetric(float64(writes.Load())/float64(b.N), "writes/heartbeat")
				b.ReportMetric(float64(b.N)/elapsed.Seconds(), "touches/s")
				b.ReportMetric(float64(latencies[min(len(latencies)-1, int(float64(len(latencies))*0.99))].Microseconds()), "p99-us")
			})
		}
	}
}
