package service

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
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

func (s *ForwardObservabilityService) GetLatencyTrend(targetKey string, from, to time.Time) (*LatencyTrend, error) {
	targetKey = strings.TrimSpace(targetKey)
	if targetKey == "" {
		return nil, errors.New("targetKey 不能为空")
	}
	if to.IsZero() {
		to = time.Now()
	}
	if from.IsZero() {
		from = to.Add(-time.Hour)
	}
	isAllowed, err := s.isAllowedLatencyTrendTarget(targetKey)
	if err != nil {
		return nil, err
	}
	if !isAllowed {
		return &LatencyTrend{TargetKey: targetKey, Points: []LatencyPoint{}}, nil
	}

	var rows []model.ForwardLatencyBucket
	if err := s.db.
		Where("target_key = ? AND bucket_at BETWEEN ? AND ?", targetKey, from, to).
		Order("bucket_at ASC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	trend := &LatencyTrend{TargetKey: targetKey, Points: make([]LatencyPoint, 0, len(rows))}
	for i := range rows {
		r := &rows[i]
		if i == 0 {
			trend.Label = r.Label
			trend.Host = r.Host
			trend.Port = r.Port
			trend.IntervalSeconds = r.IntervalSeconds
		}
		trend.Points = append(trend.Points, LatencyPoint{
			BucketAt: r.BucketAt.UnixMilli(),
			Min:      r.MinRTT,
			Avg:      r.AvgRTT,
			Max:      r.MaxRTT,
			P95:      r.P95RTT,
			Loss:     r.LossPct,
			Samples:  r.SampleCount,
			Success:  r.SuccessCount,
		})
	}
	return trend, nil
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

func (s *ForwardObservabilityService) ListTargets() ([]TargetCatalogItem, error) {
	latest, err := s.latestBucketByTarget()
	if err != nil {
		return nil, err
	}
	validNodeIDs, err := s.activeParentProxyNodeIDSet()
	if err != nil {
		return nil, err
	}
	items := make([]TargetCatalogItem, 0, len(latest))
	for _, b := range latest {
		if b.TargetType == model.LatencyTargetTypeNode {
			if !validNodeIDs[b.TargetID] {
				continue
			}
		}
		items = append(items, TargetCatalogItem{
			TargetKey:      b.TargetKey,
			TargetType:     b.TargetType,
			TargetID:       b.TargetID,
			Label:          b.Label,
			Host:           b.Host,
			Port:           b.Port,
			LatestBucketAt: b.BucketAt.UnixMilli(),
			LatestAvgRTT:   b.AvgRTT,
			LatestLoss:     b.LossPct,
			Online:         b.SuccessCount > 0,
		})
	}
	return items, nil
}

func (s *ForwardObservabilityService) activeParentProxyNodeIDSet() (map[uint]bool, error) {
	var ids []uint
	if err := s.db.Model(&model.Node{}).
		Where("status <> ? AND parent_id IS NULL", model.NodeStatusDisabled).
		Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out, nil
}

func (s *ForwardObservabilityService) isAllowedLatencyTrendTarget(targetKey string) (bool, error) {
	targetType, targetID, ok := parseLatencyTargetKey(targetKey)
	if !ok || targetType != model.LatencyTargetTypeNode {
		return false, nil
	}

	var count int64
	if err := s.db.Model(&model.Node{}).
		Where("id = ? AND status <> ? AND parent_id IS NULL", targetID, model.NodeStatusDisabled).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func parseLatencyTargetKey(targetKey string) (string, uint, bool) {
	parts := strings.SplitN(targetKey, ":", 3)
	if len(parts) < 2 {
		return "", 0, false
	}
	id, err := strconv.ParseUint(parts[1], 10, 32)
	if err != nil {
		return "", 0, false
	}
	return parts[0], uint(id), true
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

func (s *ForwardObservabilityService) GetTopology() (*Topology, error) {
	var nodes []model.ForwardNode
	if err := s.db.
		Where("type IN ?", []string{model.ForwardNodeTypeRelay, model.ForwardNodeTypeExit}).
		Order("id ASC").
		Find(&nodes).Error; err != nil {
		return nil, err
	}

	latest, err := s.latestBucketByTarget()
	if err != nil {
		return nil, err
	}
	nodeBucket := func(id uint) (model.ForwardLatencyBucket, bool) {
		for _, b := range latest {
			if b.TargetType == model.LatencyTargetTypeForwardNode && b.TargetID == id {
				return b, true
			}
		}
		return model.ForwardLatencyBucket{}, false
	}
	proxyBucket := func(id uint) (model.ForwardLatencyBucket, bool) {
		for _, b := range latest {
			if b.TargetType == model.LatencyTargetTypeNode && b.TargetID == id {
				return b, true
			}
		}
		return model.ForwardLatencyBucket{}, false
	}

	topo := &Topology{
		Nodes: make([]TopologyNode, 0, len(nodes)),
		Edges: make([]TopologyEdge, 0),
	}
	for i := range nodes {
		n := &nodes[i]
		online := n.Status == model.ForwardNodeStatusOnline
		latency := n.Latency
		if b, ok := nodeBucket(n.ID); ok {
			online = b.SuccessCount > 0
			latency = int(math.Round(b.AvgRTT))
		}
		topo.Nodes = append(topo.Nodes, TopologyNode{
			ID:          fmt.Sprintf("node-%d", n.ID),
			Label:       n.Name,
			Kind:        n.Type,
			Region:      n.Region,
			Online:      online,
			Status:      n.Status,
			LatencyMs:   latency,
			Load:        n.Load,
			CurrentConn: n.CurrentConn,
			Weight:      n.Weight,
		})
	}

	// regular proxy nodes (v2_node, registered by V2bX) shown as standalone nodes
	var proxyNodes []model.Node
	if err := s.db.
		Where("status <> ?", model.NodeStatusDisabled).
		Order("id ASC").
		Find(&proxyNodes).Error; err != nil {
		return nil, err
	}
	for i := range proxyNodes {
		n := &proxyNodes[i]
		online := n.Status == model.NodeStatusOnline
		latency := 0
		if b, ok := proxyBucket(n.ID); ok {
			online = b.SuccessCount > 0
			latency = int(math.Round(b.AvgRTT))
		}
		topo.Nodes = append(topo.Nodes, TopologyNode{
			ID:          fmt.Sprintf("proxy-%d", n.ID),
			Label:       n.Name,
			Kind:        model.LatencyTargetTypeNode,
			Online:      online,
			Status:      int(n.Status),
			LatencyMs:   latency,
			Load:        n.CPUUsage / 100,
			CurrentConn: n.OnlineUsers,
		})
	}

	var tunnels []model.ForwardTunnel
	if err := s.db.
		Where("status = ?", model.ForwardTunnelStatusActive).
		Order("id ASC").
		Find(&tunnels).Error; err != nil {
		return nil, err
	}
	forwardCounts, err := s.forwardCountByTunnel()
	if err != nil {
		return nil, err
	}
	for i := range tunnels {
		t := &tunnels[i]
		if t.OutNodeID == nil {
			continue // single-ended tunnel has no edge
		}
		avgRTT := 0.0
		for _, b := range latest {
			if b.TargetType == model.LatencyTargetTypeTunnelNode && b.TargetID == t.InNodeID {
				avgRTT = b.AvgRTT
				break
			}
		}
		topo.Edges = append(topo.Edges, TopologyEdge{
			ID:           fmt.Sprintf("tunnel-%d", t.ID),
			Source:       fmt.Sprintf("node-%d", t.InNodeID),
			Target:       fmt.Sprintf("node-%d", *t.OutNodeID),
			Label:        t.Name,
			TunnelID:     t.ID,
			ForwardCount: forwardCounts[t.ID],
			Status:       t.Status,
			AvgRTT:       avgRTT,
		})
	}
	return topo, nil
}

func (s *ForwardObservabilityService) forwardCountByTunnel() (map[uint]int, error) {
	type row struct {
		TunnelID uint
		Cnt      int
	}
	var rows []row
	if err := s.db.Model(&model.Forward{}).
		Select("tunnel_id, COUNT(*) AS cnt").
		Group("tunnel_id").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[uint]int, len(rows))
	for _, r := range rows {
		out[r.TunnelID] = r.Cnt
	}
	return out, nil
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
