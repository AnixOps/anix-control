package kernelforward

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/planner"
	"github.com/AnixOps/anix-control/sdk/forward/validate"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
)

// The node inventory and the forward node registry as ForwardControl
// serves them (F5a: ListNodes, GetNode, SetNodeSettings, CreateForwardNode,
// UpdateForwardNode, DeleteForwardNode). A forward node's row
// (v2_forward_node) is written through service.ForwardNodeService, as the
// v2 handlers write it, so its credential split and Agent certificates
// follow; no answer carries the node's credential.

const (
	methodCreateNode = "create_forward_node"
	methodUpdateNode = "update_forward_node"
	methodDeleteNode = "delete_forward_node"

	maxNodeName = 100
	maxNodeHost = 255
	maxNodeText = 50
	// maxNodeID is the largest id an Agent identity names.
	maxNodeID = 1<<32 - 1
)

// ansibleTag marks a forward node driven by the Ansible fallback.
const ansibleTag = service.ForwardNodeInventoryTagAnsibleMachine

// NodeFilter keeps part of the inventory (ListNodes).
type NodeFilter struct {
	// Kind keeps forward or proxy nodes; empty for both.
	Kind string
	// Transport keeps the forward nodes reached so (and proxy nodes, which
	// run an Agent, for AGENT); UNSPECIFIED for every node.
	Transport forwardv1.NodeTransport
	// nodeRef keeps one node.
	nodeRef string
}

// SettingsFromProto converts the contract's settings; nil means the
// defaults.
func SettingsFromProto(settings *forwardv1.NodeSettings) NodeSettings {
	out := NodeSettings{
		ReservedPorts: append([]uint32(nil), settings.GetReservedPorts()...),
		Addresses:     append([]string(nil), settings.GetAddresses()...),
	}
	if r := settings.GetPortRange(); r != nil {
		out.PortFirst, out.PortLast = r.GetFirst(), r.GetLast()
	}
	if labels := settings.GetLabels(); len(labels) > 0 {
		out.Labels = make(map[string]string, len(labels))
		for k, v := range labels {
			out.Labels[k] = v
		}
	}
	return out
}

func settingsToProto(row model.KernelForwardNode) *forwardv1.NodeSettings {
	settings := &forwardv1.NodeSettings{}
	if row.PortFirst != 0 {
		settings.PortRange = &forwardv1.PortRange{First: row.PortFirst, Last: row.PortLast}
	}
	if row.ReservedPortsJSON != "" {
		_ = json.Unmarshal([]byte(row.ReservedPortsJSON), &settings.ReservedPorts)
	}
	if row.AddressesJSON != "" {
		_ = json.Unmarshal([]byte(row.AddressesJSON), &settings.Addresses)
	}
	if row.LabelsJSON != "" {
		_ = json.Unmarshal([]byte(row.LabelsJSON), &settings.Labels)
	}
	return settings
}

func nonNegative32(value int) uint32 {
	if value <= 0 {
		return 0
	}
	if value > int(^uint32(0)>>1) {
		return ^uint32(0) >> 1
	}
	return uint32(value) // #nosec G115 -- bounded above.
}

func nonNegative64(value int64) uint64 {
	if value <= 0 {
		return 0
	}
	return uint64(value)
}

func hasTag(tags, tag string) bool {
	var list []string
	if tags == "" || json.Unmarshal([]byte(tags), &list) != nil {
		return false
	}
	for _, item := range list {
		if item == tag {
			return true
		}
	}
	return false
}

// withTag adds or removes tag from a JSON tag list, keeping the others.
func withTag(tags, tag string, present bool) string {
	var list []string
	if tags != "" {
		_ = json.Unmarshal([]byte(tags), &list)
	}
	out := make([]string, 0, len(list)+1)
	for _, item := range list {
		if item != tag {
			out = append(out, item)
		}
	}
	if present {
		out = append(out, tag)
	}
	if len(out) == 0 {
		return ""
	}
	encoded, _ := json.Marshal(out)
	return string(encoded)
}

