package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"gorm.io/gorm"
)

// ErrMFAAlreadyEnabled refuses a new TOTP secret for an account whose second
// factor is enabled; the identity module says the same.
var ErrMFAAlreadyEnabled = errors.New("MFA already enabled; disable it first")

// MFAService MFA服务
type MFAService struct {
	db     *gorm.DB
	config *model.MFAConfig
}

// NewMFAService 创建服务
func NewMFAService(db *gorm.DB, config *model.MFAConfig) *MFAService {
	return &MFAService{
		db:     db,
		config: config,
	}
}

// GetUserMFA 获取用户MFA配置
func (s *MFAService) GetUserMFA(userID uint) (*model.UserMFA, error) {
	var mfa model.UserMFA
	err := s.db.Where("user_id = ?", userID).First(&mfa).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &mfa, nil
}

// VerifyTOTP 验证TOTP代码
func (s *MFAService) VerifyTOTP(secret, code string) bool {
	valid, err := totp.ValidateCustom(code, secret, time.Now(), totp.ValidateOpts{
		Period:    30,
		Skew:      1,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	})
	if err != nil {
		return false
	}
	return valid
}

// VerifyBackupCode 验证备用码
func (s *MFAService) VerifyBackupCode(mfa *model.UserMFA, code string) (bool, error) {
	codes, err := parseBackupCodesWithError(mfa.BackupCodes)
	if err != nil {
		return false, err
	}

	for i, c := range codes {
		if c == code {
			// 移除已使用的备用码
			codes = append(codes[:i], codes[i+1:]...)
			newCodes, err := json.Marshal(codes)
			if err != nil {
				return false, err
			}
			if err := s.db.Model(mfa).Update("backup_codes", string(newCodes)).Error; err != nil {
				return false, err
			}
			return true, nil
		}
	}
	return false, nil
}

// Verify 验证MFA代码
func (s *MFAService) Verify(userID uint, code, method string) (bool, error) {
	mfa, err := s.GetUserMFA(userID)
	if err != nil {
		return false, err
	}
	if mfa == nil || !mfa.Enabled {
		return true, nil // MFA未启用，直接通过
	}

	method = strings.ToLower(strings.TrimSpace(method))
	usedMethod := method
	var valid bool
	switch method {
	case model.MFAMethodTOTP:
		valid = s.VerifyTOTP(mfa.TOTPSecret, code)
	case model.MFAMethodBackup:
		valid, err = s.VerifyBackupCode(mfa, code)
		if err != nil {
			return false, err
		}
	case model.MFAMethodEmail:
		// Unimplemented: email-based MFA verification.
		// When implemented, this should look up a time-limited verification code
		// sent to the user's email and compare it with the provided code.
		log.Printf("[mfa] email MFA method not implemented")
		valid = false
	case model.MFAMethodSMS:
		// Unimplemented: SMS-based MFA verification.
		// When implemented, this should look up a time-limited verification code
		// sent via SMS gateway and compare it with the provided code.
		log.Printf("[mfa] SMS MFA method not implemented")
		valid = false
	default:
		// 默认尝试验证TOTP
		usedMethod = model.MFAMethodTOTP
		valid = s.VerifyTOTP(mfa.TOTPSecret, code)
		if !valid {
			// 再尝试备用码
			valid, err = s.VerifyBackupCode(mfa, code)
			if err != nil {
				return false, err
			}
			if valid {
				usedMethod = model.MFAMethodBackup
			}
		}
	}

	if valid {
		// 更新最后使用时间
		now := time.Now()
		if err := s.db.Model(mfa).Updates(map[string]any{
			"last_used":   now,
			"last_method": usedMethod,
		}).Error; err != nil {
			return false, err
		}
	}

	return valid, nil
}

// CheckBruteForce 检查暴力破解
//
// Implemented but not yet wired into the MFA verification flow; see TODO.md
// ("Implemented but not wired").
func (s *MFAService) CheckBruteForce(userID uint, maxAttempts int, window time.Duration) (bool, error) {
	var count int64
	since := time.Now().Add(-window)
	err := s.db.Model(&model.MFALoginAttempt{}).
		Where("user_id = ? AND success = ? AND created_at > ?", userID, false, since).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count >= int64(maxAttempts), nil
}

func parseBackupCodesWithError(jsonCodes string) ([]string, error) {
	if strings.TrimSpace(jsonCodes) == "" {
		return []string{}, nil
	}

	var codes []string
	if err := json.Unmarshal([]byte(jsonCodes), &codes); err != nil {
		return nil, fmt.Errorf("parse MFA backup codes: %w", err)
	}
	return codes, nil
}

// TOTPSetup TOTP设置结果
type TOTPSetup struct {
	Secret      string   `json:"secret"`       // 密钥 (base32)
	URL         string   `json:"url"`          // otpauth:// URL
	QRCode      string   `json:"qr_code"`      // 用于生成二维码
	BackupCodes []string `json:"backup_codes"` // 备用码
}
