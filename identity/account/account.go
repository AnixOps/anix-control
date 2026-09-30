// Package account stores identity accounts and their MFA in the identity
// service's own tables. Account ids are the product's user ids,
// so accounts and the product's subscriber rows share one key.
//
// The tables (created by the product's migrations) are, with times in Unix
// seconds and flags as 0/1:
//
//	account(user_id BIGINT PK, account_uuid VARCHAR(36) UNIQUE, email VARCHAR(255) UNIQUE,
//	  password_hash TEXT, password_algo VARCHAR(20), password_salt VARCHAR(64),
//	  is_admin, is_staff, banned SMALLINT, token_version, version, invite_user_id BIGINT,
//	  created_at, updated_at BIGINT)
//	mfa(user_id BIGINT PK, enabled SMALLINT, sealed_totp_secret TEXT,
//	  backup_code_hashes TEXT, enabled_at, updated_at BIGINT)
//	import_run(import_id VARCHAR(64) PK, checkpoint TEXT, accounts, updated_at BIGINT)
package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Account is an identity account.
type Account struct {
	UserID       uint64
	AccountUUID  string
	Email        string
	PasswordHash string
	PasswordAlgo string
	PasswordSalt string
	IsAdmin      bool
	IsStaff      bool
	Banned       bool
	// TokenVersion starts at 1; raising it ends every token of the account.
	TokenVersion uint64
	// Version rises on every change.
	Version      uint64
	InviteUserID uint64
	CreatedAt    time.Time
	UpdatedAt    time.Time
	MFAEnabled   bool
}

// MFA is an account's second factor. BackupCodeHashes are SHA-256 hex
// digests of the codes; the store keeps only keyed hashes of those.
type MFA struct {
	Enabled          bool
	TOTPSecret       string
	BackupCodeHashes []string
	EnabledAt        time.Time
	// LastUsed and LastMethod describe the last successful check.
	LastUsed   time.Time
	LastMethod string
}

// Imported is one account of an import batch.
type Imported struct {
	Account Account
	// MFA is nil when the account has none.
	MFA *MFA
}

// Tables names the store's tables.
type Tables struct {
	Account   string
	MFA       string
	ImportRun string
	// MFAAttempt records MFA checks at login; optional for imports.
	MFAAttempt string
}

// Store reads and writes accounts.
type Store struct {
	DB     *gorm.DB
	Tables Tables
	// Secrets seals TOTP secrets and hashes backup codes.
	Secrets *secretbox.Box
	// Now defaults to time.Now.
	Now func() time.Time
}

type accountRow struct {
	UserID       uint64 `gorm:"column:user_id;primaryKey"`
	AccountUUID  string `gorm:"column:account_uuid"`
	Email        string `gorm:"column:email"`
	PasswordHash string `gorm:"column:password_hash"`
	PasswordAlgo string `gorm:"column:password_algo"`
	PasswordSalt string `gorm:"column:password_salt"`
	IsAdmin      int    `gorm:"column:is_admin"`
	IsStaff      int    `gorm:"column:is_staff"`
	Banned       int    `gorm:"column:banned"`
	TokenVersion uint64 `gorm:"column:token_version"`
	Version      uint64 `gorm:"column:version"`
	InviteUserID uint64 `gorm:"column:invite_user_id"`
	CreatedAt    int64  `gorm:"column:created_at"`
	UpdatedAt    int64  `gorm:"column:updated_at"`
}

type mfaRow struct {
	UserID           uint64 `gorm:"column:user_id;primaryKey"`
	Enabled          int    `gorm:"column:enabled"`
	SealedTOTPSecret string `gorm:"column:sealed_totp_secret"`
	BackupCodeHashes string `gorm:"column:backup_code_hashes"`
	EnabledAt        int64  `gorm:"column:enabled_at"`
	LastUsed         int64  `gorm:"column:last_used"`
	LastMethod       string `gorm:"column:last_method"`
	UpdatedAt        int64  `gorm:"column:updated_at"`
}

type importRow struct {
	ImportID   string `gorm:"column:import_id;primaryKey"`
	Checkpoint string `gorm:"column:checkpoint"`
	Accounts   uint64 `gorm:"column:accounts"`
	UpdatedAt  int64  `gorm:"column:updated_at"`
}

