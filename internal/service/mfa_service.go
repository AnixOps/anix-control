package service

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/model"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

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

// IsEnabled 检查MFA是否启用
func (s *MFAService) IsEnabled() bool {
	return s.config != nil && s.config.Enabled
}

// IsEnforcedForUser 检查是否对用户强制MFA
func (s *MFAService) IsEnforcedForUser(user *model.User) bool {
	if s.config == nil || !s.config.Enabled {
		return false
	}
	if s.config.EnforceForAll {
		return true
	}
	if s.config.EnforceForAdmin && user.IsAdmin == 1 {
		return true
	}
	return false
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

// IsUserMFAEnabled 检查用户是否启用了MFA
func (s *MFAService) IsUserMFAEnabled(userID uint) (bool, error) {
	mfa, err := s.GetUserMFA(userID)
	if err != nil {
		return false, err
	}
	return mfa != nil && mfa.Enabled, nil
}

// SetupTOTP 设置TOTP
func (s *MFAService) SetupTOTP(userID uint, email string) (*TOTPSetup, error) {
	// 生成密钥
	issuer := "V2Board"
	if s.config != nil && s.config.TOTPIssuer != "" {
		issuer = s.config.TOTPIssuer
	}

	accountName := email
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      issuer,
		AccountName: accountName,
		SecretSize:  32,
	})
	if err != nil {
		return nil, err
	}

	// 生成备用码
	backupCodes, err := s.generateBackupCodes()
	if err != nil {
		return nil, err
	}

	// 创建或更新MFA记录
	var mfa model.UserMFA
	result := s.db.Where("user_id = ?", userID).First(&mfa)
	if result.Error == gorm.ErrRecordNotFound {
		mfa = model.UserMFA{
			UserID:      userID,
			TOTPSecret:  key.Secret(),
			BackupCodes: backupCodes,
			Enabled:     false, // 需要验证后才启用
		}
		if err := s.db.Create(&mfa).Error; err != nil {
			return nil, err
		}
	} else {
		mfa.TOTPSecret = key.Secret()
		mfa.BackupCodes = backupCodes
		if err := s.db.Save(&mfa).Error; err != nil {
			return nil, err
		}
	}

	return &TOTPSetup{
		Secret:      key.Secret(),
		URL:         key.URL(),
		QRCode:      key.String(),
		BackupCodes: parseBackupCodes(backupCodes),
	}, nil
}

// EnableTOTP 启用TOTP
func (s *MFAService) EnableTOTP(userID uint, code string) error {
	mfa, err := s.GetUserMFA(userID)
	if err != nil {
		return err
	}
	if mfa == nil {
		return fmt.Errorf("MFA not setup")
	}

	// 验证代码
	if !s.VerifyTOTP(mfa.TOTPSecret, code) {
		return fmt.Errorf("invalid code")
	}

	// 启用MFA
	mfa.Enabled = true
	now := time.Now()
	mfa.LastUsed = &now
	mfa.LastMethod = model.MFAMethodTOTP

	return s.db.Save(mfa).Error
}

// DisableMFA 禁用MFA
func (s *MFAService) DisableMFA(userID uint, password string) error {
	// 验证密码
	var user model.User
	if err := s.db.First(&user, userID).Error; err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return fmt.Errorf("invalid password")
	}

	return s.db.Where("user_id = ?", userID).Delete(&model.UserMFA{}).Error
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
func (s *MFAService) VerifyBackupCode(mfa *model.UserMFA, code string) bool {
	codes := parseBackupCodes(mfa.BackupCodes)
	for i, c := range codes {
		if c == code {
			// 移除已使用的备用码
			codes = append(codes[:i], codes[i+1:]...)
			newCodes, _ := json.Marshal(codes)
			s.db.Model(mfa).Update("backup_codes", string(newCodes))
			return true
		}
	}
	return false
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

	var valid bool
	switch method {
	case model.MFAMethodTOTP:
		valid = s.VerifyTOTP(mfa.TOTPSecret, code)
	case model.MFAMethodBackup:
		valid = s.VerifyBackupCode(mfa, code)
	case model.MFAMethodEmail:
		// TODO: 实现邮箱验证
		valid = false
	case model.MFAMethodSMS:
		// TODO: 实现短信验证
		valid = false
	default:
		// 默认尝试验证TOTP
		valid = s.VerifyTOTP(mfa.TOTPSecret, code)
		if !valid {
			// 再尝试备用码
			valid = s.VerifyBackupCode(mfa, code)
		}
	}

	if valid {
		// 更新最后使用时间
		now := time.Now()
		s.db.Model(mfa).Updates(map[string]interface{}{
			"last_used":   now,
			"last_method": method,
		})
	}

	return valid, nil
}

