package native

import (
	"context"
	"strconv"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"gorm.io/gorm"
)

// multiIngressRow is the legacy MultiIngressRow.
type multiIngressRow struct {
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

// MultiIngressLatency is GET /api/v2/admin/forward/observability/multi-ingress:
// for a forward, the latest ingress latency of every tunnel that reaches the
// same exit node, from the latency prober's buckets.
func (s *Service) MultiIngressLatency(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var forwardID uint
	if raw := query(request, "targetId"); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 32); err == nil {
			forwardID = uint(parsed)
		}
	}
	if forwardID == 0 {
		return s.panelError("targetId 不能为空")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	rows, err := multiIngressLatency(db, forwardID)
	if err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(map[string]any{"list": rows})
}

func multiIngressLatency(db *gorm.DB, forwardID uint) ([]multiIngressRow, error) {
	var forward Forward
	if err := db.First(&forward, forwardID).Error; err != nil {
		return nil, err
	}
	var ownTunnel Tunnel
	if err := db.First(&ownTunnel, forward.TunnelID).Error; err != nil {
		return nil, err
	}
	var tunnels []Tunnel
	query := db.Order("id ASC")
	if ownTunnel.OutNodeID != nil {
		query = query.Where("out_node_id = ?", *ownTunnel.OutNodeID)
	} else {
		query = query.Where("id = ?", ownTunnel.ID)
	}
	if err := query.Find(&tunnels).Error; err != nil {
		return nil, err
	}
	latest, err := latestBucketByTarget(db)
	if err != nil {
		return nil, err
	}
	rows := make([]multiIngressRow, 0, len(tunnels))
	for i := range tunnels {
		tunnel := &tunnels[i]
		row := multiIngressRow{TunnelID: tunnel.ID, TunnelName: tunnel.Name, IngressNodeID: tunnel.InNodeID, IngressIP: tunnel.InIP}
		// The kernel takes the first matching bucket of a map, so with
		// several tunnel_node buckets for one node its pick is arbitrary.
		for _, bucket := range latest {
			if bucket.TargetType == latencyTargetTunnelNode && bucket.TargetID == tunnel.InNodeID {
				row.IngressLabel = bucket.Label
				row.AvgRTT = bucket.AvgRTT
				row.Loss = bucket.LossPct
				row.BucketAt = bucket.BucketAt.UnixMilli()
				row.Online = bucket.SuccessCount > 0
				break
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// latestBucketByTarget is the newest bucket of each target, the one with
// the highest id.
func latestBucketByTarget(db *gorm.DB) (map[string]LatencyBucket, error) {
	var ids []uint
	if err := db.Model(&LatencyBucket{}).Select("MAX(id)").Group("target_key").Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[string]LatencyBucket{}, nil
	}
	var rows []LatencyBucket
	if err := db.Where("id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make(map[string]LatencyBucket, len(rows))
	for i := range rows {
		result[rows[i].TargetKey] = rows[i]
	}
	return result, nil
}
