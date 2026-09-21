package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/maintenance"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ReceiveMaintenanceEvents returns success only after a durable insert/duplicate
// check. Each item has its own transaction so invalid siblings cannot block it.
func ReceiveMaintenanceEvents(db *gorm.DB, nodeID uint, raw []byte, now time.Time) maintenance.Acknowledgement {
	ack := maintenance.Acknowledgement{Version: maintenance.WireVersion, Events: []maintenance.Ack{}}
	var batch maintenance.Batch
	if maintenance.Decode(raw, &batch, maintenance.MaxPayloadBytes) != nil || batch.Version != maintenance.WireVersion || len(batch.Events) == 0 || len(batch.Events) > maintenance.MaxBatch {
		ack.Events = append(ack.Events, maintenance.Ack{Error: "invalid_batch"})
		return ack
	}
	for _, data := range batch.Events {
		var event maintenance.Event
		err := maintenance.Decode(data, &event, maintenance.MaxEventBytes)
		// Never echo unvalidated/free-form IDs to a peer or logs.
		item := maintenance.Ack{}
		if err == nil {
			err = event.ValidateAt(now)
		}
		if err != nil {
			item.Error = "invalid_event"
			ack.Events = append(ack.Events, item)
			continue
		}
		item.EventID = event.EventID
		if nodeID == 0 || event.NodeID != strconv.FormatUint(uint64(nodeID), 10) || (event.Source != "agent" && event.Source != "networkcore") {
			item.Error = "identity_mismatch"
			ack.Events = append(ack.Events, item)
			continue
		}
		_, _, err = recordMaintenanceEvent(db, event, now)
		if err != nil {
			item.Error = "persistence_failed"
			if errors.Is(err, errMaintenanceIDConflict) {
				item.Error = "event_id_conflict"
			}
		} else {
			item.Persisted = true
		}
		ack.Events = append(ack.Events, item)
	}
	return ack
}

var errMaintenanceIDConflict = errors.New("event_id_conflict")