func recordOf(node model.ForwardNode, ansible bool) *forwardv1.ForwardNodeRecord {
	transport := forwardv1.NodeTransport_NODE_TRANSPORT_AGENT
	if ansible {
		transport = forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE
	}
	return &forwardv1.ForwardNodeRecord{
		Id: uint64(node.ID), Name: node.Name, Host: node.Host, Role: node.Type, Transport: transport,
		Port: nonNegative32(node.Port), ApiPort: nonNegative32(node.APIPort), MetricsPort: nonNegative32(node.MetricsPort),
		Region: node.Region, Isp: node.ISP, BandwidthMbps: nonNegative64(node.Bandwidth),
		Weight: nonNegative32(node.Weight), MaxConns: nonNegative32(node.MaxConn), Enabled: node.Enabled,
		CreatedAtUnixMs: node.CreatedAt.UnixMilli(), UpdatedAtUnixMs: node.UpdatedAt.UnixMilli(),
	}
}

// forwardRef is a forward node's reference.
func forwardRef(id uint) string { return fmt.Sprintf("forward-%d", id) }

// ansibleIDs are the forward nodes on the Ansible transport: those the v2
// handlers scope so (a relay tagged so, or a relay without an API port and
// credential), except an untagged one whose Agent negotiated forward.v1,
// which runs an Agent.
func ansibleIDs(db *gorm.DB) (map[uint]bool, error) {
	nodes, _, err := service.NewForwardNodeService(db).ListByInventoryScope(service.ForwardNodeInventoryScopeAnsible, model.ForwardNodeTypeRelay, nil, 1, 1<<20)
	if err != nil {
		return nil, fmt.Errorf("kernel forward: ansible inventory: %w", err)
	}
	var negotiated []uint64
	if err := db.Model(&model.KernelForwardNode{}).Where("node_kind = ? AND negotiated = ?", agentcontrol.NodeKindForward, true).
		Pluck("node_id", &negotiated).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: ansible inventory: %w", err)
	}
	agents := make(map[uint64]bool, len(negotiated))
	for _, id := range negotiated {
		agents[id] = true
	}
	out := make(map[uint]bool, len(nodes))
	for _, node := range nodes {
		if hasTag(node.Tags, ansibleTag) || !agents[uint64(node.ID)] {
			out[node.ID] = true
		}
	}
	return out, nil
}

// ListNodes answers the forward nodes and the inventory's proxy nodes, as
// ForwardControl.ListNodes.
func (s *Service) ListNodes(ctx context.Context, filter NodeFilter) ([]*forwardv1.NodeSummary, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	if filter.Kind != "" && filter.Kind != agentcontrol.NodeKindForward && filter.Kind != agentcontrol.NodeKindProxy {
		return nil, fmt.Errorf("%w: kind must be forward or proxy", ErrInvalidRequest)
	}
	return nodeSummaries(db, filter)
}

// GetNode answers one node with its desired state and latest report, as
// ForwardControl.GetNode; ErrNodeNotFound when its row does not exist.
func (s *Service) GetNode(ctx context.Context, nodeRef string) (*forwardv1.GetNodeResponse, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	node, err := agentcontrol.ParseAgentNode(nodeRef)
	if err != nil {
		return nil, fmt.Errorf("%w: node_ref: %v", ErrInvalidRequest, err)
	}
	summary, err := nodeSummary(db, node)
	if err != nil {
		return nil, err
	}
	response := &forwardv1.GetNodeResponse{Node: summary}
	if state, found, err := loadState(db, node.String()); err != nil {
		return nil, err
	} else if found {
		response.State = state
	}
	if report, found, err := s.LatestReport(ctx, node.String()); err != nil {
		return nil, err
	} else if found {
		response.Report = report
	}
	return response, nil
}

// nodeSummary answers one node's summary; ErrNodeNotFound when its row
// does not exist.
func nodeSummary(db *gorm.DB, node agentcontrol.AgentNode) (*forwardv1.NodeSummary, error) {
	summaries, err := nodeSummaries(db, NodeFilter{nodeRef: node.String()})
	if err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNodeNotFound, node)
	}
	return summaries[0], nil
}

// proxySummaryRow is the part of a proxy node row a summary shows.
type proxySummaryRow struct {
	ID     uint
	Name   string
	Host   string
	Status model.NodeStatus
}

