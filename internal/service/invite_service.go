package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"gorm.io/gorm"
)

// InviteService 邀请服务
//
// It keeps the invite configuration (v2_invite_config) in memory and
// reloads it when the invite settings namespace changed
// (SettingsGeneration): the affiliate module writes the table it adopted,
// then the frontend settings through KernelSettings, which bumps the
// namespace.
type InviteService struct {
	db *gorm.DB

	mu               sync.Mutex
	config           *model.InviteConfig
	configGeneration uint64
}

// NewInviteService 创建服务
func NewInviteService(db *gorm.DB) *InviteService {
	return &InviteService{db: db}
}

// SetConfig 设置配置: cfg becomes the configuration in memory, and every
// other copy reloads.
func (s *InviteService) SetConfig(cfg *model.InviteConfig) {
	generation := BumpSettingsGeneration(SettingsNamespaceInvite)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config, s.configGeneration = cfg, generation
}

// SaveConfig saves cfg and makes it the configuration in memory; every
// other copy reloads.
func (s *InviteService) SaveConfig(cfg *model.InviteConfig) error {
	before := SettingsGeneration(SettingsNamespaceInvite)
	if err := s.db.Save(cfg).Error; err != nil {
		return err
	}
	generation, keep := BumpSettingsGenerationAfter(SettingsNamespaceInvite, before)
	s.mu.Lock()
	defer s.mu.Unlock()
	if keep {
		s.config, s.configGeneration = cfg, generation
	} else {
		s.config = nil
	}
	return nil
}

// GetConfig 获取配置: the copy in memory while the invite settings
// namespace has not changed since it was loaded.
func (s *InviteService) GetConfig() (*model.InviteConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	generation := SettingsGeneration(SettingsNamespaceInvite)
	if s.config != nil && s.configGeneration == generation {
		return s.config, nil
	}
	cfg, err := s.loadConfig()
	if err != nil {
		return nil, err
	}
	s.config, s.configGeneration = cfg, generation
	return cfg, nil
}

