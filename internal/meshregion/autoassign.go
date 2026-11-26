package meshregion

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// Agent nodes added without a region (appendix D.5 item 2): the node is
// recorded as waiting (model.MeshAutoNode) and gets a configuration with no
// network and a region object that has no current region (nodeConfig). It
// measures the regions of its directory and reports the round trips in
// "mesh.status" (region_probe.regions); the panel then chooses the region as
// it does for a device (chooseRegion, the settings' default preferred
// region) and gives the node its fixed address there like AddNode, with the
// basis latency.

// AutoNodeView is a node waiting for its region.
type AutoNodeView struct {
	model.MeshAutoNode
	NodeName string `json:"node_name"`
}

// AddAutoNode records node, an Agent node that is in no region, as waiting
// for the region its latencies choose, and pushes its configuration.
func AddAutoNode(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, createdBy *uint, now time.Time) (model.MeshAutoNode, error) {
	row := model.MeshAutoNode{NodeRef: node.String(), CreatedBy: createdBy, CreatedAt: now}
	err := change(ctx, db, func(tx *gorm.DB) error {
		if _, err := lookupNode(tx, row.NodeRef); err != nil {
			return err
		}
		var members, waiting int64
		if err := tx.Model(&model.MeshMember{}).Where("node_ref = ?", row.NodeRef).Count(&members).Error; err != nil {
			return err
		}
		if members > 0 {
			return conflict("the node is already in a region")
		}
		if err := tx.Model(&model.MeshAutoNode{}).Where("node_ref = ?", row.NodeRef).Count(&waiting).Error; err != nil {
			return err
		}
		if waiting > 0 {
			return conflict("the node already waits for its region")
		}
		return tx.Create(&row).Error
	})
	if err == nil {
		// Not a member yet, so change did not tell it.
		notify([]agentcontrol.AgentNode{node})
	}
	return row, err
}

// AutoNodes answers the nodes waiting for their region.
func AutoNodes(ctx context.Context, db *gorm.DB) ([]AutoNodeView, error) {
	db = db.WithContext(ctx)
	var rows []model.MeshAutoNode
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	views := make([]AutoNodeView, 0, len(rows))
	for _, row := range rows {
		info, err := lookupNode(db, row.NodeRef)
		if err != nil && !isNotFound(err) {
			return nil, err
		}
		views = append(views, AutoNodeView{MeshAutoNode: row, NodeName: info.name})
	}
	return views, nil
}

// RemoveAutoNode stops a node from waiting for a region; its configuration
// loses the member mesh. It answers the node.
func RemoveAutoNode(ctx context.Context, db *gorm.DB, id uint) (string, error) {
	var row model.MeshAutoNode
	err := change(ctx, db, func(tx *gorm.DB) error {
		var rows []model.MeshAutoNode
		if err := forUpdate(tx).Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return fmt.Errorf("%w: waiting node %d", ErrNotFound, id)
		}
		row = rows[0]
		return tx.Delete(&model.MeshAutoNode{}, id).Error
	})
	if err == nil {
		if node, parseErr := agentcontrol.ParseAgentNode(row.NodeRef); parseErr == nil {
			notify([]agentcontrol.AgentNode{node})
		}
	}
	return row.NodeRef, err
}

func isNotFound(err error) bool { return err != nil && errors.Is(err, ErrNotFound) }

// waiting tells whether nodeRef waits for its region.
func waiting(db *gorm.DB, nodeRef string) (bool, error) {
	var count int64
	err := db.Model(&model.MeshAutoNode{}).Where("node_ref = ?", nodeRef).Count(&count).Error
	return count > 0, err
}

// maxProbeRegions bounds the regions read from one report.
const maxProbeRegions = 64

// reportedLatencies answers the round trips a status payload reports in
// region_probe.regions by region code: the lowest of a code that is
// listed twice; entries without a finite round trip of 0 to 600000 ms are
// skipped.
func reportedLatencies(payload []byte) map[string]float64 {
	var shape reportedProbe
	latencies := map[string]float64{}
	if json.Unmarshal(payload, &shape) != nil || shape.RegionProbe == nil {
		return latencies
	}
	for i, entry := range shape.RegionProbe.Regions {
		if i == maxProbeRegions {
			break
		}
		if entry.RTT == nil || math.IsNaN(*entry.RTT) || *entry.RTT < 0 || *entry.RTT > 600000 || entry.Region == "" {
			continue
		}
		if known, ok := latencies[entry.Region]; !ok || *entry.RTT < known {
			latencies[entry.Region] = *entry.RTT
		}
	}
	return latencies
}

// placeAutoNode gives a waiting node its region from the latencies of the
// status payload it just reported: the enabled region of the lowest score
// (chooseRegion) with the settings' default preferred region. It does
// nothing when the report has no latency of an enabled region, or when
// the node no longer waits (assigned or removed meanwhile). It runs as an
// administrator's AddNode does, under the chosen region's lock, and the
// listeners are told the node's new configuration.
func placeAutoNode(ctx context.Context, db *gorm.DB, node agentcontrol.AgentNode, payload []byte, now time.Time) error {
	latencies := reportedLatencies(payload)
	if len(latencies) == 0 {
		return nil
	}
	return change(ctx, db, func(tx *gorm.DB) error {
		var regions []model.MeshRegion
		if err := tx.Where("enabled = ?", true).Order("sort ASC, id ASC").Find(&regions).Error; err != nil {
			return err
		}
		preferred, err := preferredRegionID(tx, nil)
		if err != nil {
			return err
		}
		chosen, ok := chooseRegion(regions, latencies, preferred)
		if !ok {
			return nil
		}
		// Regions before rows, as every change takes them: the region is
		// read again under its lock.
		var region model.MeshRegion
		if err := findRegion(tx, chosen.ID, &region); err != nil {
			return err
		}
		if !region.Enabled {
			return nil
		}
		var marks []model.MeshAutoNode
		if err := forUpdate(tx).Where("node_ref = ?", node.String()).Limit(1).Find(&marks).Error; err != nil {
			return err
		}
		if len(marks) == 0 {
			return nil
		}
		var members int64
		if err := tx.Model(&model.MeshMember{}).Where("node_ref = ?", node.String()).Count(&members).Error; err != nil {
			return err
		}
		if members == 0 {
			if _, err := lookupNode(tx, node.String()); err != nil {
				return err
			}
			row, err := join(tx, region, node.String(), true, false, now)
			if err != nil {
				return err
			}
			if err := setBasis(tx, &row, model.MeshBasisLatency); err != nil {
				return err
			}
		}
		return tx.Where("node_ref = ?", node.String()).Delete(&model.MeshAutoNode{}).Error
	})
}