func nodeSummaries(db *gorm.DB, filter NodeFilter) ([]*forwardv1.NodeSummary, error) {
	var one *agentcontrol.AgentNode
	if filter.nodeRef != "" {
		node, err := agentcontrol.ParseAgentNode(filter.nodeRef)
		if err != nil {
			return nil, fmt.Errorf("%w: node_ref: %v", ErrInvalidRequest, err)
		}
		one = &node
	}
	wantForward := filter.Kind == "" || filter.Kind == agentcontrol.NodeKindForward
	wantProxy := (filter.Kind == "" || filter.Kind == agentcontrol.NodeKindProxy) &&
		filter.Transport != forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE
	if one != nil {
		wantForward = wantForward && one.Kind == agentcontrol.NodeKindForward
		wantProxy = wantProxy && one.Kind == agentcontrol.NodeKindProxy
	}

	inventoryQuery := db.Model(&model.KernelForwardNode{})
	if one != nil {
		inventoryQuery = inventoryQuery.Where("node_ref = ?", one.String())
	}
	var rows []model.KernelForwardNode
	if err := inventoryQuery.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load inventory: %w", err)
	}
	inventoryRows := make(map[string]model.KernelForwardNode, len(rows))
	var proxyIDs []uint64
	for _, row := range rows {
		inventoryRows[row.NodeRef] = row
		if row.NodeKind == agentcontrol.NodeKindProxy {
			proxyIDs = append(proxyIDs, row.NodeID)
		}
	}

	var forwards []model.ForwardNode
	ansible := map[uint]bool{}
	if wantForward {
		query := db.Model(&model.ForwardNode{}).Omit("api_token")
		if one != nil {
			query = query.Where("id = ?", one.ID)
		}
		if err := query.Order("id").Find(&forwards).Error; err != nil {
			return nil, fmt.Errorf("kernel forward: load forward nodes: %w", err)
		}
		var err error
		if ansible, err = ansibleIDs(db); err != nil {
			return nil, err
		}
	}
	var proxies []proxySummaryRow
	if wantProxy {
		if one != nil {
			// A proxy node not in the inventory yet is still answered, so its
			// settings can be read before they are set.
			proxyIDs = []uint64{uint64(one.ID)}
		}
		if len(proxyIDs) > 0 {
			if err := db.Model(&model.Node{}).Select("id", "name", "host", "status").Where("id IN ?", proxyIDs).
				Order("id").Find(&proxies).Error; err != nil {
				return nil, fmt.Errorf("kernel forward: load proxy nodes: %w", err)
			}
		}
	}

	inv, err := loadInventory(db)
	if err != nil {
		return nil, err
	}
	infos := make(map[string]*forwardv1.NodeInfo, len(inv.nodes))
	for _, info := range inv.nodes {
		infos[info.GetNodeRef()] = info
	}
	states, err := loadStateRows(db, one)
	if err != nil {
		return nil, err
	}
	reports, err := loadReportRows(db, one)
	if err != nil {
		return nil, err
	}

	out := make([]*forwardv1.NodeSummary, 0, len(forwards)+len(proxies))
	for _, node := range forwards {
		isAnsible := ansible[node.ID]
		switch filter.Transport {
		case forwardv1.NodeTransport_NODE_TRANSPORT_AGENT:
			if isAnsible {
				continue
			}
		case forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE:
			if !isAnsible {
				continue
			}
		}
		ref := forwardRef(node.ID)
		summary := &forwardv1.NodeSummary{
			NodeRef: ref, Kind: agentcontrol.NodeKindForward, Name: node.Name, Host: node.Host, Enabled: node.Enabled,
			Record: recordOf(node, isAnsible),
		}
		out = append(out, fillSummary(summary, inventoryRows, infos, inv.reserved, states, reports))
	}
	for _, node := range proxies {
		ref := fmt.Sprintf("proxy-%d", node.ID)
		summary := &forwardv1.NodeSummary{
			NodeRef: ref, Kind: agentcontrol.NodeKindProxy, Name: node.Name, Host: node.Host,
			Enabled: node.Status != model.NodeStatusDisabled,
		}
		out = append(out, fillSummary(summary, inventoryRows, infos, inv.reserved, states, reports))
	}
	return out, nil
}