// ErrNotConfigured means the store has no storage or no KEK.
var ErrNotConfigured = errors.New("the identity account store is not configured")

func (s *Store) check() error {
	if s == nil || s.DB == nil || s.Secrets == nil || s.Tables.Account == "" || s.Tables.MFA == "" || s.Tables.ImportRun == "" {
		return ErrNotConfigured
	}
	return nil
}

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// ImportResult is what one import batch stored.
type ImportResult struct {
	Accounts uint64
}

// Import stores one batch of accounts from the product's legacy tables in a single
// transaction and records the batch's checkpoint under importID. The source
// is authoritative for imported fields: an account that changed gets a new
// version, one that did not keeps it, so a repeated batch changes nothing.
func (s *Store) Import(ctx context.Context, importID, checkpoint string, accounts []Imported) (ImportResult, error) {
	if err := s.check(); err != nil {
		return ImportResult{}, err
	}
	if importID == "" || len(importID) > 64 {
		return ImportResult{}, errors.New("an import needs an id of at most 64 bytes")
	}
	now := s.now().Unix()
	var result ImportResult
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, imported := range accounts {
			if err := s.importAccount(tx, imported, now); err != nil {
				return err
			}
			result.Accounts++
		}
		run := importRow{ImportID: importID, Checkpoint: checkpoint, UpdatedAt: now}
		var existing importRow
		switch err := tx.Table(s.Tables.ImportRun).Where("import_id = ?", importID).Take(&existing).Error; {
		case err == nil:
			run.Accounts = existing.Accounts + result.Accounts
		case errors.Is(err, gorm.ErrRecordNotFound):
			run.Accounts = result.Accounts
		default:
			return err
		}
		return tx.Table(s.Tables.ImportRun).Clauses(clause.OnConflict{UpdateAll: true}).Create(&run).Error
	})
	return result, err
}

