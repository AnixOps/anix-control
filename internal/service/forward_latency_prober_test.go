package service

import (
	"context"
	"testing"
	"time"

	appconfig "github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"
)

func TestAggregateRTTs(t *testing.T) {
	cases := []struct {
		name     string
		rtts     []float64
		attempts int
		wantMin  float64
		wantAvg  float64
		wantMax  float64
		wantP95  float64
		wantLoss float64
	}{
		{
			name:     "all success",
			rtts:     []float64{10, 20, 30, 40, 50},
			attempts: 5,
			wantMin:  10, wantAvg: 30, wantMax: 50, wantP95: 50, wantLoss: 0,
		},
		{
			name:     "partial loss 5 attempts 3 success",
			rtts:     []float64{10, 30, 20},
			attempts: 5,
			wantMin:  10, wantAvg: 20, wantMax: 30, wantP95: 30, wantLoss: 40,
		},
		{
			name:     "all fail",
			rtts:     []float64{},
			attempts: 5,
			wantMin:  0, wantAvg: 0, wantMax: 0, wantP95: 0, wantLoss: 100,
		},
		{
			name:     "single sample",
			rtts:     []float64{42},
			attempts: 1,
			wantMin:  42, wantAvg: 42, wantMax: 42, wantP95: 42, wantLoss: 0,
		},
		{
			name:     "zero attempts",
			rtts:     nil,
			attempts: 0,
			wantMin:  0, wantAvg: 0, wantMax: 0, wantP95: 0, wantLoss: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agg := aggregateRTTs(tc.rtts, tc.attempts)
			assert.InDelta(t, tc.wantMin, agg.Min, 0.001, "min")
			assert.InDelta(t, tc.wantAvg, agg.Avg, 0.001, "avg")
			assert.InDelta(t, tc.wantMax, agg.Max, 0.001, "max")
			assert.InDelta(t, tc.wantP95, agg.P95, 0.001, "p95")
			assert.InDelta(t, tc.wantLoss, agg.Loss, 0.001, "loss")
		})
	}
}

// TestAggregateRTTs_P95NearestRank pins the nearest-rank method: ceil(0.95*n)-1.
func TestAggregateRTTs_P95NearestRank(t *testing.T) {
	rtts := make([]float64, 100)
	for i := range rtts {
		rtts[i] = float64(i + 1) // 1..100
	}
	agg := aggregateRTTs(rtts, 100)
	// ceil(0.95*100)-1 = 94 -> sorted[94] = 95
	assert.InDelta(t, 95, agg.P95, 0.001)
}

type ForwardLatencyProberTestSuite struct {
	ServiceTestSuite
	prober *ForwardLatencyProber
}

func (s *ForwardLatencyProberTestSuite) SetupSuite() {
	s.ServiceTestSuite.SetupSuite()
	s.Require().NoError(database.AutoMigrate(
		&model.ForwardNode{},
		&model.ForwardTunnel{},
		&model.Forward{},
		&model.ForwardLatencyBucket{},
	))
}

func (s *ForwardLatencyProberTestSuite) SetupTest() {
	s.ServiceTestSuite.SetupTest()
	appconfig.Set(nil)
	s.prober = NewForwardLatencyProber(database.Get())
}

func (s *ForwardLatencyProberTestSuite) TestRunOnce_NoTargetsReturnsZero() {
	probed, err := s.prober.runOnce(context.Background())
	assert.NoError(s.T(), err)
	assert.Equal(s.T(), 0, probed)
}