func RecordMaintenanceEvent(event maintenance.Event) (*model.MaintenanceEvent, bool, error) {
	return recordMaintenanceEvent(database.Get(), event, time.Now())
}
func recordMaintenanceEvent(db *gorm.DB, event maintenance.Event, now time.Time) (*model.MaintenanceEvent, bool, error) {
	if err := event.ValidateAt(now); err != nil {
		return nil, false, err
	}
	original, err := json.Marshal(event)
	if err != nil {
		return nil, false, err
	}
	sum := sha256.Sum256(original)
	safe, _ := json.Marshal(event.Safe())
	node, _ := strconv.ParseUint(event.NodeID, 10, 64)
	record := model.MaintenanceEvent{EventID: event.EventID, NodeID: uint(node), StreamKey: event.DedupeKey(), Digest: hex.EncodeToString(sum[:]), Body: string(safe), OccurredAt: event.OccurredAt, CreatedAt: now, RetryAt: now}
	created := false
	err = WithRetryableTransaction(db, func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record)
		if result.Error != nil {
			return result.Error
		}
		created = result.RowsAffected == 1
		if created {
			return nil
		}
		var existing model.MaintenanceEvent
		if err := tx.First(&existing, "event_id = ?", event.EventID).Error; err != nil {
			return err
		}
		if existing.NodeID != record.NodeID || existing.Digest != record.Digest {
			return errMaintenanceIDConflict
		}
		record = existing
		return nil
	})
	return &record, created, err
}
func ProcessPendingMaintenanceEvents(limit int) (int, error) {
	return ProcessMaintenanceEvents(database.Get(), limit, time.Now())
}
func ProcessMaintenanceEvents(db *gorm.DB, limit int, now time.Time) (int, error) {
	if db == nil {
		return 0, errors.New("database unavailable")
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var events []model.MaintenanceEvent
	if err := db.Where("processed = ? AND retry_at <= ?", false, now).Order("occurred_at ASC, event_id ASC").Limit(limit).Find(&events).Error; err != nil {
		return 0, err
	}
	processed := 0
	var allErrors []error
	for _, record := range events {
		didProcess := false
		err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
			didProcess = false
			// Conditional update acquires the row before aggregation; rollback releases
			// the claim. Concurrent processes observe zero rows after commit.
			result := tx.Model(&model.MaintenanceEvent{}).Where("event_id = ? AND processed = ?", record.EventID, false).Update("processed", true)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			var event maintenance.Event
			if err := maintenance.Decode([]byte(record.Body), &event, maintenance.MaxEventBytes); err != nil {
				return err
			}
			if err := event.ValidateAt(now); err != nil {
				return err
			}
			slot := model.MaintenanceStream{Key: record.StreamKey}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&slot).Error; err != nil {
				return err
			}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&slot, "key = ?", record.StreamKey).Error; err != nil {
				return err
			}
			var ticket model.MaintenanceTicket
			if slot.TicketID != nil {
				if err := tx.First(&ticket, *slot.TicketID).Error; err != nil {
					return err
				}
			}
			// Old/replayed observations remain evidence but cannot regress current state.
			if event.OccurredAt.Before(slot.LastEventAt) || event.OccurredAt.Equal(slot.LastEventAt) {
				didProcess = true
				return tx.Model(&model.MaintenanceEvent{}).Where("event_id = ?", record.EventID).Update("ticket_id", slot.TicketID).Error
			}
			slot.LastEventAt = event.OccurredAt
			if event.Fault() {
				if ticket.ID == 0 || ticket.Status == "closed" {
					previous := slot.TicketID
					ticket = model.MaintenanceTicket{NodeID: record.NodeID, StreamKey: record.StreamKey, Environment: event.Environment, PluginID: event.PluginID, InstanceID: event.InstanceID, Status: "open", FirstFailedAt: *event.FirstFailedAt, PreviousTicketID: previous, Episode: 1, Occurrences: 0, CreatedAt: now}
					if err := tx.Create(&ticket).Error; err != nil {
						return err
					}
					slot.TicketID = &ticket.ID
				} else if ticket.Status == "recovered" {
					ticket.Status = "open"
					ticket.RecoveredAt = nil
					ticket.EscalatedAt = nil
					ticket.Episode++
					if err := tx.Create(&model.MaintenanceRecord{TicketID: ticket.ID, Kind: "recurrence", Note: "健康恢复后故障复发", CreatedAt: now}).Error; err != nil {
						return err
					}
				}
				ticket.Occurrences++
				ticket.LastEventAt = event.OccurredAt
				ticket.PluginVersion, ticket.AgentVersion, ticket.ConfigVersion = event.PluginVersion, event.AgentVersion, event.ConfigVersion
				ticket.ErrorCode = event.ErrorCode
				ticket.Severity = "P1"
				if event.Major() {
					ticket.Severity = "P0"
				}
				ticket.Title = fmt.Sprintf("节点 %d · %s · %s", record.NodeID, event.PluginID, event.ErrorCode)
				if event.FirstFailedAt.Before(ticket.FirstFailedAt) {
					ticket.FirstFailedAt = *event.FirstFailedAt
				}
				if err := tx.Save(&ticket).Error; err != nil {
					return err
				}
				reason := "opened"
				if event.Major() {
					reason = "major"
				}
				if err := queueMaintenanceNotice(tx, ticket, reason, event.Major(), now); err != nil {
					return err
				}
			} else if event.Recovered() && ticket.ID != 0 && ticket.Status == "open" {
				ticket.Status = "recovered"
				ticket.RecoveredAt = &event.OccurredAt
				ticket.LastEventAt = event.OccurredAt
				if err := tx.Save(&ticket).Error; err != nil {
					return err
				}
				if err := tx.Create(&model.MaintenanceRecord{TicketID: ticket.ID, Kind: "recovered", Note: "已持续健康至少 5 分钟，等待人工处理记录和关闭", CreatedAt: now}).Error; err != nil {
					return err
				}
				// Suppress pending failure notifications immediately upon recovery.
				if err := tx.Model(&model.MaintenanceDelivery{}).Where("ticket_id = ? AND status = ?", ticket.ID, "pending").Updates(map[string]any{"status": "cancelled"}).Error; err != nil {
					return err
				}
				if err := queueMaintenanceNotice(tx, ticket, "recovered", true, now); err != nil {
					return err
				}
			}
			if err := tx.Save(&slot).Error; err != nil {
				return err
			}
			didProcess = true
			return tx.Model(&model.MaintenanceEvent{}).Where("event_id = ?", record.EventID).Update("ticket_id", slot.TicketID).Error
		})
		if err != nil {
			// Do not log client payload or arbitrary database strings. One poison record
			// backs off while later records keep progressing.
			next := now.Add(time.Minute * time.Duration(min(record.Attempts+1, 60)))
			updateErr := db.Model(&model.MaintenanceEvent{}).Where("event_id = ? AND processed = ?", record.EventID, false).Updates(map[string]any{"attempts": gorm.Expr("attempts + 1"), "retry_at": next, "last_error": "processing_failed"}).Error
			allErrors = append(allErrors, errors.New("maintenance event processing failed"), updateErr)
		} else if didProcess {
			processed++
		}
	}
	return processed, errors.Join(allErrors...)
}

