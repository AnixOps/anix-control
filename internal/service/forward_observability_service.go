package service

import (
	"errors"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// ForwardObservabilityService serves read-only latency/topology queries backed by
// the v2_forward_latency_bucket time-series written by ForwardLatencyProber.
type ForwardObservabilityService struct {
	db *gorm.DB
}

func NewForwardObservabilityService(db *gorm.DB) *ForwardObservabilityService {
	if db == nil {
		db = database.Get()
	}
	return &ForwardObservabilityService{db: db}
}

// ---- latency trend ----

type LatencyPoint struct {
	BucketAt int64   `json:"bucketAt"` // unix milli
	Min      float64 `json:"min"`
	Avg      float64 `json:"avg"`
	Max      float64 `json:"max"`
	P95      float64 `json:"p95"`
	Loss     float64 `json:"loss"`
	Samples  int     `json:"samples"`
	Success  int     `json:"success"`
}

type LatencyTrend struct {
	TargetKey       string         `json:"targetKey"`
	Label           string         `json:"label"`
	Host            string         `json:"host"`
	Port            int            `json:"port"`
	IntervalSeconds int            `json:"intervalSeconds"`
	Points          []LatencyPoint `json:"points"`
}

// ---- target catalog ----

type TargetCatalogItem struct {
	TargetKey      string  `json:"targetKey"`
	TargetType     string  `json:"targetType"`
	TargetID       uint    `json:"targetId"`
	Label          string  `json:"label"`
	Host           string  `json:"host"`
	Port           int     `json:"port"`
	LatestBucketAt int64   `json:"latestBucketAt"`
	LatestAvgRTT   float64 `json:"latestAvgRtt"`
	LatestLoss     float64 `json:"latestLoss"`
	Online         bool    `json:"online"`
}

// latestBucketByTarget returns the most recent bucket row for each target_key.
// Uses MAX(id) per target (id is monotonic with bucket_at given the write pattern:
// a later cycle always produces a higher id), avoiding a time round-trip through SQL.
func (s *ForwardObservabilityService) latestBucketByTarget() (map[string]model.ForwardLatencyBucket, error) {
	var ids []uint
	if err := s.db.Model(&model.ForwardLatencyBucket{}).
		Select("MAX(id)").
		Group("target_key").
		Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[string]model.ForwardLatencyBucket{}, nil
	}

	var rows []model.ForwardLatencyBucket
	if err := s.db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]model.ForwardLatencyBucket, len(rows))
	for i := range rows {
		result[rows[i].TargetKey] = rows[i]
	}
	return result, nil
}

// ---- topology (G6 shape) ----

type TopologyNode struct {
	ID          string  `json:"id"` // "node-<id>"
	Label       string  `json:"label"`
	Kind        string  `json:"kind"` // relay | exit
	Region      string  `json:"region"`
	Online      bool    `json:"online"`
	Status      int     `json:"status"`
	LatencyMs   int     `json:"latencyMs"`
	Load        float64 `json:"load"`
	CurrentConn int     `json:"currentConn"`
	Weight      int     `json:"weight"`
}

type TopologyEdge struct {
	ID           string  `json:"id"` // "tunnel-<id>"
	Source       string  `json:"source"`
	Target       string  `json:"target"`
	Label        string  `json:"label"`
	TunnelID     uint    `json:"tunnelId"`
	ForwardCount int     `json:"forwardCount"`
	Status       int     `json:"status"`
	AvgRTT       float64 `json:"avgRtt"`
}

type Topology struct {
	Nodes []TopologyNode `json:"nodes"`
	Edges []TopologyEdge `json:"edges"`
}

// ---- multi-ingress latency ----

type MultiIngressRow struct {
	TunnelID      uint    `json:"tunnelId"`
	TunnelName    string  `json:"tunnelName"`
	IngressNodeID uint    `json:"ingressNodeId"`
	IngressLabel  string  `json:"ingressLabel"`
	IngressIP     string  `json:"ingressIp"`
	AvgRTT        float64 `json:"avgRtt"`
	Loss          float64 `json:"loss"`
	BucketAt      int64   `json:"bucketAt"`
	Online        bool    `json:"online"`
}

// GetMultiIngressLatency lists, for the forward's exit target, the latest ingress
// latency of every tunnel reaching the same exit node (incl. the forward's own tunnel).
func (s *ForwardObservabilityService) GetMultiIngressLatency(forwardID uint) ([]MultiIngressRow, error) {
	if forwardID == 0 {
		return nil, errors.New("targetId 不能为空")
	}
	var forward model.Forward
	if err := s.db.First(&forward, forwardID).Error; err != nil {
		return nil, err
	}
	var ownTunnel model.ForwardTunnel
	if err := s.db.First(&ownTunnel, forward.TunnelID).Error; err != nil {
		return nil, err
	}

	// sibling tunnels share the same exit node; if no exit node, just the own tunnel.
	var tunnels []model.ForwardTunnel
	query := s.db.Order("id ASC")
	if ownTunnel.OutNodeID != nil {
		query = query.Where("out_node_id = ?", *ownTunnel.OutNodeID)
	} else {
		query = query.Where("id = ?", ownTunnel.ID)
	}
	if err := query.Find(&tunnels).Error; err != nil {
		return nil, err
	}

	latest, err := s.latestBucketByTarget()
	if err != nil {
		return nil, err
	}
	ingressBucket := func(nodeID uint) (model.ForwardLatencyBucket, bool) {
		for _, b := range latest {
			if b.TargetType == model.LatencyTargetTypeTunnelNode && b.TargetID == nodeID {
				return b, true
			}
		}
		return model.ForwardLatencyBucket{}, false
	}

	rows := make([]MultiIngressRow, 0, len(tunnels))
	for i := range tunnels {
		t := &tunnels[i]
		row := MultiIngressRow{
			TunnelID:      t.ID,
			TunnelName:    t.Name,
			IngressNodeID: t.InNodeID,
			IngressIP:     t.InIP,
		}
		if b, ok := ingressBucket(t.InNodeID); ok {
			row.IngressLabel = b.Label
			row.AvgRTT = b.AvgRTT
			row.Loss = b.LossPct
			row.BucketAt = b.BucketAt.UnixMilli()
			row.Online = b.SuccessCount > 0
		}
		rows = append(rows, row)
	}
	return rows, nil
}
