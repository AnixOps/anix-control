package agentupgrade

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// BatchView is one batch of a campaign with its nodes counted by state.
type BatchView struct {
	Index              int            `json:"index"`
	Percent            int            `json:"percent"`
	MinDurationSeconds int            `json:"min_duration_seconds"`
	Nodes              int            `json:"nodes"`
	States             map[string]int `json:"states"`
	// Offered and Failed are the batch's failure ratio's terms (H19):
	// skipped and pending nodes do not count.
	Offered int `json:"offered"`
	Failed  int `json:"failed"`
}

// NodeView is one node of a campaign.
type NodeView struct {
	model.AgentUpgradeNode
	Node string `json:"node"`
}

// CampaignView is a campaign as the API and the command line show it.
type CampaignView struct {
	model.AgentUpgradeCampaign
	Artifacts []agentcontrol.UpgradeArtifact `json:"artifacts"`
	Batches   []BatchView                    `json:"batches"`
	Exclude   Exclude                        `json:"exclude"`
	Total     int                            `json:"total"`
	States    map[string]int                 `json:"states"`
	// BatchEndsAt is the earliest the current batch can pass.
	BatchEndsAt *time.Time `json:"batch_ends_at"`
	Nodes       []NodeView `json:"nodes,omitempty"`
}

// List returns the latest campaigns, newest first, without their nodes.
func (s *Service) List(ctx context.Context, limit int) ([]CampaignView, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	var campaigns []model.AgentUpgradeCampaign
	if err := s.DB.WithContext(ctx).Order("created_at DESC").Order("id DESC").Limit(limit).Find(&campaigns).Error; err != nil {
		return nil, err
	}
	views := make([]CampaignView, 0, len(campaigns))
	for _, campaign := range campaigns {
		view, err := s.view(ctx, campaign, false)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

// Get returns one campaign with its nodes in canary order.
func (s *Service) Get(ctx context.Context, id string) (CampaignView, error) {
	var campaign model.AgentUpgradeCampaign
	if err := s.DB.WithContext(ctx).Where("id = ?", id).First(&campaign).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return CampaignView{}, ErrNotFound
		}
		return CampaignView{}, err
	}
	return s.view(ctx, campaign, true)
}

// Active returns the active campaign, if any.
func (s *Service) Active(ctx context.Context) (CampaignView, bool, error) {
	var campaign model.AgentUpgradeCampaign
	err := s.DB.WithContext(ctx).Where("active_slot IS NOT NULL").First(&campaign).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CampaignView{}, false, nil
	}
	if err != nil {
		return CampaignView{}, false, err
	}
	view, err := s.view(ctx, campaign, false)
	return view, err == nil, err
}

func (s *Service) view(ctx context.Context, campaign model.AgentUpgradeCampaign, withNodes bool) (CampaignView, error) {
	view := CampaignView{AgentUpgradeCampaign: campaign, States: map[string]int{}}
	_ = json.Unmarshal([]byte(campaign.ArtifactsJSON), &view.Artifacts)
	_ = json.Unmarshal([]byte(campaign.ExcludeJSON), &view.Exclude)
	var batches []Batch
	_ = json.Unmarshal([]byte(campaign.BatchesJSON), &batches)
	view.Batches = make([]BatchView, len(batches))
	for index, batch := range batches {
		view.Batches[index] = BatchView{Index: index, Percent: batch.Percent, MinDurationSeconds: batch.MinDurationSeconds, States: map[string]int{}}
	}
	if campaign.BatchStartedAt != nil && campaign.CurrentBatch < len(batches) && campaign.FinishedAt == nil {
		ends := campaign.BatchStartedAt.Add(time.Duration(batches[campaign.CurrentBatch].MinDurationSeconds) * time.Second)
		view.BatchEndsAt = &ends
	}
	var nodes []model.AgentUpgradeNode
	if err := s.DB.WithContext(ctx).Where("campaign_id = ?", campaign.ID).Order("batch").Order("order_key").Find(&nodes).Error; err != nil {
		return CampaignView{}, err
	}
	perBatch := map[int][]model.AgentUpgradeNode{}
	for _, node := range nodes {
		view.Total++
		view.States[node.State]++
		perBatch[node.Batch] = append(perBatch[node.Batch], node)
		if withNodes {
			view.Nodes = append(view.Nodes, NodeView{
				AgentUpgradeNode: node,
				Node:             agentcontrol.AgentNode{Kind: node.NodeKind, ID: uint32(node.NodeID)}.String(), // #nosec G115 -- stored from a 32-bit agentcontrol.AgentNode.
			})
		}
	}
	for index := range view.Batches {
		members := perBatch[index]
		stats := batchStats(members)
		view.Batches[index].Nodes = len(members)
		view.Batches[index].Offered, view.Batches[index].Failed = stats.offered, stats.failed
		for _, node := range members {
			view.Batches[index].States[node.State]++
		}
	}
	return view, nil
}