func fillSummary(summary *forwardv1.NodeSummary, rows map[string]model.KernelForwardNode, infos map[string]*forwardv1.NodeInfo,
	reserved map[string][]uint32, states map[string]model.KernelForwardNodeState, reports map[string]model.KernelForwardNodeReport,
) *forwardv1.NodeSummary {
	ref := summary.GetNodeRef()
	if row, ok := rows[ref]; ok {
		summary.Negotiated = row.Negotiated
		summary.Settings = settingsToProto(row)
		if row.CapabilitiesJSON != "" {
			caps := &forwardv1.NodeCapabilities{}
			if err := jsonRead.Unmarshal([]byte(row.CapabilitiesJSON), caps); err == nil {
				summary.Capabilities = caps
			}
		}
	}
	if info, ok := infos[ref]; ok {
		summary.InInventory, summary.Info, summary.ReservedPorts = true, info, reserved[ref]
	}
	if state, ok := states[ref]; ok {
		summary.DesiredGeneration, summary.DesiredStateHash = state.Generation, state.StateHash
		decoded := &forwardv1.NodeForwardState{}
		if err := jsonRead.Unmarshal([]byte(state.StateJSON), decoded); err == nil {
			summary.DesiredHops = uint32(len(decoded.GetHops())) // #nosec G115 -- bounded by the planner's caps.
		}
	}
	if report, ok := reports[ref]; ok {
		summary.Reported, summary.ReportedGeneration, summary.ReportedStateHash = true, report.Generation, report.StateHash
		summary.Applied, summary.HopErrors = report.Applied, nonNegative32(report.HopErrors)
		summary.ReportedAtUnixMs = report.ObservedAt.UnixMilli()
	}
	return summary
}

func loadStateRows(db *gorm.DB, one *agentcontrol.AgentNode) (map[string]model.KernelForwardNodeState, error) {
	query := db.Model(&model.KernelForwardNodeState{})
	if one != nil {
		query = query.Where("node_ref = ?", one.String())
	}
	var rows []model.KernelForwardNodeState
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load node states: %w", err)
	}
	out := make(map[string]model.KernelForwardNodeState, len(rows))
	for _, row := range rows {
		out[row.NodeRef] = row
	}
	return out, nil
}

func loadReportRows(db *gorm.DB, one *agentcontrol.AgentNode) (map[string]model.KernelForwardNodeReport, error) {
	query := db.Model(&model.KernelForwardNodeReport{}).Select("node_ref", "generation", "state_hash", "applied", "hop_errors", "observed_at")
	if one != nil {
		query = query.Where("node_ref = ?", one.String())
	}
	var rows []model.KernelForwardNodeReport
	if err := query.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("kernel forward: load node reports: %w", err)
	}
	out := make(map[string]model.KernelForwardNodeReport, len(rows))
	for _, row := range rows {
		out[row.NodeRef] = row
	}
	return out, nil
}

