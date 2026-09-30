package account

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TOTPSetup is a new TOTP secret and backup codes, shown to the user once.
type TOTPSetup struct {
	Secret      string
	URL         string
	QRCode      string
	BackupCodes []string
}

// MFA errors, with the messages the v2 API shows.
var (
	ErrMFANotSetup   = errors.New("MFA not setup")
	ErrMFANotEnabled = errors.New("MFA not enabled")
	ErrInvalidCode   = errors.New("invalid code")
)

// SetupTOTP creates a TOTP secret and backup codes for an account. An
// existing second factor gets them replaced; whether it is enabled stays as
// it was, and a new one starts disabled until EnableTOTP.
func (s *Store) SetupTOTP(ctx context.Context, userID uint64, issuer, accountName string, backupCodes int) (TOTPSetup, error) {
	if err := s.check(); err != nil {
		return TOTPSetup{}, err
	}
	key, err := totp.Generate(totp.GenerateOpts{Issuer: issuer, AccountName: accountName, SecretSize: 32})
	if err != nil {
		return TOTPSetup{}, err
	}
	codes, err := newBackupCodes(backupCodes)
	if err != nil {
		return TOTPSetup{}, err
	}
	owner := mfaOwner(userID)
	sealed, err := s.Secrets.Seal(owner, []byte(key.Secret()))
	if err != nil {
		return TOTPSetup{}, err
	}
	hashes, err := s.keyedHashes(owner, codes)
	if err != nil {
		return TOTPSetup{}, err
	}
	now := s.now().Unix()
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing mfaRow
		err := tx.Table(s.Tables.MFA).Where("user_id = ?", userID).Take(&existing).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			return tx.Table(s.Tables.MFA).Create(&mfaRow{
				UserID: userID, SealedTOTPSecret: sealed, BackupCodeHashes: hashes, UpdatedAt: now,
			}).Error
		case err != nil:
			return err
		default:
			return tx.Table(s.Tables.MFA).Where("user_id = ?", userID).
				Updates(map[string]any{"sealed_totp_secret": sealed, "backup_code_hashes": hashes, "updated_at": now}).Error
		}
	})
	if err != nil {
		return TOTPSetup{}, err
	}
	return TOTPSetup{Secret: key.Secret(), URL: key.URL(), QRCode: key.String(), BackupCodes: codes}, nil
}

// EnableTOTP enables the second factor once a code from the new secret
// checks out.
func (s *Store) EnableTOTP(ctx context.Context, userID uint64, code string) error {
	if err := s.check(); err != nil {
		return err
	}
	var row mfaRow
	err := s.DB.WithContext(ctx).Table(s.Tables.MFA).Where("user_id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrMFANotSetup
	}
	if err != nil {
		return err
	}
	secret, err := s.Secrets.Open(mfaOwner(userID), row.SealedTOTPSecret)
	if err != nil {
		return err
	}
	if !validTOTP(string(secret), code, s.now()) {
		return ErrInvalidCode
	}
	now := s.now().Unix()
	updates := map[string]any{"enabled": 1, "last_used": now, "last_method": MethodTOTP, "updated_at": now}
	if row.Enabled != 1 {
		updates["enabled_at"] = now
	}
	return s.DB.WithContext(ctx).Table(s.Tables.MFA).Where("user_id = ?", userID).Updates(updates).Error
}

// DisableMFA removes the account's second factor.
func (s *Store) DisableMFA(ctx context.Context, userID uint64) error {
	if err := s.check(); err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Table(s.Tables.MFA).Where("user_id = ?", userID).Delete(&mfaRow{}).Error
}

// RegenerateBackupCodes replaces the backup codes of an enabled second
// factor.
func (s *Store) RegenerateBackupCodes(ctx context.Context, userID uint64, count int) ([]string, error) {
	if err := s.check(); err != nil {
		return nil, err
	}
	codes, err := newBackupCodes(count)
	if err != nil {
		return nil, err
	}
	hashes, err := s.keyedHashes(mfaOwner(userID), codes)
	if err != nil {
		return nil, err
	}
	err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row mfaRow
		err := tx.Table(s.Tables.MFA).Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ?", userID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && row.Enabled != 1) {
			return ErrMFANotEnabled
		}
		if err != nil {
			return err
		}
		return tx.Table(s.Tables.MFA).Where("user_id = ?", userID).
			Updates(map[string]any{"backup_code_hashes": hashes, "updated_at": s.now().Unix()}).Error
	})
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// MFAStatus describes an account's second factor.
type MFAStatus struct {
	Exists         bool
	Enabled        bool
	HasBackupCodes bool
	RemainingCodes int
	LastUsed       time.Time
	LastMethod     string
}

// Status returns the account's second factor state.
func (s *Store) Status(ctx context.Context, userID uint64) (MFAStatus, error) {
	if err := s.check(); err != nil {
		return MFAStatus{}, err
	}
	var row mfaRow
	err := s.DB.WithContext(ctx).Table(s.Tables.MFA).Where("user_id = ?", userID).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return MFAStatus{}, nil
	}
	if err != nil {
		return MFAStatus{}, err
	}
	var keyed []string
	remaining := 0
	if json.Unmarshal([]byte(row.BackupCodeHashes), &keyed) == nil {
		remaining = len(keyed)
	}
	return MFAStatus{
		Exists: true, Enabled: row.Enabled == 1, HasBackupCodes: len(row.BackupCodeHashes) > 0, RemainingCodes: remaining,
		LastUsed: fromUnix(row.LastUsed), LastMethod: row.LastMethod,
	}, nil
}

func (s *Store) keyedHashes(owner string, codes []string) (string, error) {
	keyed := make([]string, 0, len(codes))
	for _, code := range codes {
		sum := sha256.Sum256([]byte(code))
		keyed = append(keyed, s.Secrets.Hash(owner, hex.EncodeToString(sum[:])))
	}
	encoded, err := json.Marshal(keyed)
	return string(encoded), err
}

// newBackupCodes returns count codes XXXX-XXXX from A-Z and 0-9.
func newBackupCodes(count int) ([]string, error) {
	if count <= 0 {
		count = 10
	}
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	codes := make([]string, count)
	for index := range codes {
		raw := make([]byte, 8)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		for position := range raw {
			raw[position] = alphabet[int(raw[position])%len(alphabet)]
		}
		codes[index] = string(raw[:4]) + "-" + string(raw[4:])
	}
	return codes, nil
}
