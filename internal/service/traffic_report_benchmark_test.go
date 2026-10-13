package service

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// The scales of docs/design/single-binary Appendix B: N nodes report once a
// minute, A users carried traffic in that minute (spread over the nodes, so
// A/N users to a report), U users exist.
var trafficReportScales = []struct {
	name                 string
	nodes, active, users int
}{
	{"N10_A200", 10, 200, 1000},
	{"N50_A1000", 50, 1000, 5000},
	{"N200_A5000", 200, 5000, 10000},
}

type trafficReportFixture struct {
	db      *gorm.DB
	server  *ServerService
	nodes   *NodeService
	perNode [][]uint // the active users of each node
	nodeIDs []uint
}

// newTrafficReportFixture opens a SQLite file with the kernel's connection
// settings (one writer, a read pool, WAL), N nodes, U users, and assigns the
// A active users to the nodes.
func newTrafficReportFixture(b *testing.B, nodes, active, users int) *trafficReportFixture {
	b.Helper()
	db, err := database.Open(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(b.TempDir(), "report.db"), LogLevel: "silent"})
	if err != nil {
		b.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Node{}, &model.User{}, &model.TrafficLog{}, &model.StatServer{},
		&model.SubscriberRequest{}, &model.SubscriberChange{}, &model.UserActivity{}, &model.AgentReportBatch{}); err != nil {
		b.Fatal(err)
	}
	nodeRows := make([]model.Node, nodes)
	for i := range nodeRows {
		nodeRows[i] = model.Node{Name: fmt.Sprintf("node-%d", i+1), Host: "node.example.test", APIKey: fmt.Sprintf("bench-key-%d", i+1)}
	}
	if err := db.CreateInBatches(&nodeRows, 100).Error; err != nil {
		b.Fatal(err)
	}
	userRows := make([]model.User, users)
	for i := range userRows {
		userRows[i] = model.User{Email: fmt.Sprintf("u%d@bench.test", i+1), Token: fmt.Sprintf("token-%d", i+1), UUID: fmt.Sprintf("uuid-%d", i+1), TransferEnable: 1 << 50}
	}
	if err := db.CreateInBatches(&userRows, 500).Error; err != nil {
		b.Fatal(err)
	}
	fixture := &trafficReportFixture{db: db, server: &ServerService{db: db}, nodes: &NodeService{db: db}}
	fixture.perNode = make([][]uint, nodes)
	for _, node := range nodeRows {
		fixture.nodeIDs = append(fixture.nodeIDs, node.ID)
	}
	for i := 0; i < active; i++ {
		node := i % nodes
		fixture.perNode[node] = append(fixture.perNode[node], userRows[i].ID)
	}
	b.Cleanup(func() { _ = database.CloseHandle(db) })
	return fixture
}

// report applies the traffic report of node index n, the way an Agent's
// stream applies it: the batch claim, the buckets, the counters and the node
// in one transaction, then the node's online-user count.
func (f *trafficReportFixture) report(n int, sequence int64, random *rand.Rand) error {
	traffics := make(map[uint][2]int64, len(f.perNode[n]))
	for _, user := range f.perNode[n] {
		traffics[user] = [2]int64{random.Int63n(1 << 24), random.Int63n(1 << 26)}
	}
	node := f.nodeIDs[n]
	if _, err := f.server.RecordAgentTrafficReport("node", node, fmt.Sprintf("bench-%d-%d", n, sequence), traffics, 1); err != nil {
		return err
	}
	return f.nodes.UpdateOnlineUsers(node, len(traffics))
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted))*p+0.999999) - 1
	return sorted[min(max(index, 0), len(sorted)-1)]
}

func reportLatencies(b *testing.B, latencies []time.Duration, elapsed time.Duration) {
	b.Helper()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	b.ReportMetric(float64(len(latencies))/elapsed.Seconds(), "reports/s")
	b.ReportMetric(float64(percentile(latencies, 0.50).Microseconds())/1000, "p50-ms")
	b.ReportMetric(float64(percentile(latencies, 0.99).Microseconds())/1000, "p99-ms")
	b.ReportMetric(float64(latencies[len(latencies)-1].Microseconds())/1000, "max-ms")
}

// paceSpeedup compresses the design's time in the paced benchmark: a node
// reports once a minute in the design and once every 60s/paceSpeedup here.
const paceSpeedup = 20

// pacedRun is how long the paced benchmark runs, whatever b.N is; with b.N of
// 1 (the CI smoke, -benchtime=1x) it runs pacedSmokeRun.
const (
	pacedRun      = 6 * time.Second
	pacedSmokeRun = time.Second
)

