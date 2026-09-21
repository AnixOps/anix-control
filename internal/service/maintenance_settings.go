package service

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func GetMaintenanceSettings(db *gorm.DB) (model.MaintenanceSettings, error) {
	var s model.MaintenanceSettings
	err := db.First(&s, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.MaintenanceSettings{ID: 1, TechnicianIDs: []uint{}, Contacts: []model.MaintenanceContact{}}, nil
	}
	return s, err
}
func MaintenanceActorRole(db *gorm.DB, actorID uint) (string, error) {
	s, err := GetMaintenanceSettings(db)
	if err != nil {
		return "", err
	}
	if actorID == 0 {
		return "", nil
	}
	if s.OwnerID == actorID {
		return "owner", nil
	}
	if slices.Contains(s.TechnicianIDs, actorID) {
		return "technician", nil
	}
	return "", nil
}
func SaveMaintenanceSettings(db *gorm.DB, actorID uint, bootstrapAdmin bool, input model.MaintenanceSettings, now time.Time) (model.MaintenanceSettings, error) {
	result := model.MaintenanceSettings{}
	err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
		seed := model.MaintenanceSettings{ID: 1}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var current model.MaintenanceSettings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, 1).Error; err != nil {
			return err
		}
		if actorID == 0 || (current.OwnerID == 0 && !bootstrapAdmin) || (current.OwnerID != 0 && actorID != current.OwnerID) {
			return errors.New("owner_required")
		}
		if input.Revision != current.Revision {
			return errors.New("settings_changed")
		}
		if input.OwnerID == 0 || len(input.TechnicianIDs) > 100 || len(input.Contacts) > 101 {
			return errors.New("invalid_contacts")
		}
		required := map[uint]bool{input.OwnerID: true}
		for _, id := range input.TechnicianIDs {
			if id == 0 || required[id] {
				return errors.New("invalid_technicians")
			}
			required[id] = true
		}
		contacts := map[uint]model.MaintenanceContact{}
		for _, c := range input.Contacts {
			address, err := mail.ParseAddress(c.Email)
			if err != nil || address.Address != c.Email || len(c.Email) > 254 || strings.ContainsAny(c.Email, "\r\n") || !regexp.MustCompile(`^-?[0-9]{1,20}$`).MatchString(c.TelegramChatID) || !required[c.UserID] {
				return errors.New("invalid_contacts")
			}
			if _, exists := contacts[c.UserID]; exists {
				return errors.New("duplicate_contact")
			}
			contacts[c.UserID] = c
		}
		for id := range required {
			if _, ok := contacts[id]; !ok {
				return errors.New("missing_contact")
			}
			var count int64
			if err := tx.Model(&model.User{}).Where("id = ?", id).Count(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return errors.New("unknown_operator")
			}
		}
		ownerChanged := current.OwnerID != input.OwnerID
		old := maintenanceContact(current, current.OwnerID)
		newContact := contacts[input.OwnerID]
		if ownerChanged || old.Email != newContact.Email {
			current.OwnerEmailVerifiedAt = nil
			current.EmailChallengeHash = ""
			current.EmailChallengeExpiresAt = nil
		}
		if ownerChanged || old.TelegramChatID != newContact.TelegramChatID {
			current.OwnerTelegramVerifiedAt = nil
			current.TelegramChallengeHash = ""
			current.TelegramChallengeExpiresAt = nil
		}
		if input.Enabled && (current.OwnerEmailVerifiedAt == nil || current.OwnerTelegramVerifiedAt == nil) {
			return errors.New("owner_channels_unverified")
		}
		current.OwnerID, current.TechnicianIDs, current.Contacts, current.Enabled = input.OwnerID, input.TechnicianIDs, input.Contacts, input.Enabled
		current.Revision++
		current.UpdatedAt = now
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		// A queued delivery must never use a stale recipient after settings change.
		if err := tx.Model(&model.MaintenanceDelivery{}).Where("status = ?", "pending").Update("status", "cancelled").Error; err != nil {
			return err
		}
		if err := tx.Create(&model.MaintenanceRecord{ActorID: actorID, Kind: "settings", Note: fmt.Sprintf("运维设置修订 %d", current.Revision), CreatedAt: now}).Error; err != nil {
			return err
		}
		result = current
		return nil
	})
	return result, err
}
func maintenanceContact(s model.MaintenanceSettings, id uint) model.MaintenanceContact {
	for _, c := range s.Contacts {
		if c.UserID == id {
			return c
		}
	}
	return model.MaintenanceContact{UserID: id}
}
func QueueMaintenanceVerification(db *gorm.DB, actorID uint, channel string, now time.Time) error {
	if channel != "email" && channel != "telegram" {
		return errors.New("invalid_channel")
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	code := hex.EncodeToString(random)
	digest := sha256.Sum256([]byte(code))
	expires := now.Add(15 * time.Minute)
	return WithRetryableTransaction(db, func(tx *gorm.DB) error {
		var s model.MaintenanceSettings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, 1).Error; err != nil {
			return err
		}
		if actorID == 0 || actorID != s.OwnerID {
			return errors.New("owner_required")
		}
		contact := maintenanceContact(s, actorID)
		destination := contact.Email
		if channel == "email" {
			s.EmailChallengeHash = hex.EncodeToString(digest[:])
			s.EmailChallengeExpiresAt = &expires
		} else {
			s.TelegramChallengeHash = hex.EncodeToString(digest[:])
			s.TelegramChallengeExpiresAt = &expires
			destination = contact.TelegramChatID
		}
		if destination == "" {
			return errors.New("missing_contact")
		}
		if err := tx.Save(&s).Error; err != nil {
			return err
		}
		return tx.Create(&model.MaintenanceDelivery{DedupeKey: "verify:" + code, SettingsRevision: s.Revision, Channel: channel, RecipientID: actorID, Destination: destination, Reason: "verification", Body: "AnixOps 运维渠道验证码（15 分钟有效）：" + code, Status: "pending", NextAttemptAt: now, CreatedAt: now}).Error
	})
}
func ConfirmMaintenanceVerification(db *gorm.DB, actorID uint, channel, code string, now time.Time) error {
	if channel != "email" && channel != "telegram" {
		return errors.New("invalid_channel")
	}
	if len(code) != 32 {
		return errors.New("invalid_verification")
	}
	sum := sha256.Sum256([]byte(code))
	digest := hex.EncodeToString(sum[:])
	return WithRetryableTransaction(db, func(tx *gorm.DB) error {
		var s model.MaintenanceSettings
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&s, 1).Error; err != nil {
			return err
		}
		if actorID != s.OwnerID || actorID == 0 {
			return errors.New("owner_required")
		}
		expected, expires := s.EmailChallengeHash, s.EmailChallengeExpiresAt
		if channel == "telegram" {
			expected, expires = s.TelegramChallengeHash, s.TelegramChallengeExpiresAt
		}
		if expires == nil || !now.Before(*expires) || subtle.ConstantTimeCompare([]byte(expected), []byte(digest)) != 1 {
			return errors.New("invalid_verification")
		}
		// Require evidence that this challenge's delivery actually succeeded.
		var count int64
		if err := tx.Model(&model.MaintenanceDelivery{}).Where("dedupe_key = ? AND status = ? AND settings_revision = ?", "verify:"+code, "sent", s.Revision).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return errors.New("verification_not_delivered")
		}
		if channel == "email" {
			s.OwnerEmailVerifiedAt = &now
			s.EmailChallengeHash = ""
			s.EmailChallengeExpiresAt = nil
		} else {
			s.OwnerTelegramVerifiedAt = &now
			s.TelegramChallengeHash = ""
			s.TelegramChallengeExpiresAt = nil
		}
		return tx.Save(&s).Error
	})
}

