package account

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrAccountNotFound means no account has the user id.
var ErrAccountNotFound = errors.New("account not found")

// Changes are the identity fields an administrator edits; nil fields stay.
type Changes struct {
	Email        *string
	PasswordHash *string
	IsAdmin      *bool
	Banned       *bool
}

// UpdateResult is an account after Update and what changed.
type UpdateResult struct {
	Account Account
	// Changed is true when any field changed (the version rose).
	Changed bool
	// PasswordChanged is true when the password hash changed.
	PasswordChanged bool
}

// Update applies an administrator's changes. The version rises with any
// change; the token version rises when the email, password, admin or ban
// flag changes, which ends the account's sessions.
func (s *Store) Update(ctx context.Context, userID uint64, changes Changes) (UpdateResult, error) {
	if err := s.check(); err != nil {
		return UpdateResult{}, err
	}
	var result UpdateResult
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row accountRow
		err := tx.Table(s.Tables.Account).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		updates := map[string]any{}
		if changes.Email != nil && *changes.Email != row.Email {
			updates["email"] = *changes.Email
		}
		if changes.PasswordHash != nil && *changes.PasswordHash != row.PasswordHash {
			updates["password_hash"], updates["password_algo"], updates["password_salt"] = *changes.PasswordHash, "", ""
			result.PasswordChanged = true
		}
		if changes.IsAdmin != nil && flag(*changes.IsAdmin) != row.IsAdmin {
			updates["is_admin"] = flag(*changes.IsAdmin)
		}
		if changes.Banned != nil && flag(*changes.Banned) != row.Banned {
			updates["banned"] = flag(*changes.Banned)
		}
		if len(updates) > 0 {
			result.Changed = true
			updates["version"] = row.Version + 1
			updates["token_version"] = row.TokenVersion + 1
			updates["updated_at"] = s.now().Unix()
			if err := tx.Table(s.Tables.Account).Where("user_id = ?", userID).Updates(updates).Error; err != nil {
				if isUniqueViolation(err) {
					return ErrEmailTaken
				}
				return err
			}
		}
		var updated accountRow
		if err := tx.Table(s.Tables.Account).Where("user_id = ?", userID).Take(&updated).Error; err != nil {
			return err
		}
		accounts, err := s.withMFA(ctx, []accountRow{updated})
		if err != nil {
			return err
		}
		result.Account = accounts[0]
		return nil
	})
	return result, err
}

// Delete removes an account and its second factor.
func (s *Store) Delete(ctx context.Context, userID uint64) error {
	if err := s.check(); err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table(s.Tables.MFA).Where("user_id = ?", userID).Delete(&mfaRow{}).Error; err != nil {
			return err
		}
		return tx.Table(s.Tables.Account).Where("user_id = ?", userID).Delete(&accountRow{}).Error
	})
}

func isUniqueViolation(err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique constraint") || strings.Contains(message, "duplicate key")
}

// Touch raises an account's version without changing its fields, for
// changes the product mirrors elsewhere, such as the second factor.
func (s *Store) Touch(ctx context.Context, userID uint64) (Account, error) {
	if err := s.check(); err != nil {
		return Account{}, err
	}
	var touched Account
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row accountRow
		err := tx.Table(s.Tables.Account).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrAccountNotFound
		}
		if err != nil {
			return err
		}
		row.Version++
		row.UpdatedAt = s.now().Unix()
		if err := tx.Table(s.Tables.Account).Where("user_id = ?", userID).
			Updates(map[string]any{"version": row.Version, "updated_at": row.UpdatedAt}).Error; err != nil {
			return err
		}
		accounts, err := s.withMFA(ctx, []accountRow{row})
		if err != nil {
			return err
		}
		touched = accounts[0]
		return nil
	})
	return touched, err
}