// loadConfig reads the configuration from the database, creating the
// default one when there is none.
func (s *InviteService) loadConfig() (*model.InviteConfig, error) {
	var cfg model.InviteConfig
	err := s.db.First(&cfg).Error
	if err == gorm.ErrRecordNotFound {
		// 创建默认配置
		cfg = model.InviteConfig{
			Enabled:             true,
			AutoGenerate:        true,
			CodeCount:           5,
			CodeExpireDays:      0,
			CommissionEnabled:   true,
			CommissionType:      1,
			CommissionRate:      0.1,
			CommissionFixed:     0,
			CommissionMinAmount: 10,
		}
		if createErr := s.db.Create(&cfg).Error; createErr != nil {
			return nil, createErr
		}
	} else if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DefaultInviteCodeLimit is how many unused invite codes a user may hold
// when the invite configuration sets no code_count, as v2board's
// invite_gen_limit defaults to 5.
const DefaultInviteCodeLimit = 5

// ErrInviteCodeLimit is returned by GenerateUserInviteCode when the user
// already holds as many unused invite codes as allowed.
var ErrInviteCodeLimit = errors.New("invite code limit reached")

// InviteCodeLimitMessage is the answer at the limit, v2board's.
const InviteCodeLimitMessage = "The maximum number of creations has been reached"

// GenerateInviteCode 生成邀请码
func (s *InviteService) GenerateInviteCode(userID *uint) (*model.InviteCode, error) {
	inviteCode, err := s.newInviteCode(userID)
	if err != nil {
		return nil, err
	}

	if err := s.db.Create(inviteCode).Error; err != nil {
		return nil, err
	}

	return inviteCode, nil
}

// GenerateUserInviteCode generates an invite code a user asked for. As in
// v2board, a user may hold at most the configured number (code_count,
// default DefaultInviteCodeLimit) of unused codes; an expired code is not
// counted, since it can no longer be used. Codes made otherwise
// (GenerateInviteCode) are not limited.
func (s *InviteService) GenerateUserInviteCode(userID uint) (*model.InviteCode, error) {
	inviteCode, err := s.newInviteCode(&userID)
	if err != nil {
		return nil, err
	}
	err = WithRetryableTransaction(s.db, func(tx *gorm.DB) error {
		// Concurrent generations for one user count one after another: on
		// PostgreSQL under the user's invite code lock, which the affiliate
		// package takes too; SQLite runs one writer at a time and a
		// transaction that read before another's write retries.
		if err := LockUserInviteCodes(tx, userID); err != nil {
			return err
		}
		var unused int64
		if err := tx.Model(&model.InviteCode{}).
			Where("user_id = ? AND status = ? AND (expired_at IS NULL OR expired_at > ?)", userID, 0, time.Now()).
			Count(&unused).Error; err != nil {
			return err
		}
		if unused >= int64(inviteCodeLimit(tx)) {
			return ErrInviteCodeLimit
		}
		return tx.Create(inviteCode).Error
	})
	if err != nil {
		return nil, err
	}
	return inviteCode, nil
}

// inviteCodeLockClass is the first key of the PostgreSQL advisory lock that
// serializes a user's invite code generations ("invc" in ASCII). Advisory
// locks need no grant, so the affiliate package's role takes the same lock
// (packages/affiliate/native); a row lock on v2_user, which no package may
// read or write, would not serialize the two.
const inviteCodeLockClass int32 = 0x696e7663

// InviteCodeLockKeys are the two keys of a user's invite code lock: the
// class and the user id. A user id beyond 31 bits shares its key with
// another user's, which only makes their generations wait for each other.
func InviteCodeLockKeys(userID uint) (int32, int32) {
	return inviteCodeLockClass, int32(uint64(userID) & math.MaxInt32) // #nosec G115 -- masked to 31 bits.
}

// LockUserInviteCodes takes the user's invite code lock until tx ends, on
// PostgreSQL; on SQLite it does nothing.
func LockUserInviteCodes(tx *gorm.DB, userID uint) error {
	if tx.Name() != "postgres" {
		return nil
	}
	class, key := InviteCodeLockKeys(userID)
	return tx.Exec("SELECT pg_advisory_xact_lock(CAST(? AS integer), CAST(? AS integer))", class, key).Error
}

// inviteCodeLimit is the invite configuration's code_count, read from the
// database: the administrator's update changes another InviteService's
// cached copy. A missing configuration or a code_count below one is
// DefaultInviteCodeLimit.
func inviteCodeLimit(db *gorm.DB) int {
	var cfg model.InviteConfig
	if err := db.Select("id", "code_count").First(&cfg).Error; err != nil || cfg.CodeCount < 1 {
		return DefaultInviteCodeLimit
	}
	return cfg.CodeCount
}

// newInviteCode is a new unused code of userID, expiring as configured.
func (s *InviteService) newInviteCode(userID *uint) (*model.InviteCode, error) {
	code, err := generateInviteCodeStr(8)
	if err != nil {
		return nil, err
	}

	inviteCode := &model.InviteCode{
		Code:   code,
		UserID: userID,
		Status: 0,
	}

	// 设置过期时间
	cfg, _ := s.GetConfig()
	if cfg != nil && cfg.CodeExpireDays > 0 {
		expiredAt := time.Now().AddDate(0, 0, cfg.CodeExpireDays)
		inviteCode.ExpiredAt = &expiredAt
	}
	return inviteCode, nil
}

// GetUserInviteCodes 获取用户邀请码
func (s *InviteService) GetUserInviteCodes(userID uint) ([]model.InviteCode, error) {
	var codes []model.InviteCode
	err := s.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&codes).Error
	return codes, err
}

// generateInviteCodeStr 生成随机码
func generateInviteCodeStr(length int) (string, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b)[:length], nil
}
