package kernelalerts

import (
	"context"
	"fmt"
	"strings"
	"time"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/v5/internal/l2standby"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/networkmodel"
	"google.golang.org/protobuf/encoding/protojson"
)

// CrossBorderDownAfter is how long a cross-border transport has to be down
// on the devices that run it before it alerts: long enough to ride out a
// reconnect, short enough to switch before most users notice.
const CrossBorderDownAfter = 10 * time.Minute

// crossBorderStatusFresh is how old a device's status may be for the scan
// to trust it: a device that stopped reporting is the Agent's alert, not a
// blocked link.
const crossBorderStatusFresh = 15 * time.Minute

// crossBorderRunbook is where the alerts send the administrator.
const crossBorderRunbook = "docs/guide/cross-border-standby.md"

var crossBorderDown = map[networkv1.ComponentState]bool{
	networkv1.ComponentState_COMPONENT_STATE_DEGRADED: true,
	networkv1.ComponentState_COMPONENT_STATE_DOWN:     true,
	networkv1.ComponentState_COMPONENT_STATE_ERROR:    true,
}

// crossBorder finds cross-border transports (kind anx, not trusted) that
// the devices at their ends have reported down for CrossBorderDownAfter,
// and, when at least two transports exist and half or more of them are
// down, the wide outage that says the jacket itself is targeted rather
// than one address.
func (s *Scanner) crossBorder(ctx context.Context, now time.Time) ([]Finding, error) {
	db := s.DB.WithContext(ctx)
	if !db.Migrator().HasTable(&model.NetworkModel{}) || !db.Migrator().HasTable(&model.NetworkDevice{}) {
		return nil, nil
	}
	doc, err := networkmodel.LoadDocument(db)
	if err != nil {
		return nil, err
	}
	links := l2standby.Links(doc)
	if len(links) == 0 {
		return nil, nil
	}
	tiers, err := l2standby.ActiveTiers(db)
	if err != nil {
		return nil, err
	}
	var rows []model.NetworkDevice
	if err := db.Where("status_json <> ''").Find(&rows).Error; err != nil {
		return nil, err
	}
	// The component of each transport on every fresh device that reports it.
	reports := map[string][]*networkv1.ComponentStatus{}
	reporters := map[string][]string{}
	for _, row := range rows {
		if row.StatusAt == nil || row.StatusAt.Before(now.Add(-crossBorderStatusFresh)) {
			continue
		}
		status := &networkv1.NetworkStatus{}
		if (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(row.StatusJSON), status) != nil {
			continue
		}
		for _, component := range status.GetComponents() {
			reports[component.GetId()] = append(reports[component.GetId()], component)
			reporters[component.GetId()] = append(reporters[component.GetId()], row.NodeRef)
		}
	}
	var findings []Finding
	var down []string
	for _, link := range links {
		components := reports[link.TransportID]
		if len(components) == 0 {
			continue
		}
		// Down on every device that reports it, since before the window.
		since := time.Time{}
		var details []string
		allDown := true
		for _, component := range components {
			if !crossBorderDown[component.GetState()] {
				allDown = false
				break
			}
			if component.GetChangedAtUnixMs() <= 0 {
				// Down since an unknown time: wait for a report that says.
				allDown = false
				break
			}
			changed := time.UnixMilli(component.GetChangedAtUnixMs()).UTC()
			if changed.After(since) {
				since = changed
			}
			if detail := strings.TrimSpace(component.GetDetail()); detail != "" {
				details = append(details, detail)
			}
		}
		if !allDown || since.IsZero() || now.Sub(since) < CrossBorderDownAfter {
			continue
		}
		down = append(down, link.TransportID)
		tier := tiers[link.TransportID]
		if tier == "" {
			tier = l2standby.TierOff
		}
		severity := model.KernelAlertCritical
		action := fmt.Sprintf("Switch it to a standby channel: POST /api/v4/kernel/network/standby/switch {\"transports\":[%q],\"tier\":\"reality\"} (节点 → 过境应急), xhttp if reality is blocked too; see %s.", link.TransportID, crossBorderRunbook)
		if tier != l2standby.TierOff {
			severity = model.KernelAlertWarning
			action = fmt.Sprintf("Its traffic runs on the %s standby channel now; switch back (tier off) once the transport is up again, see %s.", tier, crossBorderRunbook)
		}
		findings = append(findings, Finding{
			Kind: KindCrossBorderLinkDown, Severity: severity, SubjectKind: SubjectTransport, Subject: link.TransportID,
			Message: truncate(fmt.Sprintf("The cross-border transport %s (%s → %s) has been down since %s. %s",
				link.TransportID, link.From, link.To, stamp(since), action), 500),
			Detail: map[string]any{"transport": link.TransportID, "from": link.From, "to": link.To, "since": stamp(since),
				"reporters": reporters[link.TransportID], "reasons": truncate(strings.Join(details, "; "), 300),
				"active_tier": tier, "runbook": crossBorderRunbook},
		})
	}
	if len(links) >= 2 && len(down)*2 >= len(links) {
		findings = append(findings, Finding{
			Kind: KindCrossBorderWideOutage, Severity: model.KernelAlertCritical, SubjectKind: SubjectTransport, Subject: "all",
			Message: truncate(fmt.Sprintf("%d of %d cross-border transports are down at once: the ANX-A jacket itself is likely targeted, not one address. Follow the manual plan of %s: switch all to reality (or xhttp), then report.",
				len(down), len(links), crossBorderRunbook), 500),
			Detail: map[string]any{"down": down, "total": len(links), "runbook": crossBorderRunbook},
		})
	}
	return findings, nil
}
