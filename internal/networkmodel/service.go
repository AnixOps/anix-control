package networkmodel

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
	"github.com/AnixOps/anix-control/sdk/network"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	modelRowID = 1
	// linkKeyAdditionalData prefixes the transport id in a sealed link
	// key's additional data.
	linkKeyAdditionalData = "anixops-network-link-key:"

	auditModule = "network"
	auditTarget = "network_model"
	// AuditActionReplace and AuditActionRotate are the operation log
	// actions of a document write and a link key rotation.
	AuditActionReplace = "network_model_replace"
	AuditActionRotate  = "network_link_key_rotate"
)

var (
	// ErrConflict: the document changed since the revision the writer read.
	ErrConflict = errors.New("network model: the document changed since it was read")
	// ErrNoKEK: link keys are sealed with module_runtime.ca_kek, which is
	// not set.
	ErrNoKEK = errors.New("set module_runtime.ca_kek: ANX-Line link keys are sealed with Control's key-encryption key")
	// ErrNotFound: no such transport.
	ErrNotFound = errors.New("network model: not found")
	// ErrUnknownNode: a line names a node that does not exist.
	ErrUnknownNode = errors.New("network model: unknown node")
)

// Actor is who made an administrator's change, for the audit entry.
type Actor struct {
	UserID uint
	Name   string
	IP     string
}

// Service is the network model on one database.
type Service struct {
	DB *gorm.DB
	// KEK answers module_runtime.ca_kek; nil reads the configuration.
	KEK func() string
	// Now defaults to time.Now.
	Now func() time.Time
	// Forwards answers the forward routes on the network model (forward.v1
	// ENGINE_NETWORK); nil asks the source set by SetForwardSource.
	Forwards func(db *gorm.DB) ([]Forward, error)
}

var (
	forwardSourceMu sync.RWMutex
	forwardSource   func(db *gorm.DB) ([]Forward, error)
)

// SetForwardSource sets where every Service without Forwards gets the
// forward routes: the kernel forward service, which owns the routes. Nil
// has none.
func SetForwardSource(source func(db *gorm.DB) ([]Forward, error)) {
	forwardSourceMu.Lock()
	defer forwardSourceMu.Unlock()
	forwardSource = source
}

func (s *Service) forwards(db *gorm.DB) ([]Forward, error) {
	source := s.Forwards
	if source == nil {
		forwardSourceMu.RLock()
		source = forwardSource
		forwardSourceMu.RUnlock()
	}
	if source == nil {
		return nil, nil
	}
	return source(db)
}

// ReplanTx replans every device in tx, which the caller holds, after the
// forward routes changed; notify tells the devices whose plan changed and
// is called once tx commits.
func (s *Service) ReplanTx(tx *gorm.DB) (notifyChanged func(), err error) {
	_, doc, err := lockModel(tx, s.now())
	if err != nil {
		return nil, err
	}
	changed, err := s.replan(tx, doc)
	if err != nil {
		return nil, err
	}
	return func() { notify(changed) }, nil
}

// New returns the service on db.
func New(db *gorm.DB) *Service { return &Service{DB: db} }

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// DeviceView is a device as the API shows it.
type DeviceView struct {
	Node       string     `json:"node"`
	Generation uint64     `json:"generation"`
	PlanHash   string     `json:"plan_hash"`
	Negotiated bool       `json:"negotiated"`
	PublicKeys []string   `json:"public_keys"` // the kinds with a reported key
	StatusAt   *time.Time `json:"status_at"`
	// Status is the device's latest NetworkStatus (protojson).
	Status json.RawMessage `json:"status,omitempty" swaggertype:"object"`
}

// View is the model as the API shows it: the document with its revision,
// the paths left out of the plans for now, and the devices.
type View struct {
	Revision uint64       `json:"revision"`
	Document Document     `json:"document"`
	Pending  []Pending    `json:"pending"`
	Devices  []DeviceView `json:"devices"`
	// Nodes are the devices a route may use (Agent nodes), whether or not
	// the document has lines on them.
	Nodes []AgentNode `json:"nodes"`
}

