package native

import (
	"context"
	"time"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/AnixOps/anix-control/sdk/v2compat"
	"github.com/AnixOps/anix-control/v4/packages/subscription/native/model"
)

// Route ids of the protocol pool routes, which read the views of the node
// credential split's remainder.
const (
	GroupProtocolsRouteID     = "subscription.admin.subscription.groups.id.protocols.get"
	AvailableProtocolsRouteID = "subscription.admin.subscription.protocols.available.get"
)

// The views the protocol pool reads. The kernel creates them, and grants
// them to the package's lease, only once v2_node_protocol and v2_node are
// finalized (docs/architecture/node-ops-service.md sections 4.3 and 4.6):
// their JSON columns then hold the redacted documents, never a key.
const (
	nodeProtocolPublicView = "kapi_node_protocol_public_v1"
	nodePublicView         = "kapi_node_public_v1"
)

// PoolProtocol is a row of kapi_node_protocol_public_v1: a node protocol
// with its settings as a finalized v2_node_protocol keeps them, the
// placeholder at every secret position. It answers as the kernel's
// model.NodeProtocol does.
type PoolProtocol struct {
	ID      uint               `gorm:"primaryKey" json:"id"`
	NodeID  uint               `json:"node_id"`
	Name    string             `json:"name"`
	Type    model.ProtocolType `json:"type"`
	Port    int                `json:"port"`
	Enable  int                `json:"enable"`
	Show    int                `json:"show"`
	Sort    int                `json:"sort"`
	GroupID *uint              `json:"group_id"`

	Host *string `json:"host"`
	TLS  int     `json:"tls"`
	ALPN *string `json:"alpn"`

	Settings          *string `json:"settings"`
	TLSSettings       *string `json:"tls_settings"`
	Transport         *string `json:"transport"`
	TransportSettings *string `json:"transport_settings"`

	RealitySettings *string `json:"reality_settings"`

	CustomConfig *string `json:"custom_config"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Node               *PoolNode                 `gorm:"foreignKey:NodeID" json:"node,omitempty"`
	SubscriptionGroups []model.SubscriptionGroup `gorm:"many2many:v2_subscription_group_node_protocols;joinForeignKey:NodeProtocolID;joinReferences:SubscriptionGroupID" json:"subscription_groups,omitempty"`
}

// TableName is the kernel view.
func (PoolProtocol) TableName() string { return nodeProtocolPublicView }

// PoolNode is a row of kapi_node_public_v1: a proxy node without its API
// key, key hash and shared secret, its raw configuration redacted. It
// answers as the kernel's model.Node does, whose credentials are never
// serialized.
type PoolNode struct {
	ID           uint             `gorm:"primaryKey" json:"id"`
	Name         string           `json:"name"`
	Host         string           `json:"host"`
	Port         int              `json:"port"`
	Status       model.NodeStatus `json:"status"`
	Tags         *string          `json:"tags"`
	GroupID      *uint            `json:"group_id"`
	Rate         float64          `json:"rate"`
	TrafficRate  float64          `json:"traffic_rate"`
	Sort         int              `json:"sort"`
	Show         int              `json:"show"`
	AutoRegister int              `json:"auto_register"`

	ParentID *uint `json:"parent_id"`

	MonthlyLimit    *int64 `json:"monthly_limit"`
	MonthlyUpload   int64  `json:"monthly_upload"`
	MonthlyDownload int64  `json:"monthly_download"`
	MonthlyResetDay int    `json:"monthly_reset_day"`

	RawConfig *string `json:"raw_config"`

	ServerIP      *string `json:"server_ip"`
	ServerVersion *string `json:"server_version"`
	ServerOS      *string `json:"server_os"`
	CPUUsage      float64 `json:"cpu_usage"`
	MemoryUsage   float64 `json:"memory_usage"`
	DiskUsage     float64 `json:"disk_usage"`
	Uptime        int64   `json:"uptime"`
	OnlineUsers   int     `json:"online_users"`

	RuntimeHealthy   bool   `json:"runtime_healthy"`
	RuntimeError     string `json:"runtime_error"`
	RuntimeCheckedAt *int64 `json:"runtime_checked_at"`

	TotalUpload   int64 `json:"total_upload"`
	TotalDownload int64 `json:"total_download"`

	LastCheckAt *int64    `json:"last_check_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName is the kernel view.
func (PoolNode) TableName() string { return nodePublicView }

// redact masks what the kernel's administrator answers mask
// (service.RedactNodeProtocol): every secret position of the settings and
// of the node's raw configuration. The views already show the redacted
// documents, so this changes nothing a kernel writer stored; it keeps a
// value written past the kernel's writers out of the answer too.
func (p *PoolProtocol) redact() {
	for _, field := range []**string{&p.Settings, &p.TLSSettings, &p.TransportSettings, &p.RealitySettings, &p.CustomConfig} {
		if *field != nil {
			redacted := v2compat.RedactNodeSecrets(**field)
			*field = &redacted
		}
	}
	if p.Node != nil && p.Node.RawConfig != nil {
		redacted := v2compat.RedactNodeSecrets(*p.Node.RawConfig)
		p.Node.RawConfig = &redacted
	}
}

// poolReady reports whether the package's lease grants both views: false
// until the kernel finalized the two tables, or with a host that has no
// lease, and the route then answers from the legacy handler.
func (s *Service) poolReady(ctx context.Context) bool {
	return s.Leased != nil && s.Leased(ctx, nodeProtocolPublicView) && s.Leased(ctx, nodePublicView)
}

// GroupProtocols is GET /api/v2/admin/subscription/groups/:id/protocols:
// the node protocols linked to a group, each with its node, settings and
// raw configuration redacted.
func (s *Service) GroupProtocols(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	id, ok := pathUint(request, "id")
	if !ok {
		return s.panelError(invalidID)
	}
	if !s.poolReady(ctx) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var protocols []PoolProtocol
	if err := db.Preload("Node").
		Joins("JOIN v2_subscription_group_node_protocols ON v2_subscription_group_node_protocols.node_protocol_id = "+nodeProtocolPublicView+".id").
		Where("v2_subscription_group_node_protocols.subscription_group_id = ?", id).
		Find(&protocols).Error; err != nil {
		return s.panelError(err.Error())
	}
	for i := range protocols {
		protocols[i].redact()
	}
	return s.panel(protocols)
}

// AvailableProtocols is GET /api/v2/admin/subscription/protocols/available:
// the protocol pool, every protocol shown in subscriptions, by node and
// sort order, each with its node and groups, redacted.
func (s *Service) AvailableProtocols(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	if !s.poolReady(ctx) {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	db, err := s.Open(ctx)
	if err != nil {
		return pluginhostsdk.NativeResponse{}, pluginhostsdk.ErrNativeUnavailable
	}
	var protocols []PoolProtocol
	if err := db.Preload("Node").Preload("SubscriptionGroups").
		Where("show = 1").
		Order("node_id ASC, sort ASC").
		Find(&protocols).Error; err != nil {
		return s.panelError(err.Error())
	}
	for i := range protocols {
		protocols[i].redact()
	}
	return s.panel(protocols)
}