// BenchmarkTrafficReportSQLite measures what the single writer of a SQLite
// database sustains for the traffic reports of N nodes with A active users
// (docs/design/single-binary Appendix B). Run it with
//
//	TMPDIR=/path/on/the/database/disk //	go test ./internal/service -run '^$' -bench TrafficReportSQLite -benchtime 3000x
//
// The temporary directory must be on the disk the database will live on: a
// tmpfs has no fsync. The three sub-benchmarks of each scale are:
//
//   - serial: one report at a time. reports/s is the commit rate of the
//     writer for a report of A/N users; p99-ms is the time one report holds
//     the write lock (the design's target is 50 ms).
//   - paced: N nodes report once every 60s/paceSpeedup, each with the
//     heartbeat write and the online-user count a real stream adds, for
//     pacedRun, so the load is the design's times paceSpeedup and below the
//     capacity. p99-ms is the latency a report sees in that load.
//   - concurrent: N nodes report back to back, the way N Agent streams
//     would if they never paused. reports/s is the capacity (the design
//     needs N/60 a second, one report per node and minute); p99-ms is a
//     report's wait and service time with the writers saturated and queued
//     behind each other, which no real load reaches.
func BenchmarkTrafficReportSQLite(b *testing.B) {
	for _, scale := range trafficReportScales {
		b.Run(scale.name, func(b *testing.B) {
			b.Run("serial", func(b *testing.B) {
				fixture := newTrafficReportFixture(b, scale.nodes, scale.active, scale.users)
				random := rand.New(rand.NewSource(1))
				latencies := make([]time.Duration, 0, b.N)
				b.ReportAllocs()
				b.ResetTimer()
				started := time.Now()
				for i := 0; i < b.N; i++ {
					at := time.Now()
					if err := fixture.report(i%scale.nodes, int64(i), random); err != nil {
						b.Fatal(err)
					}
					latencies = append(latencies, time.Since(at))
				}
				elapsed := time.Since(started)
				b.StopTimer()
				reportLatencies(b, latencies, elapsed)
			})
			b.Run("paced", func(b *testing.B) {
				fixture := newTrafficReportFixture(b, scale.nodes, scale.active, scale.users)
				period := time.Minute / paceSpeedup
				var group sync.WaitGroup
				var failed atomic.Int64
				results := make([][]time.Duration, scale.nodes)
				run := pacedRun
				if b.N == 1 {
					run = pacedSmokeRun
				}
				deadline := time.Now().Add(run)
				b.ReportAllocs()
				b.ResetTimer()
				started := time.Now()
				for n := 0; n < scale.nodes; n++ {
					group.Add(1)
					go func() {
						defer group.Done()
						random := rand.New(rand.NewSource(int64(n) + 1))
						// Nodes do not report in step: a random phase.
						next := started.Add(time.Duration(random.Int63n(int64(period))))
						for sequence := int64(0); next.Before(deadline); sequence++ {
							time.Sleep(time.Until(next))
							next = next.Add(period)
							// The heartbeat of the node's stream (once a
							// minute after the liveness throttle), then its
							// traffic report.
							if err := fixture.nodes.UpdateLastCheckAt(fixture.nodeIDs[n]); err != nil {
								failed.Add(1)
							}
							at := time.Now()
							if err := fixture.report(n, sequence, random); err != nil {
								failed.Add(1)
								continue
							}
							results[n] = append(results[n], time.Since(at))
						}
					}()
				}
				group.Wait()
				elapsed := time.Since(started)
				b.StopTimer()
				if failed.Load() > 0 {
					b.Fatalf("%d writes failed", failed.Load())
				}
				var all []time.Duration
				for _, latencies := range results {
					all = append(all, latencies...)
				}
				if len(all) == 0 {
					b.Fatal("no report ran")
				}
				reportLatencies(b, all, elapsed)
			})
			b.Run("concurrent", func(b *testing.B) {
				fixture := newTrafficReportFixture(b, scale.nodes, scale.active, scale.users)
				var next atomic.Int64
				var group sync.WaitGroup
				results := make([][]time.Duration, scale.nodes)
				errs := make(chan error, scale.nodes)
				b.ReportAllocs()
				b.ResetTimer()
				started := time.Now()
				for n := 0; n < scale.nodes; n++ {
					group.Add(1)
					go func() {
						defer group.Done()
						random := rand.New(rand.NewSource(int64(n) + 1))
						for {
							sequence := next.Add(1)
							if sequence > int64(b.N) {
								return
							}
							at := time.Now()
							if err := fixture.report(n, sequence, random); err != nil {
								errs <- err
								return
							}
							results[n] = append(results[n], time.Since(at))
						}
					}()
				}
				group.Wait()
				elapsed := time.Since(started)
				b.StopTimer()
				select {
				case err := <-errs:
					b.Fatal(err)
				default:
				}
				var all []time.Duration
				for _, latencies := range results {
					all = append(all, latencies...)
				}
				reportLatencies(b, all, elapsed)
			})
		})
	}
}

// BenchmarkNodeLivenessWriteSQLite measures the heartbeat path of the node
// table: N nodes touch their row every 20 seconds of simulated time.
// unthrottled is every heartbeat writing, as before AgentLivenessWriteInterval
// (the service's UpdateLastCheckAt); the throttled variant lives with the
// throttle in internal/grpc.
func BenchmarkNodeLivenessWriteSQLite(b *testing.B) {
	for _, scale := range trafficReportScales {
		b.Run(scale.name, func(b *testing.B) {
			fixture := newTrafficReportFixture(b, scale.nodes, 0, 1)
			latencies := make([]time.Duration, 0, b.N)
			b.ReportAllocs()
			b.ResetTimer()
			started := time.Now()
			for i := 0; i < b.N; i++ {
				at := time.Now()
				if err := fixture.nodes.UpdateLastCheckAt(fixture.nodeIDs[i%scale.nodes]); err != nil {
					b.Fatal(err)
				}
				latencies = append(latencies, time.Since(at))
			}
			elapsed := time.Since(started)
			b.StopTimer()
			sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
			b.ReportMetric(float64(len(latencies))/elapsed.Seconds(), "heartbeats/s")
			b.ReportMetric(float64(percentile(latencies, 0.99).Microseconds())/1000, "p99-ms")
		})
	}
}