// Get answers the model.
func (s *Service) Get(ctx context.Context) (*View, error) {
	db := s.DB.WithContext(ctx)
	row, doc, err := loadDocument(db)
	if err != nil {
		return nil, err
	}
	devices, rows, err := loadDevices(db)
	if err != nil {
		return nil, err
	}
	nodes, err := agentNodes(db)
	if err != nil {
		return nil, err
	}
	view := &View{Revision: row.Revision, Document: *doc, Pending: []Pending{}, Devices: deviceViews(rows), Nodes: nodes}
	if view.Nodes == nil {
		view.Nodes = []AgentNode{}
	}
	forwards, err := s.forwards(db)
	if err != nil {
		return nil, err
	}
	facts, err := devicesFor(db, forwards)
	if err != nil {
		return nil, err
	}
	full, forwards := complete(doc, forwards, facts)
	if keys, err := s.openKeys(db); err == nil {
		if result, err := Plan(full, keys, devices, forwards); err == nil {
			view.Pending = append(view.Pending, result.Pending...)
		}
	}
	return view, nil
}

// Replace makes doc the model, audited. ifRevision, when not nil, must be
// the stored revision. Every anx_line transport gets a link key the first
// time it appears; a removed transport's key is deleted. The document is
// refused (an *Invalid) unless every device's plan passes the planner;
// each device whose plan changed gets a new generation.
func (s *Service) Replace(ctx context.Context, doc *Document, ifRevision *uint64, actor Actor) (*View, error) {
	normalize(doc)
	var changed []string
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, _, err := lockModel(tx, s.now())
		if err != nil {
			return err
		}
		if ifRevision != nil && *ifRevision != row.Revision {
			return ErrConflict
		}
		if err := checkNoAuto(doc); err != nil {
			return err
		}
		if err := requireNodes(tx, doc); err != nil {
			return err
		}
		if err := s.syncKeys(tx, doc); err != nil {
			return err
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			return err
		}
		row.Revision++
		row.DocumentJSON, row.UpdatedAt = string(encoded), s.now()
		if err := tx.Save(row).Error; err != nil {
			return err
		}
		if changed, err = s.replan(tx, doc); err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionReplace, map[string]any{
			"revision": row.Revision, "lines": len(doc.Lines), "ingresses": len(doc.Ingresses), "egresses": len(doc.Egresses),
			"transports": len(doc.Transports), "paths": len(doc.Paths),
		})
	})
	if err != nil {
		return nil, err
	}
	notify(changed)
	return s.Get(ctx)
}

// RotateLinkKey gives a transport a new link key, audited, and replans
// its two devices.
func (s *Service) RotateLinkKey(ctx context.Context, transportID string, actor Actor) (*View, error) {
	var changed []string
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, doc, err := lockModel(tx, s.now())
		if err != nil {
			return err
		}
		found := false
		for _, transport := range doc.Transports {
			found = found || (transport.ID == transportID && isLink(transport.Kind))
		}
		if !found {
			return ErrNotFound
		}
		sealer, err := s.sealer()
		if err != nil {
			return err
		}
		sealed, err := sealer.newKey(transportID)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.NetworkTransportKey{}).Where("transport_id = ?", transportID).
			Updates(map[string]any{"sealed_key": sealed, "updated_at": s.now()}).Error; err != nil {
			return err
		}
		if changed, err = s.replan(tx, doc); err != nil {
			return err
		}
		return writeAudit(tx, actor, AuditActionRotate, map[string]any{"transport": transportID})
	})
	if err != nil {
		return nil, err
	}
	notify(changed)
	return s.Get(ctx)
}

