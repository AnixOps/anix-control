package agenttransport

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// PersistInterval is how often at most one node's sightings on one
// transport are written to v4_kernel_agent_transport. Between writes the
// live value stays in memory; a change of version or identity is written at
// once.
const PersistInterval = time.Minute

// persistTimeout bounds one write, which runs on the request's path.
const persistTimeout = 2 * time.Second

// Sighting is one authenticated request of a node on a transport.
type Sighting struct {
	Node      agentcontrol.AgentNode
	Transport string
	// AgentVersion, Identity, CertSerial and CertNotAfter are kept when
	// set; an empty value keeps what was recorded before.
	AgentVersion string
	Identity     string
	CertSerial   string
	CertNotAfter *time.Time
}

type sightingKey struct {
	kind      string
	id        uint32
	transport string
}

type sightingEntry struct {
	row         model.AgentTransport
	persistedAt time.Time
	dirty       bool
	writing     bool
}

// Recorder keeps the last sighting of each node and transport in memory
// and writes it through to the database at most once per PersistInterval.
type Recorder struct {
	db       func() *gorm.DB
	now      func() time.Time
	interval time.Duration

	mu      sync.Mutex
	entries map[sightingKey]*sightingEntry
}

// NewRecorder returns a recorder writing to db (nil db writes nothing).
func NewRecorder(db func() *gorm.DB) *Recorder {
	return &Recorder{db: db, now: time.Now, interval: PersistInterval, entries: map[sightingKey]*sightingEntry{}}
}

var defaultRecorder = NewRecorder(database.Get)

// Default returns the process recorder, which writes to the kernel
// database.
func Default() *Recorder { return defaultRecorder }

// SetDefault replaces the process recorder and returns a function that
// restores the previous one (tests).
func SetDefault(r *Recorder) (restore func()) {
	previous := defaultRecorder
	defaultRecorder = r
	return func() { defaultRecorder = previous }
}

// Seen records a sighting on the process recorder.
func Seen(ctx context.Context, sighting Sighting) { Default().Seen(ctx, sighting) }

// Seen records a sighting: in memory always, in the database when the
// node's row on that transport is older than the interval or its version or
// identity changed. A failed write is logged and retried at the next
// sighting after the interval.
func (r *Recorder) Seen(ctx context.Context, sighting Sighting) {
	if r == nil || sighting.Node.ID == 0 || sighting.Transport == "" ||
		(sighting.Node.Kind != agentcontrol.NodeKindProxy && sighting.Node.Kind != agentcontrol.NodeKindForward) {
		return
	}
	now := r.now().UTC()
	key := sightingKey{kind: sighting.Node.Kind, id: sighting.Node.ID, transport: sighting.Transport}

	r.mu.Lock()
	entry := r.entries[key]
	if entry == nil {
		entry = &sightingEntry{row: model.AgentTransport{
			NodeKind: key.kind, NodeID: uint(key.id), Transport: key.transport, FirstSeenAt: now,
		}}
		r.entries[key] = entry
	}
	row := &entry.row
	changed := false
	if sighting.AgentVersion != "" && sighting.AgentVersion != row.AgentVersion {
		row.AgentVersion, changed = truncate(sighting.AgentVersion, 64), true
	}
	if sighting.Identity != "" && sighting.Identity != row.Identity {
		row.Identity, changed = truncate(sighting.Identity, 255), true
	}
	if sighting.CertSerial != "" && sighting.CertSerial != row.CertSerial {
		row.CertSerial, changed = truncate(sighting.CertSerial, 40), true
	}
	if sighting.CertNotAfter != nil && (row.CertNotAfter == nil || !row.CertNotAfter.Equal(*sighting.CertNotAfter)) {
		notAfter := sighting.CertNotAfter.UTC()
		row.CertNotAfter, changed = &notAfter, true
	}
	row.LastSeenAt = now
	entry.dirty = true
	due := changed || entry.persistedAt.IsZero() || now.Sub(entry.persistedAt) >= r.interval
	if !due || entry.writing {
		r.mu.Unlock()
		return
	}
	entry.writing, entry.dirty, entry.persistedAt = true, false, now
	snapshot := *row
	r.mu.Unlock()

	err := r.persist(ctx, snapshot)

	r.mu.Lock()
	entry.writing = false
	if err != nil {
		entry.dirty = true
	}
	r.mu.Unlock()
	if err != nil {
		slog.Debug("agent transport sighting not recorded", "component", "agent-transport",
			"node", sighting.Node.String(), "transport", sighting.Transport, "error", err)
	}
}

func (r *Recorder) persist(ctx context.Context, row model.AgentTransport) error {
	if r.db == nil {
		return nil
	}
	db := r.db()
	if db == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	// The request may end before the write; the sighting still counts.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), persistTimeout)
	defer cancel()
	updates := map[string]any{"last_seen_at": row.LastSeenAt}
	if row.AgentVersion != "" {
		updates["agent_version"] = row.AgentVersion
	}
	if row.Identity != "" {
		updates["identity"] = row.Identity
	}
	if row.CertSerial != "" {
		updates["cert_serial"] = row.CertSerial
	}
	if row.CertNotAfter != nil {
		updates["cert_not_after"] = *row.CertNotAfter
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "node_kind"}, {Name: "node_id"}, {Name: "transport"}},
		DoUpdates: clause.Assignments(updates),
	}).Create(&row).Error
}

// Snapshot returns the in-memory sightings, newest data of this process.
func (r *Recorder) Snapshot() []model.AgentTransport {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	rows := make([]model.AgentTransport, 0, len(r.entries))
	for _, entry := range r.entries {
		rows = append(rows, entry.row)
	}
	return rows
}

// Flush writes every sighting not yet written (shutdown, tests).
func (r *Recorder) Flush(ctx context.Context) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	var pending []*sightingEntry
	var rows []model.AgentTransport
	for _, entry := range r.entries {
		if entry.dirty && !entry.writing {
			pending, rows = append(pending, entry), append(rows, entry.row)
			entry.dirty, entry.persistedAt = false, r.now().UTC()
		}
	}
	r.mu.Unlock()
	var firstErr error
	for i, row := range rows {
		if err := r.persist(ctx, row); err != nil {
			r.mu.Lock()
			pending[i].dirty = true
			r.mu.Unlock()
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