func (s *Store) importAccount(tx *gorm.DB, imported Imported, now int64) error {
	a := imported.Account
	if a.UserID == 0 || len(a.AccountUUID) != 36 || a.Email == "" {
		return fmt.Errorf("imported account %d needs a user id, an account UUID and an email", a.UserID)
	}
	row := accountRow{
		UserID: a.UserID, AccountUUID: a.AccountUUID, Email: a.Email,
		PasswordHash: a.PasswordHash, PasswordAlgo: a.PasswordAlgo, PasswordSalt: a.PasswordSalt,
		IsAdmin: flag(a.IsAdmin), IsStaff: flag(a.IsStaff), Banned: flag(a.Banned), InviteUserID: a.InviteUserID,
		TokenVersion: 1, Version: 1, CreatedAt: unix(a.CreatedAt), UpdatedAt: now,
	}
	if row.CreatedAt == 0 {
		row.CreatedAt = now
	}
	var existing accountRow
	switch err := tx.Table(s.Tables.Account).Where("user_id = ?", a.UserID).Take(&existing).Error; {
	case err == nil:
		row.TokenVersion = max(existing.TokenVersion, 1)
		row.Version = existing.Version
		if importedFieldsDiffer(existing, row) {
			row.Version++
		} else {
			row.UpdatedAt = existing.UpdatedAt
		}
		if err := tx.Table(s.Tables.Account).Where("user_id = ?", a.UserID).Updates(map[string]any{
			"account_uuid": row.AccountUUID, "email": row.Email, "password_hash": row.PasswordHash,
			"password_algo": row.PasswordAlgo, "password_salt": row.PasswordSalt, "is_admin": row.IsAdmin,
			"is_staff": row.IsStaff, "banned": row.Banned, "invite_user_id": row.InviteUserID,
			"token_version": row.TokenVersion, "version": row.Version, "created_at": row.CreatedAt, "updated_at": row.UpdatedAt,
		}).Error; err != nil {
			return fmt.Errorf("import account %d: %w", a.UserID, err)
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		if err := tx.Table(s.Tables.Account).Create(&row).Error; err != nil {
			return fmt.Errorf("import account %d: %w", a.UserID, err)
		}
	default:
		return err
	}
	return s.importMFA(tx, a.UserID, imported.MFA, now)
}

func importedFieldsDiffer(a, b accountRow) bool {
	return a.AccountUUID != b.AccountUUID || a.Email != b.Email || a.PasswordHash != b.PasswordHash ||
		a.PasswordAlgo != b.PasswordAlgo || a.PasswordSalt != b.PasswordSalt || a.IsAdmin != b.IsAdmin ||
		a.IsStaff != b.IsStaff || a.Banned != b.Banned || a.InviteUserID != b.InviteUserID
}

func (s *Store) importMFA(tx *gorm.DB, userID uint64, mfa *MFA, now int64) error {
	if mfa == nil || (!mfa.Enabled && mfa.TOTPSecret == "" && len(mfa.BackupCodeHashes) == 0) {
		return tx.Table(s.Tables.MFA).Where("user_id = ?", userID).Delete(&mfaRow{}).Error
	}
	owner := mfaOwner(userID)
	sealed := ""
	if mfa.TOTPSecret != "" {
		var err error
		if sealed, err = s.Secrets.Seal(owner, []byte(mfa.TOTPSecret)); err != nil {
			return err
		}
	}
	hashes := make([]string, 0, len(mfa.BackupCodeHashes))
	for _, digest := range mfa.BackupCodeHashes {
		hashes = append(hashes, s.Secrets.Hash(owner, digest))
	}
	encoded, err := json.Marshal(hashes)
	if err != nil {
		return err
	}
	row := mfaRow{
		UserID: userID, Enabled: flag(mfa.Enabled), SealedTOTPSecret: sealed, BackupCodeHashes: string(encoded),
		EnabledAt: unix(mfa.EnabledAt), LastUsed: unix(mfa.LastUsed), LastMethod: mfa.LastMethod, UpdatedAt: now,
	}
	return tx.Table(s.Tables.MFA).Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func mfaOwner(userID uint64) string { return "mfa:" + strconv.FormatUint(userID, 10) }

// Get returns the accounts with the given ids that exist, in id order.
func (s *Store) Get(ctx context.Context, userIDs []uint64) ([]Account, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return nil, nil
	}
	var rows []accountRow
	if err := s.DB.WithContext(ctx).Table(s.Tables.Account).Where("user_id IN ?", userIDs).Order("user_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.withMFA(ctx, rows)
}

// ChangedAfter returns up to limit accounts whose version is above version,
// in version order, for the product's projection reconciliation.
func (s *Store) ChangedAfter(ctx context.Context, version uint64, limit int) ([]Account, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	var rows []accountRow
	if err := s.DB.WithContext(ctx).Table(s.Tables.Account).Where("version > ?", version).
		Order("version, user_id").Limit(limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.withMFA(ctx, rows)
}

// MFA returns an account's second factor with the TOTP secret opened, or
// nil when it has none.
func (s *Store) MFA(ctx context.Context, userID uint64) (*MFA, []string, error) {
	if err := s.check(); err != nil {
		return nil, nil, err
	}
	var row mfaRow
	err := s.DB.WithContext(ctx).Table(s.Tables.MFA).Where("user_id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	mfa := &MFA{Enabled: row.Enabled == 1, EnabledAt: fromUnix(row.EnabledAt), LastUsed: fromUnix(row.LastUsed), LastMethod: row.LastMethod}
	if row.SealedTOTPSecret != "" {
		secret, err := s.Secrets.Open(mfaOwner(userID), row.SealedTOTPSecret)
		if err != nil {
			return nil, nil, err
		}
		mfa.TOTPSecret = string(secret)
	}
	var keyed []string
	if err := json.Unmarshal([]byte(row.BackupCodeHashes), &keyed); err != nil {
		return nil, nil, fmt.Errorf("account %d backup codes are malformed", userID)
	}
	return mfa, keyed, nil
}

// BackupCodeMatches reports whether a backup code's SHA-256 digest is one of
// the account's stored keyed hashes.
func (s *Store) BackupCodeMatches(userID uint64, digest string, keyed []string) bool {
	candidate := s.Secrets.Hash(mfaOwner(userID), digest)
	for _, stored := range keyed {
		if secretbox.Equal(candidate, stored) {
			return true
		}
	}
	return false
}

func (s *Store) withMFA(ctx context.Context, rows []accountRow) ([]Account, error) {
	enabled := map[uint64]bool{}
	if len(rows) > 0 {
		ids := make([]uint64, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.UserID)
		}
		var mfaRows []mfaRow
		if err := s.DB.WithContext(ctx).Table(s.Tables.MFA).Where("user_id IN ? AND enabled = 1", ids).Find(&mfaRows).Error; err != nil {
			return nil, err
		}
		for _, row := range mfaRows {
			enabled[row.UserID] = true
		}
	}
	accounts := make([]Account, 0, len(rows))
	for _, row := range rows {
		accounts = append(accounts, Account{
			UserID: row.UserID, AccountUUID: row.AccountUUID, Email: row.Email,
			PasswordHash: row.PasswordHash, PasswordAlgo: row.PasswordAlgo, PasswordSalt: row.PasswordSalt,
			IsAdmin: row.IsAdmin == 1, IsStaff: row.IsStaff == 1, Banned: row.Banned == 1,
			TokenVersion: row.TokenVersion, Version: row.Version, InviteUserID: row.InviteUserID,
			CreatedAt: fromUnix(row.CreatedAt), UpdatedAt: fromUnix(row.UpdatedAt), MFAEnabled: enabled[row.UserID],
		})
	}
	return accounts, nil
}

func flag(value bool) int {
	if value {
		return 1
	}
	return 0
}

func unix(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func fromUnix(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(value, 0)
}

// ErrEmailTaken means another account has the email.
var ErrEmailTaken = errors.New("email belongs to another account")

// FindByEmail returns the account whose stored email equals email exactly.
func (s *Store) FindByEmail(ctx context.Context, email string) (Account, bool, error) {
	if err := s.check(); err != nil {
		return Account{}, false, err
	}
	var rows []accountRow
	if err := s.DB.WithContext(ctx).Table(s.Tables.Account).Where("email = ?", email).Order("user_id").Limit(1).Find(&rows).Error; err != nil {
		return Account{}, false, err
	}
	if len(rows) == 0 {
		return Account{}, false, nil
	}
	accounts, err := s.withMFA(ctx, rows)
	if err != nil {
		return Account{}, false, err
	}
	return accounts[0], true, nil
}

// EmailTaken reports whether an account has the email.
func (s *Store) EmailTaken(ctx context.Context, email string) (bool, error) {
	if err := s.check(); err != nil {
		return false, err
	}
	var count int64
	err := s.DB.WithContext(ctx).Table(s.Tables.Account).Where("email = ?", email).Count(&count).Error
	return count > 0, err
}

// Create stores a new account (version 1, token version 1).
func (s *Store) Create(ctx context.Context, a Account) error {
	if err := s.check(); err != nil {
		return err
	}
	if a.UserID == 0 || len(a.AccountUUID) != 36 || a.Email == "" || a.PasswordHash == "" {
		return errors.New("a new account needs a user id, an account UUID, an email and a password hash")
	}
	now := s.now().Unix()
	row := accountRow{
		UserID: a.UserID, AccountUUID: a.AccountUUID, Email: a.Email, PasswordHash: a.PasswordHash,
		PasswordAlgo: a.PasswordAlgo, PasswordSalt: a.PasswordSalt, IsAdmin: flag(a.IsAdmin), IsStaff: flag(a.IsStaff),
		Banned: flag(a.Banned), TokenVersion: 1, Version: 1, InviteUserID: a.InviteUserID, CreatedAt: now, UpdatedAt: now,
	}
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var taken int64
		if err := tx.Table(s.Tables.Account).Where("email = ?", a.Email).Count(&taken).Error; err != nil {
			return err
		}
		if taken > 0 {
			return ErrEmailTaken
		}
		return tx.Table(s.Tables.Account).Create(&row).Error
	})
}

// MFA methods.
const (
	MethodTOTP   = "totp"
	MethodBackup = "backup"
	MethodEmail  = "email"
	MethodSMS    = "sms"
)

// VerifyMFA checks a login or step-up code against the account's second
// factor. An account without enabled MFA passes. The method selects TOTP or
// a backup code; email and SMS are not implemented and always fail; any
// other value tries TOTP, then a backup code. A used backup code is
// consumed; a success records when and how.
func (s *Store) VerifyMFA(ctx context.Context, userID uint64, code, method string) (bool, error) {
	if err := s.check(); err != nil {
		return false, err
	}
	var valid bool
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row mfaRow
		err := tx.Table(s.Tables.MFA).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			valid = true
			return nil
		}
		if err != nil {
			return err
		}
		if row.Enabled != 1 {
			valid = true
			return nil
		}
		owner := mfaOwner(userID)
		secret := ""
		if row.SealedTOTPSecret != "" {
			opened, err := s.Secrets.Open(owner, row.SealedTOTPSecret)
			if err != nil {
				return err
			}
			secret = string(opened)
		}
		var keyed []string
		if err := json.Unmarshal([]byte(row.BackupCodeHashes), &keyed); err != nil {
			return fmt.Errorf("parse MFA backup codes: %w", err)
		}
		consumeBackup := func() bool {
			sum := sha256.Sum256([]byte(code))
			candidate := s.Secrets.Hash(owner, hex.EncodeToString(sum[:]))
			for index, stored := range keyed {
				if secretbox.Equal(candidate, stored) {
					keyed = append(keyed[:index], keyed[index+1:]...)
					return true
				}
			}
			return false
		}
		method = strings.ToLower(strings.TrimSpace(method))
		used := method
		consumed := false
		switch method {
		case MethodTOTP:
			valid = validTOTP(secret, code, s.now())
		case MethodBackup:
			valid = consumeBackup()
			consumed = valid
		case MethodEmail, MethodSMS:
			valid = false
		default:
			used = MethodTOTP
			valid = validTOTP(secret, code, s.now())
			if !valid {
				valid = consumeBackup()
				consumed = valid
				if valid {
					used = MethodBackup
				}
			}
		}
		if !valid {
			return nil
		}
		updates := map[string]any{"last_used": s.now().Unix(), "last_method": used}
		if consumed {
			encoded, err := json.Marshal(keyed)
			if err != nil {
				return err
			}
			updates["backup_code_hashes"] = string(encoded)
		}
		return tx.Table(s.Tables.MFA).Where("user_id = ?", userID).Updates(updates).Error
	})
	return valid, err
}

