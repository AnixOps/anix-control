package destinationstats

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/agentreports"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Bounds of one batch.
const (
	// MaxEntries is the most entries one DestinationReport may carry.
	MaxEntries = 50000
	// MaxDomainLength and MaxProtocolLength are the column sizes.
	MaxDomainLength   = 255
	MaxProtocolLength = 32
	// futureSkew is how far ahead of Control's clock a window may end
	// before it is counted in the current hour instead.
	futureSkew  = 10 * time.Minute
	insertBatch = 500
)

// ErrInvalid refuses a batch for good: an entry without a user, with an
// oversized domain or protocol, or more than MaxEntries entries.
var ErrInvalid = errors.New("invalid destination report")

// ErrOverflow refuses a batch whose bytes exceed the counter range.
var ErrOverflow = errors.New("destination report bytes exceed the counter range")

// Entry is one user's traffic to one destination in a window.
type Entry struct {
	UserID      uint64
	Domain      string
	Protocol    string
	Upload      uint64
	Download    uint64
	Connections uint32
}

type rowKey struct {
	userID           uint
	domain, protocol string
}

// HourOf is the start of the UTC hour a window ending at windowEnd is
// counted in: the hour of its last millisecond, or of now for a window
// without an end or one too far in the future.
func HourOf(windowEnd, now time.Time) int64 {
	if windowEnd.IsZero() || windowEnd.After(now.Add(futureSkew)) {
		windowEnd = now
	} else {
		windowEnd = windowEnd.Add(-time.Millisecond)
	}
	return windowEnd.UTC().Truncate(time.Hour).Unix()
}

// normalize checks the entries and sums those of one key.
func normalize(entries []Entry) (map[rowKey][3]int64, error) {
	if len(entries) > MaxEntries {
		return nil, fmt.Errorf("%w: %d entries, at most %d", ErrInvalid, len(entries), MaxEntries)
	}
	sums := make(map[rowKey][3]int64, len(entries))
	for index, entry := range entries {
		if entry.UserID == 0 || entry.UserID > math.MaxUint32 {
			return nil, fmt.Errorf("%w: entry %d: user_id is required", ErrInvalid, index)
		}
		domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(entry.Domain)), ".")
		protocol := strings.ToLower(strings.TrimSpace(entry.Protocol))
		if len(domain) > MaxDomainLength {
			return nil, fmt.Errorf("%w: entry %d: domain is longer than %d bytes", ErrInvalid, index, MaxDomainLength)
		}
		if len(protocol) > MaxProtocolLength {
			return nil, fmt.Errorf("%w: entry %d: protocol is longer than %d bytes", ErrInvalid, index, MaxProtocolLength)
		}
		if entry.Upload > math.MaxInt64 || entry.Download > math.MaxInt64 {
			return nil, fmt.Errorf("%w: entry %d", ErrOverflow, index)
		}
		key := rowKey{userID: uint(entry.UserID), domain: domain, protocol: protocol}
		sum := sums[key]
		upload, download := int64(entry.Upload), int64(entry.Download)
		if upload > math.MaxInt64-sum[0] || download > math.MaxInt64-sum[1] {
			return nil, fmt.Errorf("%w: entry %d", ErrOverflow, index)
		}
		sums[key] = [3]int64{sum[0] + upload, sum[1] + download, sum[2] + int64(entry.Connections)}
	}
	return sums, nil
}

// Record adds a batch to its hour's rows at most once per node and batch
// id (agentreports), and reports whether this call recorded it. A batch is
// claimed and dropped while the statistics are off, so the Agent does not
// resend it.
func Record(db *gorm.DB, nodeKind string, nodeID uint, batchID string, windowEnd time.Time, entries []Entry, now time.Time) (bool, error) {
	if err := agentreports.ValidateBatchID(batchID); err != nil {
		return false, err
	}
	sums, err := normalize(entries)
	if err != nil {
		return false, err
	}
	settings, err := LoadSettings(db)
	if err != nil {
		return false, err
	}
	hour := HourOf(windowEnd, now)
	rows := make([]model.DestinationStat, 0, len(sums))
	if settings.Enabled {
		for key, sum := range sums {
			rows = append(rows, model.DestinationStat{
				HourAt: hour, NodeID: nodeID, UserID: key.userID, Domain: key.domain, Protocol: key.protocol,
				Upload: sum[0], Download: sum[1], Connections: sum[2],
			})
		}
	}
	applied := false
	err = db.Transaction(func(tx *gorm.DB) error {
		seen, err := agentreports.ClaimTx(tx, nodeKind, nodeID, batchID, agentreports.KindDestinations, now)
		if err != nil || seen {
			return err
		}
		applied = true
		if len(rows) == 0 {
			return nil
		}
		table := model.DestinationStat{}.TableName()
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "hour_at"}, {Name: "node_id"}, {Name: "user_id"}, {Name: "domain"}, {Name: "protocol"}},
			DoUpdates: clause.Assignments(map[string]any{
				"upload":      gorm.Expr(table + ".upload + excluded.upload"),
				"download":    gorm.Expr(table + ".download + excluded.download"),
				"connections": gorm.Expr(table + ".connections + excluded.connections"),
			}),
		}).CreateInBatches(rows, insertBatch).Error
	})
	return applied, err
}
