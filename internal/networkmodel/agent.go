package networkmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"google.golang.org/protobuf/encoding/protojson"
	"gorm.io/gorm"
)

// The Agent Control stream's side of the model (network.v1; sdk/network,
// internal/grpc agent_control_network.go):
//
//   - RecordHello keeps whether the device's last Hello negotiated
//     network.v1 and the DeviceCapabilities it offered.
//   - NodeConfigMember answers the "network" member of the device's
//     configuration document while that flag is set: its stamped plan.
//   - RecordStatus keeps the device's latest NetworkStatus and its public
//     keys; a new key replans (a pending path is planned).
//   - OnPlanChange tells the stream which devices' plans moved, after the
//     write that moved them committed, so it pushes their configurations.

var protojsonWrite = protojson.MarshalOptions{UseProtoNames: true}

// PlanListener receives the devices whose plan moved to a new generation.
type PlanListener func(nodes []agentcontrol.AgentNode)

var (
	listenersMu sync.Mutex
	listeners   = map[int]PlanListener{}
	listenerID  int
)

// OnPlanChange registers listener for every committed generation change of
// every Service in the process. cancel removes it.
func OnPlanChange(listener PlanListener) (cancel func()) {
	listenersMu.Lock()
	defer listenersMu.Unlock()
	listenerID++
	id := listenerID
	listeners[id] = listener
	return func() {
		listenersMu.Lock()
		delete(listeners, id)
		listenersMu.Unlock()
	}
}

func notify(refs []string) {
	if len(refs) == 0 {
		return
	}
	nodes := make([]agentcontrol.AgentNode, 0, len(refs))
	for _, ref := range refs {
		if node, err := agentcontrol.ParseAgentNode(ref); err == nil {
			nodes = append(nodes, node)
		}
	}
	listenersMu.Lock()
	ids := make([]int, 0, len(listeners))
	for id := range listeners {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	current := make([]PlanListener, 0, len(ids))
	for _, id := range ids {
		current = append(current, listeners[id])
	}
	listenersMu.Unlock()
	for _, listener := range current {
		listener(nodes)
	}
}

// RecordHello records a Hello of node: caps when the session negotiated
// network.v1, nil when it did not (the flag is cleared, so the device's
// configuration loses the member). A device without a row and without
// network.v1 gets no row.
func (s *Service) RecordHello(ctx context.Context, node agentcontrol.AgentNode, caps *networkv1.DeviceCapabilities) error {
	db := s.DB.WithContext(ctx)
	if !db.Migrator().HasTable(&model.NetworkDevice{}) {
		return nil
	}
	encoded := ""
	if caps != nil {
		raw, err := protojsonWrite.Marshal(caps)
		if err != nil {
			return err
		}
		encoded = string(raw)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		var rows []model.NetworkDevice
		if err := tx.Where("node_ref = ?", node.String()).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 && caps == nil {
			return nil
		}
		row := model.NetworkDevice{NodeRef: node.String()}
		if len(rows) > 0 {
			row = rows[0]
		}
		if row.Negotiated == (caps != nil) && row.CapabilitiesJSON == encoded && len(rows) > 0 {
			return nil
		}
		row.Negotiated, row.CapabilitiesJSON, row.UpdatedAt = caps != nil, encoded, s.now()
		return tx.Save(&row).Error
	})
}

// RecordStatus records node's NetworkStatus (checked by the caller with
// sdk/network.DecodeStatus) as its latest, and its public keys: a changed
// key replans.
func (s *Service) RecordStatus(ctx context.Context, node agentcontrol.AgentNode, status *networkv1.NetworkStatus, observedAt time.Time) error {
	encoded, err := protojsonWrite.Marshal(status)
	if err != nil {
		return err
	}
	keys := map[string]string{}
	for _, key := range status.GetPublicKeys() {
		keys[key.GetKind()] = key.GetKey()
	}
	if err := s.SetPublicKeys(ctx, node.String(), keys); err != nil {
		return err
	}
	at := observedAt.UTC()
	if err := s.DB.WithContext(ctx).Model(&model.NetworkDevice{}).Where("node_ref = ?", node.String()).
		Updates(map[string]any{"status_json": string(encoded), "status_at": &at}).Error; err != nil {
		return err
	}
	// Auto mode follows the uplinks' probes; a failure keeps the
	// uplinks as they are and the status recorded.
	if err := s.evaluateUplinks(ctx, node.String(), status); err != nil {
		slog.Warn("network model: uplinks were not evaluated", "component", "network-model", "node", node.String(), "error", err)
	}
	return nil
}

// Status answers node's latest NetworkStatus, nil without one.
func (s *Service) Status(ctx context.Context, node string) (*networkv1.NetworkStatus, error) {
	status, _, err := s.latestStatus(ctx, node)
	return status, err
}

// latestStatus answers node's latest NetworkStatus and when it was
// observed, nil without one.
func (s *Service) latestStatus(ctx context.Context, node string) (*networkv1.NetworkStatus, *time.Time, error) {
	var rows []model.NetworkDevice
	if err := s.DB.WithContext(ctx).Where("node_ref = ?", node).Limit(1).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 || rows[0].StatusJSON == "" {
		return nil, nil, nil
	}
	status := &networkv1.NetworkStatus{}
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal([]byte(rows[0].StatusJSON), status); err != nil {
		return nil, nil, fmt.Errorf("network model: %s: stored status is corrupt: %w", node, err)
	}
	return status, rows[0].StatusAt, nil
}

// NodeConfigMember answers what a device's configuration document carries
// for the network model: negotiated is the device's recorded flag (its last
// Hello negotiated network.v1); with it, member is the value of the member
// "network", the device's stamped plan, or a plan holding only device_ref
// (generation 0: the model gives the device nothing, and the Agent keeps
// what it runs). Like the forward member, it is decided from the recorded
// flag, never a live session, so every builder writes the same document.
func NodeConfigMember(db *gorm.DB, node agentcontrol.AgentNode) (negotiated bool, member map[string]any, err error) {
	if !db.Migrator().HasTable(&model.NetworkDevice{}) {
		return false, nil, nil
	}
	var rows []model.NetworkDevice
	if err := db.Select("node_ref", "negotiated").Where("node_ref = ?", node.String()).Limit(1).Find(&rows).Error; err != nil {
		return false, nil, fmt.Errorf("network model: %s: %w", node, err)
	}
	if len(rows) == 0 || !rows[0].Negotiated {
		return false, nil, nil
	}
	ctx := db.Statement.Context
	if ctx == nil {
		ctx = context.Background()
	}
	plan, found, err := New(db).DevicePlan(ctx, node.String())
	if err != nil {
		return false, nil, err
	}
	if !found {
		plan = &networkv1.NetworkPlan{DeviceRef: node.String()}
	}
	member, err = network.PlanMember(plan)
	if err != nil {
		return false, nil, err
	}
	return true, member, nil
}

// statusView is a device's latest status as the API shows it.
func statusView(raw string) json.RawMessage {
	if raw == "" {
		return nil
	}
	return json.RawMessage(raw)
}
