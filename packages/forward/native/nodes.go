package native

import (
	"context"

	"github.com/AnixOps/anix-control/sdk/pluginhostsdk"
)

// Stats is GET /api/v2/admin/forward/stats: the enabled relay and exit
// nodes, how many are online, and their traffic, from
// kapi_forward_node_v1.
func (s *Service) Stats(ctx context.Context, _ pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError("failed to get relay nodes")
	}
	var relayNodes, exitNodes []Node
	if err := db.Where("type = ? AND enabled = ?", nodeTypeRelay, true).Find(&relayNodes).Error; err != nil {
		return s.panelError("failed to get relay nodes")
	}
	if err := db.Where("type = ? AND enabled = ?", nodeTypeExit, true).Find(&exitNodes).Error; err != nil {
		return s.panelError("failed to get exit nodes")
	}
	var totalUpload, totalDownload int64
	var onlineRelay, onlineExit int
	for _, node := range relayNodes {
		totalUpload += node.TotalUpload
		totalDownload += node.TotalDownload
		if node.Status == nodeOnline {
			onlineRelay++
		}
	}
	for _, node := range exitNodes {
		totalUpload += node.TotalUpload
		totalDownload += node.TotalDownload
		if node.Status == nodeOnline {
			onlineExit++
		}
	}
	return s.panel(map[string]any{
		"relay_nodes": len(relayNodes), "exit_nodes": len(exitNodes), "online_relay": onlineRelay, "online_exit": onlineExit,
		"total_upload": totalUpload, "total_download": totalDownload,
	})
}

// UserRules is GET /api/v2/user/forward/rules: the caller's forward rules
// with their relay and exit nodes, which kapi_forward_node_v1 shows without
// their API tokens; the answer's api_token is empty, as the kernel's is.
func (s *Service) UserRules(ctx context.Context, request pluginhostsdk.NativeRequest) (pluginhostsdk.NativeResponse, error) {
	db, err := s.Open(ctx)
	if err != nil {
		return s.panelError(err.Error())
	}
	userID, _ := actor(request)
	var rules []*Rule
	if err := db.Where("user_id = ?", userID).Preload("RelayNode").Preload("ExitNode").Find(&rules).Error; err != nil {
		return s.panelError(err.Error())
	}
	return s.panel(rules)
}
