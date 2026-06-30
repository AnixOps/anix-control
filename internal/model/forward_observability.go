package model

import "time"

// LatencyTargetType 延迟探测目标类型
const (
	LatencyTargetTypeForward     = "forward"      // forward -> remote_addr 目标
	LatencyTargetTypeTunnelNode  = "tunnel_node"  // 隧道中继/出口节点端点
	LatencyTargetTypeForwardNode = "forward_node" // ForwardNode 管理端点 (HealthCheck)
	LatencyTargetTypeNode        = "node"         // 常规代理节点 v2_node (V2bX 注册)
)

// ForwardLatencyBucket 一个探测目标在一个时间桶内的预聚合 TCPing 结果。
// 每行 = 一个 (目标 × 时间桶)，不存原始单次样本。
type ForwardLatencyBucket struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// 跨三类目标的统一键: "<type>:<id>:<host>:<port>"
	TargetKey  string `gorm:"size:191;not null;uniqueIndex:idx_flb_key_time,priority:1;index:idx_flb_key_latest,priority:1" json:"target_key"`
	TargetType string `gorm:"size:20;not null;index" json:"target_type"`
	TargetID   uint   `gorm:"index" json:"target_id"`
	Label      string `gorm:"size:191" json:"label"`
	Host       string `gorm:"size:255" json:"host"`
	Port       int    `json:"port"`

	BucketAt        time.Time `gorm:"not null;uniqueIndex:idx_flb_key_time,priority:2;index:idx_flb_key_latest,priority:2,sort:desc;index:idx_flb_time" json:"bucket_at"`
	IntervalSeconds int       `gorm:"not null;default:60" json:"interval_seconds"`

	SampleCount  int     `gorm:"not null" json:"sample_count"`
	SuccessCount int     `gorm:"not null" json:"success_count"`
	MinRTT       float64 `json:"min_rtt"`  // ms
	AvgRTT       float64 `json:"avg_rtt"`  // ms
	MaxRTT       float64 `json:"max_rtt"`  // ms
	P95RTT       float64 `json:"p95_rtt"`  // ms
	LossPct      float64 `json:"loss_pct"` // 0..100

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (ForwardLatencyBucket) TableName() string {
	return "v2_forward_latency_bucket"
}