// SetNodeSettingsAnswer is SetNodeSettings with the contract's answer: the
// node's summary after the replan, and the violations of a refused replan.
func (s *Service) SetNodeSettingsAnswer(ctx context.Context, nodeRef string, settings NodeSettings) (*forwardv1.SetNodeSettingsResponse, error) {
	node, err := agentcontrol.ParseAgentNode(nodeRef)
	if err != nil {
		return nil, fmt.Errorf("%w: node_ref: %v", ErrInvalidRequest, err)
	}
	outcome, err := s.SetNodeSettings(ctx, node, settings)
	if err != nil {
		return nil, err
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	summary, err := nodeSummary(db, node)
	if err != nil {
		return nil, err
	}
	return &forwardv1.SetNodeSettingsResponse{Node: summary, Violations: outcomeViolations(outcome)}, nil
}

func outcomeViolations(outcome PlanOutcome) []*forwardv1.Violation {
	if !outcome.Refused {
		return nil
	}
	return (&RefusedError{Violations: outcome.Violations}).ProtoViolations()
}

// checkRecord validates a forward node record's editable fields.
func checkRecord(record *forwardv1.ForwardNodeRecord) error {
	name, host := strings.TrimSpace(record.GetName()), strings.TrimSpace(record.GetHost())
	switch {
	case name == "" || utf8.RuneCountInString(name) > maxNodeName:
		return fmt.Errorf("%w: name is required and at most %d characters", ErrInvalidRequest, maxNodeName)
	case host == "" || len(host) > maxNodeHost || strings.ContainsAny(host, " /\t\r\n"):
		return fmt.Errorf("%w: host is required, at most %d bytes, an address or a host name", ErrInvalidRequest, maxNodeHost)
	case record.GetRole() != "" && record.GetRole() != model.ForwardNodeTypeRelay && record.GetRole() != model.ForwardNodeTypeExit:
		return fmt.Errorf("%w: role must be relay or exit", ErrInvalidRequest)
	case record.GetPort() > validate.MaxPort || record.GetApiPort() > validate.MaxPort || record.GetMetricsPort() > validate.MaxPort:
		return fmt.Errorf("%w: ports must be within 0-65535", ErrInvalidRequest)
	case utf8.RuneCountInString(record.GetRegion()) > maxNodeText || utf8.RuneCountInString(record.GetIsp()) > maxNodeText:
		return fmt.Errorf("%w: region and isp are at most %d characters", ErrInvalidRequest, maxNodeText)
	case record.GetBandwidthMbps() > 1<<40 || record.GetWeight() > 1<<20 || record.GetMaxConns() > 1<<30:
		return fmt.Errorf("%w: bandwidth, weight or max_conns is too large", ErrInvalidRequest)
	}
	switch record.GetTransport() {
	case forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED, forwardv1.NodeTransport_NODE_TRANSPORT_AGENT:
	case forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE:
		if record.GetRole() == model.ForwardNodeTypeExit {
			return fmt.Errorf("%w: an Ansible machine is a relay", ErrInvalidRequest)
		}
	default:
		return fmt.Errorf("%w: transport is not a NodeTransport", ErrInvalidRequest)
	}
	return nil
}

// applyRecord copies a record's editable fields onto a node row. An
// Ansible machine is a relay tagged so, without an API port or credential
// (as the v2 handlers keep it). A new Agent node gets a credential for the
// v2 paths that still read one until F5d (as the v2 create does); an
// update never changes an Agent node's credential, since a changed one
// revokes the node's Agent certificates.
func applyRecord(node *model.ForwardNode, record *forwardv1.ForwardNodeRecord, create bool) error {
	node.Name, node.Host = strings.TrimSpace(record.GetName()), strings.TrimSpace(record.GetHost())
	node.Type = record.GetRole()
	if node.Type == "" {
		node.Type = model.ForwardNodeTypeExit
	}
	node.Port, node.APIPort, node.MetricsPort = int(record.GetPort()), int(record.GetApiPort()), int(record.GetMetricsPort())
	node.Region, node.ISP = record.GetRegion(), record.GetIsp()
	node.Bandwidth = int64(record.GetBandwidthMbps()) // #nosec G115 -- checkRecord bounds it.
	node.Weight, node.MaxConn = int(record.GetWeight()), int(record.GetMaxConns())
	ansible := record.GetTransport() == forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE
	node.Tags = withTag(node.Tags, ansibleTag, ansible)
	if ansible {
		node.Type, node.APIPort, node.APIToken = model.ForwardNodeTypeRelay, 0, ""
		return nil
	}
	if create {
		token, err := service.NewForwardNodeService(nil).GenerateAPIToken()
		if err != nil {
			return err
		}
		node.APIToken = token
	}
	return nil
}

// routesUsing answers a node_in_use violation for every hop of a stored
// route that names nodeRef.
func routesUsing(tx *gorm.DB, nodeRef string) ([]planner.RouteViolation, error) {
	routes, err := loadRoutes(tx)
	if err != nil {
		return nil, err
	}
	var out []planner.RouteViolation
	for _, stored := range routes {
		for i, hop := range stored.route.GetHops() {
			for j, ref := range hop.GetNodeRefs() {
				if ref == nodeRef {
					out = append(out, planner.RouteViolation{RouteID: stored.route.GetId(), Violation: validate.Violation{
						Field: fmt.Sprintf("hops[%d].node_refs[%d]", i, j), Code: validate.CodeNodeInUse,
						Message: fmt.Sprintf("route %q uses %s", stored.route.GetName(), nodeRef),
					}})
				}
			}
		}
	}
	return out, nil
}

// nodeWrite runs one node write under the plan lock: body writes the rows,
// a replan follows in the same transaction, and answer builds and records
// the response. Unlike a route write, a refused replan does not roll the
// write back (the inventory changed; every node keeps its generation and
// the plan status records why), except when body refuses the write itself.
func (s *Service) nodeWrite(ctx context.Context, reason string, body func(tx *gorm.DB, now time.Time) (done bool, err error),
	answer func(tx *gorm.DB, outcome PlanOutcome, now time.Time) error,
) error {
	db, err := s.db(ctx)
	if err != nil {
		return err
	}
	var outcome PlanOutcome
	err = db.Transaction(func(tx *gorm.DB) error {
		now := s.now()
		if _, err := lockPlan(tx, now); err != nil {
			return err
		}
		done, err := body(tx, now)
		if err != nil || done {
			return err
		}
		routes, err := loadRoutes(tx)
		if err != nil {
			return err
		}
		if outcome, err = s.replanTx(tx, routes, now); err != nil {
			return err
		}
		if err := recordPlan(tx, outcome, reason, now); err != nil {
			return err
		}
		return answer(tx, outcome, now)
	})
	if err != nil {
		return err
	}
	if outcome.Refused {
		logRefusedPlan(reason, outcome.Violations)
	}
	notify(outcome.Changed)
	return nil
}

// CreateForwardNode adds a forward node, as ForwardControl.CreateForwardNode.
func (s *Service) CreateForwardNode(ctx context.Context, requestID string, nodeRecord *forwardv1.ForwardNodeRecord, settings *forwardv1.NodeSettings) (*forwardv1.NodeSummary, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if nodeRecord == nil {
		return nil, fmt.Errorf("%w: node is required", ErrInvalidRequest)
	}
	if nodeRecord.GetId() != 0 {
		return nil, fmt.Errorf("%w: node id is assigned by Control", ErrInvalidRequest)
	}
	if err := checkRecord(nodeRecord); err != nil {
		return nil, err
	}
	var nodeSettings NodeSettings
	if settings != nil {
		nodeSettings = SettingsFromProto(settings)
		if err := nodeSettings.check(); err != nil {
			return nil, err
		}
	}
	hash := requestHash(methodCreateNode, &forwardv1.CreateForwardNodeRequest{Node: nodeRecord, Settings: settings})
	response := &forwardv1.CreateForwardNodeResponse{}
	var created model.ForwardNode
	err = s.nodeWrite(ctx, "node_created", func(tx *gorm.DB, now time.Time) (bool, error) {
		if found, err := replay(tx, requestID, methodCreateNode, hash, response); err != nil || found {
			return found, err
		}
		created = model.ForwardNode{Enabled: true, Status: model.ForwardNodeStatusOffline, CreatedAt: now, UpdatedAt: now}
		if err := applyRecord(&created, nodeRecord, true); err != nil {
			return false, err
		}
		if err := service.NewForwardNodeService(tx).Create(&created); err != nil {
			return false, fmt.Errorf("kernel forward: create forward node: %w", err)
		}
		if settings != nil {
			node, err := agentcontrol.ParseAgentNode(forwardRef(created.ID))
			if err != nil {
				return false, err
			}
			if err := storeSettings(tx, node, nodeSettings, now); err != nil {
				return false, err
			}
		}
		return false, nil
	}, func(tx *gorm.DB, _ PlanOutcome, now time.Time) error {
		node, err := agentcontrol.ParseAgentNode(forwardRef(created.ID))
		if err != nil {
			return err
		}
		summary, err := nodeSummary(tx, node)
		if err != nil {
			return err
		}
		response.Node = summary
		return record(tx, requestID, methodCreateNode, node.String(), hash, response, now)
	})
	if err != nil {
		return nil, err
	}
	return response.GetNode(), nil
}

// loadForwardNode loads a forward node row for update; ErrNodeNotFound when
// it does not exist.
func loadForwardNode(tx *gorm.DB, id uint64) (model.ForwardNode, error) {
	var rows []model.ForwardNode
	if err := tx.Where("id = ?", id).Limit(1).Find(&rows).Error; err != nil {
		return model.ForwardNode{}, err
	}
	if len(rows) == 0 {
		return model.ForwardNode{}, fmt.Errorf("%w: forward-%d", ErrNodeNotFound, id)
	}
	return rows[0], nil
}

// UpdateForwardNode replaces a forward node's editable fields, as
// ForwardControl.UpdateForwardNode.
func (s *Service) UpdateForwardNode(ctx context.Context, requestID string, nodeRecord *forwardv1.ForwardNodeRecord) (*forwardv1.UpdateForwardNodeResponse, error) {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return nil, err
	}
	if nodeRecord.GetId() == 0 || nodeRecord.GetId() > maxNodeID {
		return nil, fmt.Errorf("%w: node id is required", ErrInvalidRequest)
	}
	if err := checkRecord(nodeRecord); err != nil {
		return nil, err
	}
	hash := requestHash(methodUpdateNode, &forwardv1.UpdateForwardNodeRequest{Node: nodeRecord})
	response := &forwardv1.UpdateForwardNodeResponse{}
	nodeRef := fmt.Sprintf("forward-%d", nodeRecord.GetId())
	err = s.nodeWrite(ctx, "node_updated", func(tx *gorm.DB, now time.Time) (bool, error) {
		if found, err := replay(tx, requestID, methodUpdateNode, hash, response); err != nil || found {
			return found, err
		}
		node, err := loadForwardNode(tx, nodeRecord.GetId())
		if err != nil {
			return false, err
		}
		if node.Enabled && !nodeRecord.GetEnabled() {
			using, err := routesUsing(tx, nodeRef)
			if err != nil {
				return false, err
			}
			if len(using) > 0 {
				return false, &RefusedError{Violations: using, Precondition: true}
			}
		}
		effective := nodeRecord
		if nodeRecord.GetTransport() == forwardv1.NodeTransport_NODE_TRANSPORT_UNSPECIFIED {
			// UNSPECIFIED keeps the node's transport.
			ansible, err := ansibleIDs(tx)
			if err != nil {
				return false, err
			}
			effective = proto.Clone(nodeRecord).(*forwardv1.ForwardNodeRecord)
			effective.Transport = forwardv1.NodeTransport_NODE_TRANSPORT_AGENT
			if ansible[node.ID] {
				effective.Transport = forwardv1.NodeTransport_NODE_TRANSPORT_ANSIBLE
			}
		}
		if err := applyRecord(&node, effective, false); err != nil {
			return false, err
		}
		if node.Enabled != nodeRecord.GetEnabled() {
			node.Status = model.ForwardNodeStatusOffline
			if nodeRecord.GetEnabled() {
				node.Status = model.ForwardNodeStatusOnline
			}
		}
		node.Enabled, node.UpdatedAt = nodeRecord.GetEnabled(), now
		if err := service.NewForwardNodeService(tx).Update(&node); err != nil {
			return false, fmt.Errorf("kernel forward: update forward node: %w", err)
		}
		return false, nil
	}, func(tx *gorm.DB, outcome PlanOutcome, now time.Time) error {
		node, err := agentcontrol.ParseAgentNode(nodeRef)
		if err != nil {
			return err
		}
		summary, err := nodeSummary(tx, node)
		if err != nil {
			return err
		}
		response.Node, response.Violations = summary, outcomeViolations(outcome)
		return record(tx, requestID, methodUpdateNode, nodeRef, hash, response, now)
	})
	if err != nil {
		return nil, err
	}
	return response, nil
}