// RecordLoginAttempt 记录登录尝试
func (s *MFAService) RecordLoginAttempt(userID uint, ip, userAgent string, success bool, method string) error {
	attempt := &model.MFALoginAttempt{
		UserID:    userID,
		IP:        ip,
		UserAgent: userAgent,
		Success:   success,
		Method:    method,
	}
	return s.db.Create(attempt).Error
}

// CheckBruteForce 检查暴力破解
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

// generateBackupCodes 生成备用码
func (s *MFAService) generateBackupCodes() (string, error) {
	count := 10
	if s.config != nil && s.config.BackupCodeCount > 0 {
		count = s.config.BackupCodeCount
	}

	codes := make([]string, count)
	for i := 0; i < count; i++ {
		code, err := generateRandomCode(8)
		if err != nil {
			return "", err
		}
		// 格式: XXXX-XXXX
		codes[i] = code[:4] + "-" + code[4:]
	}

	jsonCodes, err := json.Marshal(codes)
	if err != nil {
		return "", err
	}
	return string(jsonCodes), nil
}

// generateRandomCode 生成随机代码
func generateRandomCode(length int) (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}

// parseBackupCodes 解析备用码
func parseBackupCodes(jsonCodes string) []string {
	var codes []string
	json.Unmarshal([]byte(jsonCodes), &codes)
	return codes
}

// GetRemainingBackupCodes 获取剩余备用码数量
func (s *MFAService) GetRemainingBackupCodes(userID uint) (int, error) {
	mfa, err := s.GetUserMFA(userID)
	if err != nil {
		return 0, err
	}
	if mfa == nil {
		return 0, nil
	}
	return len(parseBackupCodes(mfa.BackupCodes)), nil
}

// RegenerateBackupCodes 重新生成备用码
func (s *MFAService) RegenerateBackupCodes(userID uint) ([]string, error) {
	mfa, err := s.GetUserMFA(userID)
	if err != nil {
		return nil, err
	}
	if mfa == nil || !mfa.Enabled {
		return nil, fmt.Errorf("MFA not enabled")
	}

	backupCodes, err := s.generateBackupCodes()
	if err != nil {
		return nil, err
	}

	mfa.BackupCodes = backupCodes
	if err := s.db.Save(mfa).Error; err != nil {
		return nil, err
	}

	return parseBackupCodes(backupCodes), nil
}

// TOTPSetup TOTP设置结果
type TOTPSetup struct {
	Secret      string   `json:"secret"`        // 密钥 (base32)
	URL         string   `json:"url"`           // otpauth:// URL
	QRCode      string   `json:"qr_code"`       // 用于生成二维码
	BackupCodes []string `json:"backup_codes"`  // 备用码
}

// GenerateQRCodeURL 生成二维码URL (用于Google Chart API等)
func (s *MFAService) GenerateQRCodeURL(secret, email string) string {
	issuer := "V2Board"
	if s.config != nil && s.config.TOTPIssuer != "" {
		issuer = s.config.TOTPIssuer
	}
	return fmt.Sprintf("otpauth://totp/%s:%s?secret=%s&issuer=%s",
		issuer, email, secret, issuer)
}

// ValidateCodeFormat 验证代码格式
func ValidateCodeFormat(code string) bool {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, " ", "")
	return len(code) == 6 || len(code) == 8
}

// EncodeSecret 编码密钥
func EncodeSecret(secret []byte) string {
	return base32.StdEncoding.EncodeToString(secret)
}

// DecodeSecret 解码密钥
func DecodeSecret(encoded string) ([]byte, error) {
	return base32.StdEncoding.DecodeString(encoded)
}