func (s *ForwardLatencyProberTestSuite) TestEnumerateTargets_DedupAndThreeSources() {
	db := database.Get()
	// forward with duplicate + distinct remote targets
	s.Require().NoError(db.Create(&model.Forward{
		Name:       "fwd-a",
		UserID:     1,
		TunnelID:   1,
		InPort:     1000,
		RemoteAddr: "1.1.1.1:80,1.1.1.1:80,2.2.2.2:80",
		Status:     model.ForwardStatusActive,
	}).Error)
	// enabled forward node
	s.Require().NoError(db.Create(&model.ForwardNode{
		Name:    "node-a",
		Type:    model.ForwardNodeTypeRelay,
		Host:    "3.3.3.3",
		Port:    443,
		Enabled: true,
	}).Error)
	// disabled node must be skipped (force the column: GORM treats zero-value
	// false as "unset" and would otherwise apply the default:true).
	s.Require().NoError(db.Create(&model.ForwardNode{
		Name:    "node-disabled",
		Type:    model.ForwardNodeTypeExit,
		Host:    "9.9.9.9",
		Port:    443,
		Enabled: true,
	}).Error)
	s.Require().NoError(db.Model(&model.ForwardNode{}).
		Where("name = ?", "node-disabled").
		Update("enabled", false).Error)
	// active tunnel referencing node-a as ingress
	var relay model.ForwardNode
	s.Require().NoError(db.Where("name = ?", "node-a").First(&relay).Error)
	s.Require().NoError(db.Create(&model.ForwardTunnel{
		Name:     "tun-a",
		InNodeID: relay.ID,
		InIP:     "3.3.3.3",
		Status:   model.ForwardTunnelStatusActive,
	}).Error)
	// regular parent proxy node (v2_node, V2bX-registered) — online
	parentProxy := model.Node{
		Name:   "proxy-a",
		Host:   "4.4.4.4",
		Port:   443,
		APIKey: "probe-test-key-a",
		Status: model.NodeStatusOnline,
	}
	s.Require().NoError(db.Create(&parentProxy).Error)
	// child proxy node must be skipped in the latency trend target set
	childParentID := parentProxy.ID
	childProxy := model.Node{
		Name:     "proxy-child",
		Host:     "5.5.5.5",
		Port:     443,
		APIKey:   "probe-test-key-child",
		Status:   model.NodeStatusOnline,
		ParentID: &childParentID,
	}
	s.Require().NoError(db.Create(&childProxy).Error)
	// disabled proxy node must be skipped
	s.Require().NoError(db.Create(&model.Node{
		Name:   "proxy-disabled",
		Host:   "8.8.8.8",
		Port:   443,
		APIKey: "probe-test-key-disabled",
		Status: model.NodeStatusDisabled,
	}).Error)

	targets, err := s.prober.enumerateTargets(context.Background())
	s.Require().NoError(err)

	byType := map[string]int{}
	for _, t := range targets {
		byType[t.TargetType]++
	}
	// forward: 1.1.1.1:80 (deduped) + 2.2.2.2:80 = 2
	assert.Equal(s.T(), 2, byType[model.LatencyTargetTypeForward])
	// forward_node: only the enabled node = 1
	assert.Equal(s.T(), 1, byType[model.LatencyTargetTypeForwardNode])
	// tunnel_node: ingress endpoint = 1
	assert.Equal(s.T(), 1, byType[model.LatencyTargetTypeTunnelNode])
	// node: only the non-disabled parent proxy node = 1
	assert.Equal(s.T(), 1, byType[model.LatencyTargetTypeNode])
	for _, target := range targets {
		if target.TargetType == model.LatencyTargetTypeNode {
			assert.NotEqual(s.T(), childProxy.ID, target.TargetID)
		}
	}
}

func (s *ForwardLatencyProberTestSuite) TestCleanup_DeletesExpiredBuckets() {
	db := database.Get()
	now := time.Now()
	old := model.ForwardLatencyBucket{
		TargetKey: "forward:1:1.1.1.1:80", TargetType: model.LatencyTargetTypeForward,
		BucketAt: now.Add(-10 * 24 * time.Hour), IntervalSeconds: 60, SampleCount: 5, SuccessCount: 5,
	}
	fresh := model.ForwardLatencyBucket{
		TargetKey: "forward:1:2.2.2.2:80", TargetType: model.LatencyTargetTypeForward,
		BucketAt: now, IntervalSeconds: 60, SampleCount: 5, SuccessCount: 5,
	}
	s.Require().NoError(db.Create(&old).Error)
	s.Require().NoError(db.Create(&fresh).Error)

	s.prober.retention = 7 * 24 * time.Hour
	s.prober.lastCleanup = time.Time{}
	s.prober.cleanup(context.Background())

	var count int64
	s.Require().NoError(db.Model(&model.ForwardLatencyBucket{}).Count(&count).Error)
	assert.Equal(s.T(), int64(1), count)
}

func TestForwardLatencyProber(t *testing.T) {
	suite.Run(t, new(ForwardLatencyProberTestSuite))
}

func TestNewForwardLatencyProber_LoadsSettingsFromConfig(t *testing.T) {
	appconfig.Set(&appconfig.Config{
		ForwardRuntime: appconfig.ForwardRuntimeConfig{
			Latency: appconfig.ForwardRuntimeLatencyConfig{
				PollInterval:     "90s",
				IdlePollInterval: "10m",
				ErrorLogInterval: "3m",
				Dials:            7,
				DialTimeout:      "2s",
				Concurrency:      8,
				RetentionDays:    14,
			},
		},
	})
	defer appconfig.Set(nil)

	prober := NewForwardLatencyProber(&gorm.DB{})
	assert.Equal(t, 90*time.Second, prober.interval)
	assert.Equal(t, 10*time.Minute, prober.idleInterval)
	assert.Equal(t, 7, prober.dials)
	assert.Equal(t, 2*time.Second, prober.dialTimeout)
	assert.Equal(t, 8, prober.concurrency)
	assert.Equal(t, 14*24*time.Hour, prober.retention)
}
