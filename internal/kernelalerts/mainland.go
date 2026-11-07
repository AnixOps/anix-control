package kernelalerts

import (
	"context"
	"fmt"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/model"
)

// mainlandWindow is how long a refused mainland destination keeps its
// node's alert open.
const mainlandWindow = 24 * time.Hour

// exitMainland finds exits that refuse mainland-China destinations and
// refused some within mainlandWindow: a client reached the exit for a
// mainland destination, so its entry or its routing sends that traffic the
// wrong way.
func (s *Scanner) exitMainland(ctx context.Context, now time.Time) ([]Finding, error) {
	var rows []model.NodeExitPolicy
	if err := s.DB.WithContext(ctx).Where("block_mainland = ? AND mainland_blocked_at IS NOT NULL", true).
		Order("node_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	var findings []Finding
	for _, row := range rows {
		if row.MainlandBlockedAt.Before(now.Add(-mainlandWindow)) {
			continue
		}
		node := agentcontrol.AgentNode{Kind: agentcontrol.NodeKindProxy, ID: uint32(row.NodeID)} // #nosec G115 -- node ids are 32-bit (agentcontrol.AgentNode).
		subject := node.String()
		name := s.nodeName(ctx, node)
		label := nodeLabel(subject, name)
		findings = append(findings, Finding{
			Kind: KindExitMainlandDestinations, Severity: model.KernelAlertWarning, SubjectKind: SubjectNode, Subject: subject,
			Message: truncate(fmt.Sprintf("The exit node %s refused connections to mainland-China destinations (%d so far, the last at %s). Check the entries and the clients' routing: mainland traffic should not reach this exit.",
				label, row.MainlandBlockedTotal, stamp(*row.MainlandBlockedAt)), 500),
			Detail: map[string]any{"node": subject, "node_name": name, "total": row.MainlandBlockedTotal, "last_at": stamp(*row.MainlandBlockedAt)},
		})
	}
	return findings, nil
}
