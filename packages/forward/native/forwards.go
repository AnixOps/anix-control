package native

import (
	"context"
	"strings"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// forwardItem is the legacy PanelForwardListItem.
type forwardItem struct {
	ID                  uint   `json:"id"`
	Name                string `json:"name"`
	TunnelID            uint   `json:"tunnelId"`
	TunnelName          string `json:"tunnelName"`
	InIP                string `json:"inIp"`
	InPort              int    `json:"inPort"`
	RemoteAddr          string `json:"remoteAddr"`
	InterfaceName       string `json:"interfaceName"`
	Strategy            string `json:"strategy"`
	Status              int    `json:"status"`
	InFlow              int64  `json:"inFlow"`
	OutFlow             int64  `json:"outFlow"`
	RuntimeBackend      string `json:"runtimeBackend"`
	RuntimeStatus       int    `json:"runtimeStatus"`
	RuntimeMessage      string `json:"runtimeMessage"`
	LastRuntimeSyncTime int64  `json:"lastRuntimeSyncTime"`
	CreatedTime         int64  `json:"createdTime"`
	UpdatedTime         int64  `json:"updatedTime"`
	UserName            string `json:"userName"`
	UserID              uint   `json:"userId"`
	Inx                 int    `json:"inx"`
}

func buildForwardItem(record *Forward) forwardItem {
	item := forwardItem{
		ID: record.ID, Name: record.Name, TunnelID: record.TunnelID, InPort: record.InPort,
		RemoteAddr: record.RemoteAddr, InterfaceName: record.InterfaceName,
		Strategy: normalizeStrategy(record.Strategy, record.RemoteAddr), Status: record.Status,
		InFlow: record.InFlow, OutFlow: record.OutFlow, RuntimeBackend: record.RuntimeBackend,
		RuntimeStatus: record.RuntimeStatus, RuntimeMessage: record.RuntimeMessage,
		CreatedTime: record.CreatedAt.UnixMilli(), UpdatedTime: record.UpdatedAt.UnixMilli(),
		UserName: record.UserName, UserID: record.UserID, Inx: record.Inx,
	}
	if record.RuntimeLastSyncAt != nil {
		item.LastRuntimeSyncTime = record.RuntimeLastSyncAt.UnixMilli()
	}
	if record.Tunnel != nil {
		item.TunnelName = record.Tunnel.Name
		item.InIP = record.Tunnel.InIP
	}
	return item
}

// normalizeRemoteAddr joins the non-empty lines of a target list with
// commas.
func normalizeRemoteAddr(raw string) string {
	lines := strings.Split(raw, "\n")
	items := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		items = append(items, line)
	}
	return strings.Join(items, ",")
}

// normalizeStrategy is the load balancing strategy a forward shows: fifo
// for a single target, else a known strategy or fifo.
func normalizeStrategy(strategy, remoteAddr string) string {
	count := 0
	for _, raw := range strings.Split(normalizeRemoteAddr(remoteAddr), ",") {
		if strings.TrimSpace(raw) != "" {
			count++
		}
	}
	if count <= 1 {
		return "fifo"
	}
	switch strategy {
	case "fifo", "round", "rand", "hash":
		return strategy
	default:
		return "fifo"
	}
}

// ListForwards is POST /api/v2/forward/list (the caller's forwards) and
// POST /api/v2/admin/forward/list; an administrator sees every forward.
func (s *Service) ListForwards(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	userID, isAdmin := actor(request)
	var records []Forward
	query := db.Preload("Tunnel").Order("inx ASC").Order("created_at DESC")
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&records).Error; err != nil {
		return s.panelError(err.Error())
	}
	items := make([]forwardItem, 0, len(records))
	for i := range records {
		items = append(items, buildForwardItem(&records[i]))
	}
	return s.panel(items)
}

// UpdateForwardOrder is POST /api/v2/forward/update-order and its
// administrator route: it sets the display order (inx) of the caller's
// forwards, or of any forward for an administrator. The order is not part
// of what a node runs, so nothing is pushed.
func (s *Service) UpdateForwardOrder(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	var req struct {
		Forwards []struct {
			ID  uint `json:"id"`
			Inx int  `json:"inx"`
		} `json:"forwards" binding:"required"`
	}
	if err := binding.JSON.BindBody(request.Body, &req); err != nil {
		return s.panelError("参数错误")
	}
	if len(req.Forwards) == 0 {
		return s.panelError("forwards 参数不能为空")
	}
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	userID, isAdmin := actor(request)
	ids := make([]uint, 0, len(req.Forwards))
	for _, update := range req.Forwards {
		ids = append(ids, update.ID)
	}
	var records []Forward
	query := db.Where("id IN ?", ids)
	if !isAdmin {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&records).Error; err != nil {
		return s.panelError(err.Error())
	}
	if len(records) != len(ids) {
		return s.panelError("只能更新可访问的转发排序")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, update := range req.Forwards {
			if err := tx.Model(&Forward{}).Where("id = ?", update.ID).Update("inx", update.Inx).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(true)
}