// DeleteForwardNode removes a forward node, its inventory entry, desired
// state and report, its credential and its Agent certificates, as
// ForwardControl.DeleteForwardNode.
func (s *Service) DeleteForwardNode(ctx context.Context, requestID string, id uint64) error {
	requestID, err := checkRequestID(requestID)
	if err != nil {
		return err
	}
	if id == 0 || id > maxNodeID {
		return fmt.Errorf("%w: node id is required", ErrInvalidRequest)
	}
	hash := requestHash(methodDeleteNode, &forwardv1.DeleteForwardNodeRequest{Id: id})
	nodeRef := fmt.Sprintf("forward-%d", id)
	return s.nodeWrite(ctx, "node_deleted", func(tx *gorm.DB, _ time.Time) (bool, error) {
		if found, err := replay(tx, requestID, methodDeleteNode, hash, &forwardv1.DeleteForwardNodeResponse{}); err != nil || found {
			return found, err
		}
		if _, err := loadForwardNode(tx, id); err != nil {
			return false, err
		}
		using, err := routesUsing(tx, nodeRef)
		if err != nil {
			return false, err
		}
		if len(using) > 0 {
			return false, &RefusedError{Violations: using, Precondition: true}
		}
		if err := service.NewForwardNodeService(tx).Delete(uint(id)); err != nil {
			return false, fmt.Errorf("kernel forward: delete forward node: %w", err)
		}
		for _, row := range []any{&model.KernelForwardNode{}, &model.KernelForwardNodeState{}, &model.KernelForwardNodeReport{}} {
			if err := tx.Where("node_ref = ?", nodeRef).Delete(row).Error; err != nil {
				return false, fmt.Errorf("kernel forward: delete node rows: %w", err)
			}
		}
		return false, nil
	}, func(tx *gorm.DB, _ PlanOutcome, now time.Time) error {
		return record(tx, requestID, methodDeleteNode, nodeRef, hash, &forwardv1.DeleteForwardNodeResponse{}, now)
	})
}