// SetPublicKeys records the public keys a device reported, by kind (the
// anx_line accepting end's server key), and replans when they changed:
// paths waiting for the key are planned.
func (s *Service) SetPublicKeys(ctx context.Context, node string, keys map[string]string) error {
	encoded, err := json.Marshal(keys)
	if err != nil {
		return err
	}
	var changed []string
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, doc, err := lockModel(tx, s.now())
		if err != nil {
			return err
		}
		var rows []model.NetworkDevice
		if err := tx.Where("node_ref = ?", node).Limit(1).Find(&rows).Error; err != nil {
			return err
		}
		row := model.NetworkDevice{NodeRef: node}
		if len(rows) > 0 {
			row = rows[0]
			if row.PublicKeysJSON == string(encoded) {
				return nil
			}
		}
		row.PublicKeysJSON, row.UpdatedAt = string(encoded), s.now()
		if err := tx.Save(&row).Error; err != nil {
			return err
		}
		changed, err = s.replan(tx, doc)
		return err
	})
	if err == nil {
		notify(changed)
	}
	return err
}

// DevicePlan answers a device's current plan, stamped, for its
// configuration. found is false when the model gives the device nothing
// and never did. Link keys are in it: it is for the device only.
func (s *Service) DevicePlan(ctx context.Context, node string) (plan *networkv1.NetworkPlan, found bool, err error) {
	db := s.DB.WithContext(ctx)
	var rows []model.NetworkDevice
	if err := db.Where("node_ref = ?", node).Limit(1).Find(&rows).Error; err != nil {
		return nil, false, err
	}
	_, doc, err := loadDocument(db)
	if err != nil {
		return nil, false, err
	}
	devices, _, err := loadDevices(db)
	if err != nil {
		return nil, false, err
	}
	keys, err := s.openKeys(db)
	if err != nil {
		return nil, false, err
	}
	full, forwards, err := s.completed(db, doc)
	if err != nil {
		return nil, false, err
	}
	result, err := Plan(full, keys, devices, forwards)
	if err != nil {
		return nil, false, err
	}
	plan = result.Plans[node]
	if plan == nil {
		if len(rows) == 0 || rows[0].Generation == 0 {
			return nil, false, nil
		}
		plan = &networkv1.NetworkPlan{DeviceRef: node}
	}
	previous := network.Generation{}
	if len(rows) > 0 {
		previous = network.Generation{Generation: rows[0].Generation, Hash: rows[0].PlanHash}
	}
	network.Stamp(plan, previous)
	return plan, true, nil
}

// Redacted answers a copy of plan without its link keys, for people.
func Redacted(plan *networkv1.NetworkPlan) *networkv1.NetworkPlan {
	out := proto.Clone(plan).(*networkv1.NetworkPlan)
	for _, transport := range out.GetTransports() {
		if anx := transport.GetAnxLine(); anx != nil && anx.LinkKey != "" {
			anx.LinkKey = "redacted"
		}
	}
	return out
}

// replan plans every device and stamps the generations: a device whose
// plan hash changed gets the next generation, and a device that had a
// plan and has none now gets an empty one.
func (s *Service) replan(tx *gorm.DB, doc *Document) ([]string, error) {
	full, forwards, err := s.completed(tx, doc)
	if err != nil {
		return nil, err
	}
	if err := s.syncAutoKeys(tx, full); err != nil {
		return nil, err
	}
	keys, err := s.openKeys(tx)
	if err != nil {
		return nil, err
	}
	devices, rows, err := loadDevices(tx)
	if err != nil {
		return nil, err
	}
	result, err := Plan(full, keys, devices, forwards)
	if err != nil {
		return nil, err
	}
	var changed []string
	stored := map[string]model.NetworkDevice{}
	for _, row := range rows {
		stored[row.NodeRef] = row
	}
	nodes := map[string]bool{}
	for node := range result.Plans {
		nodes[node] = true
	}
	for node, row := range stored {
		if row.Generation > 0 {
			nodes[node] = true
		}
	}
	now := s.now()
	for node := range nodes {
		plan := result.Plans[node]
		if plan == nil {
			plan = &networkv1.NetworkPlan{DeviceRef: node}
		}
		row, had := stored[node]
		g := network.Stamp(plan, network.Generation{Generation: row.Generation, Hash: row.PlanHash})
		if had && g.Generation == row.Generation && g.Hash == row.PlanHash {
			continue
		}
		row.NodeRef, row.Generation, row.PlanHash, row.UpdatedAt = node, g.Generation, g.Hash, now
		if err := tx.Save(&row).Error; err != nil {
			return nil, err
		}
		changed = append(changed, node)
	}
	sort.Strings(changed)
	return changed, nil
}