func queueMaintenanceNotice(tx *gorm.DB, t model.MaintenanceTicket, reason string, owner bool, now time.Time) error {
	s, err := GetMaintenanceSettings(tx)
	if err != nil {
		return err
	}
	if s.OwnerID == 0 || !s.Enabled {
		return nil
	}
	recipients := append([]uint{}, s.TechnicianIDs...)
	if owner || len(recipients) == 0 {
		recipients = append(recipients, s.OwnerID)
	}
	for _, id := range recipients {
		contact := maintenanceContact(s, id)
		for _, channel := range []string{"email", "telegram"} {
			destination := contact.Email
			if channel == "telegram" {
				destination = contact.TelegramChatID
			}
			key := fmt.Sprintf("ticket:%d:episode:%d:settings:%d:%s:%d:%s", t.ID, t.Episode, s.Revision, reason, id, channel)
			body := fmt.Sprintf("AnixOps 运维工单 #%d\n%s\n状态：%s；级别：%s\n首次失败：%s\n原因：%s", t.ID, t.Title, t.Status, t.Severity, t.FirstFailedAt.UTC().Format(time.RFC3339), reason)
			delivery := model.MaintenanceDelivery{DedupeKey: key, TicketID: t.ID, SettingsRevision: s.Revision, Channel: channel, RecipientID: id, Destination: destination, Reason: reason, Body: body, Status: "pending", NextAttemptAt: now, CreatedAt: now}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&delivery).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