const (
	// MaxTrafficWindow bounds GetTraffic's window.
	MaxTrafficWindow = 31 * 24 * time.Hour
	// MaxTrafficBuckets bounds one GetTraffic answer.
	MaxTrafficBuckets = 20000
)

// TrafficBuckets answers the ledger's hourly buckets filtered by route and
// node from since (inclusive) to until (exclusive), as
// ForwardControl.GetTraffic; zero times default to the 24 hours up to the end of the current hour.
func (s *Service) TrafficBuckets(ctx context.Context, routeID, nodeRef string, since, until time.Time) ([]*forwardv1.TrafficBucket, bool, error) {
	db, err := s.db(ctx)
	if err != nil {
		return nil, false, err
	}
	if until.IsZero() {
		// The current hour's bucket included.
		until = s.now().Truncate(time.Hour).Add(time.Hour)
	}
	if since.IsZero() {
		since = until.Add(-24 * time.Hour)
	}
	if !since.Before(until) || until.Sub(since) > MaxTrafficWindow {
		return nil, false, fmt.Errorf("%w: since must be before until, at most 31 days apart", ErrInvalidRequest)
	}
	query := db.Where("hour_start >= ? AND hour_start < ?", since.UTC(), until.UTC())
	if routeID != "" {
		query = query.Where("route_id = ?", routeID)
	}
	if nodeRef != "" {
		query = query.Where("node_ref = ?", nodeRef)
	}
	var rows []model.KernelForwardTraffic
	if err := query.Order("hour_start, route_id, hop_index, node_ref").Limit(MaxTrafficBuckets + 1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	truncated := len(rows) > MaxTrafficBuckets
	if truncated {
		rows = rows[:MaxTrafficBuckets]
	}
	out := make([]*forwardv1.TrafficBucket, 0, len(rows))
	for _, row := range rows {
		out = append(out, &forwardv1.TrafficBucket{
			RouteId: row.RouteID, HopIndex: row.HopIndex, NodeRef: row.NodeRef, HourStartUnixMs: row.HourStart.UnixMilli(),
			UpBytes: row.UpBytes, DownBytes: row.DownBytes, UpPackets: row.UpPackets, DownPackets: row.DownPackets, NewConns: row.Conns,
		})
	}
	return out, truncated, nil
}

// Enforcement answers why Control pauses each of routeIDs that it pauses
// (EnforcedQuota, EnforcedExpired).
func (s *Service) Enforcement(ctx context.Context, routeIDs []string) (map[string]string, error) {
	out := map[string]string{}
	if len(routeIDs) == 0 {
		return out, nil
	}
	db, err := s.db(ctx)
	if err != nil {
		return nil, err
	}
	var rows []model.KernelForwardRoute
	if err := db.Select("id", "enforced").Where("id IN ? AND enforced <> ''", routeIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.ID] = row.Enforced
	}
	return out, nil
}
