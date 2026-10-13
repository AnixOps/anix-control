package service

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type bucketKey struct {
	server uint
	typ    string
	user   uint
	rate   float64
	logAt  int64
}

type bucketSums struct{ u, d int64 }

func trafficLogBuckets(t *testing.T, db *gorm.DB) map[bucketKey]bucketSums {
	t.Helper()
	var rows []model.TrafficLog
	require.NoError(t, db.Find(&rows).Error)
	out := make(map[bucketKey]bucketSums, len(rows))
	for _, row := range rows {
		key := bucketKey{row.ServerID, row.ServerType, row.UserID, row.Rate, row.LogAt}
		_, exists := out[key]
		require.False(t, exists, "a bucket is one row: %+v", key)
		out[key] = bucketSums{row.U, row.D}
	}
	return out
}

func applyReport(t *testing.T, db *gorm.DB, serverType model.ServerType, node uint, traffics map[uint][2]int64, rate float64, at time.Time) {
	t.Helper()
	plan, err := planNodeTrafficReport(serverType, node, traffics, rate, at)
	require.NoError(t, err)
	require.NoError(t, db.Transaction(plan.applyTx))
}

// A node's reports of one hour are one row per user, key (node, hour, user,
// server type, rate): the bytes add up, another hour, rate or server type is
// another row.
func TestTrafficReportsOfOneHourShareARowPerUser(t *testing.T) {
	_, db := activityService(t)
	hour := time.Date(2026, 10, 13, 6, 0, 0, 0, time.UTC)
	traffics := func(u, d int64) map[uint][2]int64 { return map[uint][2]int64{1: {u, d}, 2: {u * 2, d * 2}} }

	applyReport(t, db, "vmess", 7, traffics(10, 20), 1, hour.Add(1*time.Minute))
	applyReport(t, db, "vmess", 7, traffics(1, 2), 1, hour.Add(59*time.Minute+59*time.Second))
	applyReport(t, db, "vmess", 7, traffics(100, 200), 2, hour.Add(30*time.Minute))
	applyReport(t, db, "trojan", 7, traffics(1000, 2000), 1, hour.Add(30*time.Minute))
	applyReport(t, db, "vmess", 7, traffics(5, 6), 1, hour.Add(time.Hour))

	logAt := hour.Unix()
	require.Equal(t, map[bucketKey]bucketSums{
		{7, "vmess", 1, 1, logAt}:        {11, 22},
		{7, "vmess", 2, 1, logAt}:        {22, 44},
		{7, "vmess", 1, 2, logAt}:        {100, 200},
		{7, "vmess", 2, 2, logAt}:        {200, 400},
		{7, "trojan", 1, 1, logAt}:       {1000, 2000},
		{7, "trojan", 2, 1, logAt}:       {2000, 4000},
		{7, "vmess", 1, 1, logAt + 3600}: {5, 6},
		{7, "vmess", 2, 1, logAt + 3600}: {10, 12},
	}, trafficLogBuckets(t, db))

	// The counters of the users and of the node are as without buckets.
	var user model.User
	require.NoError(t, db.First(&user, 1).Error)
	require.Equal(t, int64(10+1+100*2+1000+5), user.U)
}

// Random reports through the writers leave, per bucket key, the sums that
// one row per report would have: the same totals for every reader.
func TestTrafficLogBucketsEqualPerReportRows(t *testing.T) {
	requireBucketsEqualPerReportRows(t, func(t *testing.T) *gorm.DB {
		_, db := activityService(t)
		return db
	}, 6)
}

func requireBucketsEqualPerReportRows(t *testing.T, open func(*testing.T) *gorm.DB, seeds int64) {
	for seed := int64(1); seed <= seeds; seed++ {
		t.Run(fmt.Sprintf("seed %d", seed), func(t *testing.T) {
			random := rand.New(rand.NewSource(seed))
			db := open(t)
			rates := []float64{1, 1, 1.5, 2}
			types := []model.ServerType{"vmess", "trojan", "node"}
			base := time.Date(2026, 10, 13, 0, 0, 0, 0, time.UTC)
			want := map[bucketKey]bucketSums{}
			for report := 0; report < 80; report++ {
				at := base.Add(time.Duration(random.Intn(5*3600)) * time.Second)
				node := uint(7 + random.Intn(2))
				typ := types[random.Intn(len(types))]
				rate := rates[random.Intn(len(rates))]
				traffics := map[uint][2]int64{}
				for user := uint(1); user <= 3; user++ {
					if random.Intn(3) > 0 {
						traffics[user] = [2]int64{random.Int63n(1 << 20), random.Int63n(1 << 20)}
					}
				}
				if len(traffics) == 0 {
					continue
				}
				applyReport(t, db, typ, node, traffics, rate, at)
				hour := model.TrafficLogBucketStart(at.Unix())
				for user, traffic := range traffics {
					key := bucketKey{node, string(typ), user, rate, hour}
					sums := want[key]
					want[key] = bucketSums{sums.u + traffic[0], sums.d + traffic[1]}
				}
			}
			require.Equal(t, want, trafficLogBuckets(t, db))
		})
	}
}

func TestBucketStartRoundsDownToTheUTCHour(t *testing.T) {
	require.Equal(t, int64(0), model.TrafficLogBucketStart(0))
	require.Equal(t, int64(0), model.TrafficLogBucketStart(3599))
	require.Equal(t, int64(3600), model.TrafficLogBucketStart(3600))
	require.Equal(t, int64(1_791_849_600), model.TrafficLogBucketStart(1_791_849_600+1234))
	require.Equal(t, int64(-3600), model.TrafficLogBucketStart(-1))
}

// The latest report time of the traffic meta is precise for buckets: the
// hour's row keeps the time of its latest report in created_at.
func TestTrafficLogMetaReadsTheLatestReportOfABucket(t *testing.T) {
	_, db := activityService(t)
	stats := &StatsService{db: db}
	hour := time.Now().Truncate(time.Hour)

	meta, err := stats.GetTrafficLogMeta(0)
	require.NoError(t, err)
	require.Zero(t, meta.LatestLogAt, "never reported")

	// Two reports of one hour: the second is 7 minutes after the first.
	first := hour.Add(time.Second)
	second := hour.Add(7*time.Minute + 30*time.Second)
	applyReportAt := func(user uint, at time.Time) {
		plan, err := planNodeTrafficReport("node", 7, map[uint][2]int64{user: {1, 1}}, 1, at)
		require.NoError(t, err)
		for i := range plan.logs {
			plan.logs[i].CreatedAt = at
		}
		require.NoError(t, db.Transaction(plan.applyTx))
	}
	applyReportAt(1, first)
	applyReportAt(1, second)
	applyReportAt(2, first.Add(time.Minute))

	meta, err = stats.GetTrafficLogMeta(0)
	require.NoError(t, err)
	require.Equal(t, second.Unix(), meta.LatestLogAt, "not the hour's start")
	meta, err = stats.GetTrafficLogMeta(2)
	require.NoError(t, err)
	require.Equal(t, first.Add(time.Minute).Unix(), meta.LatestLogAt, "per user")

	// A row that is not on an hour boundary (written by an older version)
	// is its own time.
	require.NoError(t, db.Create(&model.TrafficLog{UserID: 3, ServerID: 7, ServerType: "node", U: 1, D: 1, Rate: 1, LogAt: hour.Unix() + 59}).Error)
	meta, err = stats.GetTrafficLogMeta(3)
	require.NoError(t, err)
	require.Equal(t, hour.Unix()+59, meta.LatestLogAt)
}