func validTOTP(secret, code string, now time.Time) bool {
	if secret == "" {
		return false
	}
	valid, err := totp.ValidateCustom(code, secret, now, totp.ValidateOpts{
		Period: 30, Skew: 1, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
	})
	return err == nil && valid
}

// MFAMethods returns the methods an MFA-enabled account can answer with:
// totp when it has a secret, backup when it has a backup code list.
func (s *Store) MFAMethods(ctx context.Context, userID uint64) ([]string, bool, error) {
	if err := s.check(); err != nil {
		return nil, false, err
	}
	var row mfaRow
	err := s.DB.WithContext(ctx).Table(s.Tables.MFA).Where("user_id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if row.Enabled != 1 {
		return nil, false, nil
	}
	methods := make([]string, 0, 2)
	if strings.TrimSpace(row.SealedTOTPSecret) != "" {
		methods = append(methods, MethodTOTP)
	}
	if strings.TrimSpace(row.BackupCodeHashes) != "" {
		methods = append(methods, MethodBackup)
	}
	if len(methods) == 0 {
		methods = append(methods, MethodTOTP)
	}
	return methods, true, nil
}

// RecordMFAAttempt records an MFA check at login.
func (s *Store) RecordMFAAttempt(ctx context.Context, userID uint64, ip, userAgent string, success bool, method string) error {
	if err := s.check(); err != nil {
		return err
	}
	if s.Tables.MFAAttempt == "" {
		return errors.New("the MFA attempt table is not configured")
	}
	if len(userAgent) > 255 {
		userAgent = userAgent[:255]
	}
	if len(ip) > 64 {
		ip = ip[:64]
	}
	return s.DB.WithContext(ctx).Table(s.Tables.MFAAttempt).Create(map[string]any{
		"id": uuid.NewString(), "user_id": userID, "ip": ip, "user_agent": userAgent,
		"success": flag(success), "method": method, "created_at": s.now().Unix(),
	}).Error
}