// TickMaintenanceTickets serializes transition+notification generation with event
// aggregation on each stream. Claiming never changes the first failure clock.
func TickMaintenanceTickets(db *gorm.DB, now time.Time) error {
	var tickets []model.MaintenanceTicket
	if err := db.Where("status = ?", "open").Find(&tickets).Error; err != nil {
		return err
	}
	var errs []error
	for _, candidate := range tickets {
		err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
			var slot model.MaintenanceStream
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&slot, "key = ?", candidate.StreamKey).Error; err != nil {
				return err
			}
			var t model.MaintenanceTicket
			if err := tx.First(&t, candidate.ID).Error; err != nil {
				return err
			}
			if t.Status != "open" {
				return nil
			}
			clusterMajor, err := maintenanceMultiNodeFault(tx, t, now)
			if err != nil {
				return err
			}
			wasMajor := t.Severity == "P0"
			major := clusterMajor || wasMajor
			escalate := major || (t.ClaimedBy == nil && now.Sub(t.FirstFailedAt) >= 30*time.Minute) || now.Sub(t.FirstFailedAt) >= 2*time.Hour
			if clusterMajor && !wasMajor {
				t.Severity = "P0"
				if t.EscalatedAt == nil {
					t.EscalatedAt = &now
				}
				if err := tx.Save(&t).Error; err != nil {
					return err
				}
				if err := queueMaintenanceNotice(tx, t, "major", true, now); err != nil {
					return err
				}
			} else if escalate && t.EscalatedAt == nil {
				t.EscalatedAt = &now
				if err := tx.Save(&t).Error; err != nil {
					return err
				}
				if err := queueMaintenanceNotice(tx, t, "escalated", true, now); err != nil {
					return err
				}
			}
			if t.ClaimedBy == nil && escalate {
				bucket := int64(now.Sub(*t.EscalatedAt) / (30 * time.Minute))
				if bucket >= 1 {
					return queueMaintenanceNotice(tx, t, fmt.Sprintf("reminder:%d", bucket), true, now)
				}
			} else if t.ClaimedAt != nil {
				bucket := int64(now.Sub(*t.ClaimedAt) / (2 * time.Hour))
				if bucket >= 1 {
					return queueMaintenanceNotice(tx, t, fmt.Sprintf("summary:%d", bucket), escalate, now)
				}
			}
			// Backfill recipients when operations are enabled after faults began.
			return queueMaintenanceNotice(tx, t, "opened", escalate, now)
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func maintenanceMultiNodeFault(tx *gorm.DB, t model.MaintenanceTicket, now time.Time) (bool, error) {
	var total int64
	eligible := tx.Model(&model.Node{}).Where("status <> ?", model.NodeStatusDisabled).Where("id NOT IN (?)", tx.Model(&model.MaintenanceNodeState{}).Select("node_id").Where("maintenance = ? OR disabled = ?", true, true))
	if err := eligible.Count(&total).Error; err != nil {
		return false, err
	}
	if total == 0 {
		return false, nil
	}
	var incidents []model.MaintenanceTicket
	if err := tx.Where("status = ? AND environment = ? AND node_id IN (?)", "open", t.Environment, eligible.Select("id")).Order("first_failed_at ASC").Find(&incidents).Error; err != nil {
		return false, err
	}
	// A five-minute cluster is based on first failures, not the worker's current
	// wall clock; delayed outbox delivery must still trigger escalation.
	for left := 0; left < len(incidents); left++ {
		nodes := map[uint]bool{}
		includes := false
		for right := left; right < len(incidents) && incidents[right].FirstFailedAt.Sub(incidents[left].FirstFailedAt) <= 5*time.Minute; right++ {
			nodes[incidents[right].NodeID] = true
			if incidents[right].ID == t.ID {
				includes = true
			}
		}
		if includes && (len(nodes) >= 5 || (len(nodes) >= 2 && int64(len(nodes))*5 >= total)) {
			return true, nil
		}
	}
	return false, nil
}

func CleanupMaintenance(db *gorm.DB, now time.Time) error {
	return WithRetryableTransaction(db, func(tx *gorm.DB) error {
		protected := tx.Model(&model.MaintenanceTicket{}).Select("id").Where("status <> ?", "closed")
		if err := tx.Model(&model.MaintenanceEvent{}).Where("processed = ? AND created_at < ? AND (ticket_id IS NULL OR ticket_id NOT IN (?))", true, now.AddDate(0, 0, -30), protected).Update("body", "").Error; err != nil {
			return err
		}
		if err := tx.Where("processed = ? AND created_at < ? AND (ticket_id IS NULL OR ticket_id NOT IN (?))", true, now.AddDate(0, 0, -90), protected).Delete(&model.MaintenanceEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("status IN ? AND created_at < ? AND ticket_id NOT IN (?)", []string{"sent", "cancelled"}, now.AddDate(0, 0, -90), protected).Delete(&model.MaintenanceDelivery{}).Error; err != nil {
			return err
		}
		if err := tx.Where("created_at < ? AND ticket_id NOT IN (?)", now.AddDate(-1, 0, 0), protected).Delete(&model.MaintenanceRecord{}).Error; err != nil {
			return err
		}
		// Keep the compact stream cursor/tombstone to prevent delayed replay from
		// reopening retired incidents. Summaries themselves follow the one-year rule.
		retired := tx.Model(&model.MaintenanceTicket{}).Select("id").Where("status = ? AND closed_at < ?", "closed", now.AddDate(-1, 0, 0))
		if err := tx.Model(&model.MaintenanceStream{}).Where("ticket_id IN (?)", retired).Update("ticket_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Where("status = ? AND closed_at < ?", "closed", now.AddDate(-1, 0, 0)).Delete(&model.MaintenanceTicket{}).Error; err != nil {
			return err
		}
		return CleanupMaintenanceChangesTx(tx, now)
	})
}