// syncKeys gives every anx_line transport a sealed link key and deletes
// the keys of transports that are gone.
func (s *Service) syncKeys(tx *gorm.DB, doc *Document) error {
	var rows []model.NetworkTransportKey
	if err := tx.Find(&rows).Error; err != nil {
		return err
	}
	have := map[string]bool{}
	for _, row := range rows {
		have[row.TransportID] = true
	}
	want := map[string]bool{}
	var sealer *keySealer
	for _, transport := range doc.Transports {
		if !isLink(transport.Kind) || !idPattern.MatchString(transport.ID) {
			continue
		}
		want[transport.ID] = true
		if have[transport.ID] {
			continue
		}
		if sealer == nil {
			var err error
			if sealer, err = s.sealer(); err != nil {
				return err
			}
		}
		sealed, err := sealer.newKey(transport.ID)
		if err != nil {
			return err
		}
		now := s.now()
		if err := tx.Create(&model.NetworkTransportKey{TransportID: transport.ID, SealedKey: sealed, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
	}
	var gone []string
	for id := range have {
		if !want[id] && !strings.HasPrefix(id, AutoPrefix) {
			gone = append(gone, id)
		}
	}
	if len(gone) > 0 {
		return tx.Where("transport_id IN ?", gone).Delete(&model.NetworkTransportKey{}).Error
	}
	return nil
}

// completed answers doc with Control's auto components and the forward
// routes on it (complete).
func (s *Service) completed(db *gorm.DB, doc *Document) (*Document, []Forward, error) {
	forwards, err := s.forwards(db)
	if err != nil {
		return nil, nil, err
	}
	facts, err := devicesFor(db, forwards)
	if err != nil {
		return nil, nil, err
	}
	full, forwards := complete(doc, forwards, facts)
	return full, forwards, nil
}

// syncAutoKeys gives every auto transport of full a sealed link key and
// deletes the keys of auto transports that are gone. Without a
// key-encryption key the auto transports have none, and their routes
// wait.
func (s *Service) syncAutoKeys(tx *gorm.DB, full *Document) error {
	var rows []model.NetworkTransportKey
	if err := tx.Where("transport_id LIKE ?", AutoPrefix+"%").Find(&rows).Error; err != nil {
		return err
	}
	have := map[string]bool{}
	for _, row := range rows {
		have[row.TransportID] = true
	}
	want := map[string]bool{}
	var sealer *keySealer
	for _, transport := range full.Transports {
		if !strings.HasPrefix(transport.ID, AutoPrefix) || !idPattern.MatchString(transport.ID) {
			continue
		}
		want[transport.ID] = true
		if have[transport.ID] {
			continue
		}
		if sealer == nil {
			var err error
			if sealer, err = s.sealer(); errors.Is(err, ErrNoKEK) {
				return nil
			} else if err != nil {
				return err
			}
		}
		sealed, err := sealer.newKey(transport.ID)
		if err != nil {
			return err
		}
		now := s.now()
		if err := tx.Create(&model.NetworkTransportKey{TransportID: transport.ID, SealedKey: sealed, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return err
		}
	}
	var gone []string
	for id := range have {
		if !want[id] {
			gone = append(gone, id)
		}
	}
	if len(gone) > 0 {
		return tx.Where("transport_id IN ?", gone).Delete(&model.NetworkTransportKey{}).Error
	}
	return nil
}

// openKeys unseals every link key. Without a key-encryption key there are
// none; a key that does not unseal (a database restored under another
// key) is left out, so its paths wait until the key is rotated.
func (s *Service) openKeys(db *gorm.DB) (map[string]string, error) {
	var rows []model.NetworkTransportKey
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	keys := map[string]string{}
	if len(rows) == 0 {
		return keys, nil
	}
	sealer, err := s.sealer()
	if errors.Is(err, ErrNoKEK) {
		return keys, nil
	}
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if key, err := sealer.open(row); err == nil {
			keys[row.TransportID] = hex.EncodeToString(key)
		}
	}
	return keys, nil
}

// lockModel creates the model row when missing and locks it, so writers
// of the model and the plans take turns.
func lockModel(tx *gorm.DB, now time.Time) (*model.NetworkModel, *Document, error) {
	row := model.NetworkModel{ID: modelRowID, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
		return nil, nil, fmt.Errorf("network model: create the model row: %w", err)
	}
	return loadDocument(tx.Clauses(clause.Locking{Strength: "UPDATE"}))
}

func loadDocument(db *gorm.DB) (*model.NetworkModel, *Document, error) {
	var rows []model.NetworkModel
	if err := db.Where("id = ?", modelRowID).Limit(1).Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	doc := &Document{}
	if len(rows) == 0 {
		normalize(doc)
		return &model.NetworkModel{ID: modelRowID}, doc, nil
	}
	if rows[0].DocumentJSON != "" {
		if err := json.Unmarshal([]byte(rows[0].DocumentJSON), doc); err != nil {
			return nil, nil, fmt.Errorf("network model: stored document is corrupt: %w", err)
		}
	}
	normalize(doc)
	return &rows[0], doc, nil
}

func loadDevices(db *gorm.DB) (map[string]Device, []model.NetworkDevice, error) {
	var rows []model.NetworkDevice
	if err := db.Order("node_ref").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	devices := map[string]Device{}
	for _, row := range rows {
		keys := map[string]string{}
		if row.PublicKeysJSON != "" {
			_ = json.Unmarshal([]byte(row.PublicKeysJSON), &keys)
		}
		devices[row.NodeRef] = Device{PublicKeys: keys}
	}
	return devices, rows, nil
}

func deviceViews(rows []model.NetworkDevice) []DeviceView {
	views := make([]DeviceView, 0, len(rows))
	for _, row := range rows {
		keys := map[string]string{}
		if row.PublicKeysJSON != "" {
			_ = json.Unmarshal([]byte(row.PublicKeysJSON), &keys)
		}
		kinds := make([]string, 0, len(keys))
		for kind := range keys {
			kinds = append(kinds, kind)
		}
		sort.Strings(kinds)
		views = append(views, DeviceView{Node: row.NodeRef, Generation: row.Generation, PlanHash: row.PlanHash,
			Negotiated: row.Negotiated, PublicKeys: kinds, StatusAt: row.StatusAt, Status: statusView(row.StatusJSON)})
	}
	return views
}

// requireNodes checks that every line's node exists.
func requireNodes(tx *gorm.DB, doc *Document) error {
	seen := map[string]bool{}
	for _, line := range doc.Lines {
		if seen[line.Node] {
			continue
		}
		seen[line.Node] = true
		node, err := agentcontrol.ParseAgentNode(line.Node)
		if err != nil || node.ID == 0 {
			continue // Plan names the bad reference
		}
		var count int64
		switch node.Kind {
		case agentcontrol.NodeKindProxy:
			err = tx.Model(&model.Node{}).Where("id = ?", node.ID).Count(&count).Error
		case agentcontrol.NodeKindForward:
			err = tx.Model(&model.ForwardNode{}).Where("id = ?", node.ID).Count(&count).Error
		}
		if err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("%w: %s", ErrUnknownNode, line.Node)
		}
	}
	return nil
}

// normalize trims the document's strings and gives it empty lists, so it
// encodes the same however it was written.
func normalize(doc *Document) {
	if doc.Lines == nil {
		doc.Lines = []Line{}
	}
	if doc.Ingresses == nil {
		doc.Ingresses = []Ingress{}
	}
	if doc.Egresses == nil {
		doc.Egresses = []Egress{}
	}
	if doc.Transports == nil {
		doc.Transports = []Transport{}
	}
	if doc.Paths == nil {
		doc.Paths = []Path{}
	}
	for i := range doc.Lines {
		l := &doc.Lines[i]
		l.ID, l.Node, l.Interface = strings.TrimSpace(l.ID), strings.TrimSpace(l.Node), strings.TrimSpace(l.Interface)
	}
	for i := range doc.Ingresses {
		in := &doc.Ingresses[i]
		in.ID, in.Line, in.PublicHost = strings.TrimSpace(in.ID), strings.TrimSpace(in.Line), strings.TrimSpace(in.PublicHost)
	}
	for i := range doc.Egresses {
		e := &doc.Egresses[i]
		e.ID, e.Line, e.Source = strings.TrimSpace(e.ID), strings.TrimSpace(e.Line), strings.TrimSpace(e.Source)
	}
	for i := range doc.Transports {
		t := &doc.Transports[i]
		t.ID, t.FromEgress, t.ToIngress = strings.TrimSpace(t.ID), strings.TrimSpace(t.FromEgress), strings.TrimSpace(t.ToIngress)
	}
	for i := range doc.Paths {
		p := &doc.Paths[i]
		p.ID, p.Ingress, p.Egress = strings.TrimSpace(p.ID), strings.TrimSpace(p.Ingress), strings.TrimSpace(p.Egress)
	}
}

func writeAudit(tx *gorm.DB, actor Actor, action string, content map[string]any) error {
	encoded, err := json.Marshal(content)
	if err != nil {
		return err
	}
	var userID *uint
	if actor.UserID != 0 {
		id := actor.UserID
		userID = &id
	}
	return tx.Create(&model.OperationLog{
		UserID: userID, Username: truncate(actor.Name, 100), Action: action, Module: auditModule,
		TargetType: auditTarget, Content: string(encoded), IP: truncate(actor.IP, 45), Status: 1,
	}).Error
}

func truncate(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

// keySealer seals link keys with AES-256-GCM under the key-encryption key
// module_runtime.ca_kek, with additional data naming the transport.
type keySealer struct{ aead cipher.AEAD }

func (s *Service) sealer() (*keySealer, error) {
	var raw string
	if s.KEK != nil {
		raw = s.KEK()
	} else if cfg := config.Get(); cfg != nil {
		raw = cfg.ModuleRuntime.CAKEK
	}
	if strings.TrimSpace(raw) == "" {
		return nil, ErrNoKEK
	}
	kek, err := modulepki.ParseKEK(raw)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &keySealer{aead: aead}, nil
}

// newKey answers a new random 32-byte link key, sealed.
func (k *keySealer) newKey(transportID string) (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(k.aead.Seal(nonce, nonce, key, []byte(linkKeyAdditionalData+transportID))), nil
}

func (k *keySealer) open(row model.NetworkTransportKey) ([]byte, error) {
	sealed, err := base64.StdEncoding.DecodeString(row.SealedKey)
	if err != nil || len(sealed) < k.aead.NonceSize() {
		return nil, fmt.Errorf("network link key %s is corrupt", row.TransportID)
	}
	key, err := k.aead.Open(nil, sealed[:k.aead.NonceSize()], sealed[k.aead.NonceSize():], []byte(linkKeyAdditionalData+row.TransportID))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("network link key %s cannot be unsealed: wrong key-encryption key", row.TransportID)
	}
	return key, nil
}
