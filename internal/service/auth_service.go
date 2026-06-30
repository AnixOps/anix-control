package service

import (
	"errors"
	"strings"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/anixops/v2board/internal/utils"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AuthService struct{}

func NewAuthService() *AuthService {
	return &AuthService{}
}

// Register keeps the existing public API and registers without an invite code.
func (s *AuthService) Register(email, password string, cfg *config.Config) (string, *model.User, error) {
	return s.RegisterWithInvite(email, password, "", cfg)
}

// RegisterWithInvite creates a user and optionally consumes an invite code.
func (s *AuthService) RegisterWithInvite(email, password, inviteCode string, cfg *config.Config) (string, *model.User, error) {
	db := database.GetDB()
	policy := ResolveRegistrationPolicy(cfg)
	if !policy.Enabled {
		return "", nil, errors.New("registration is disabled")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	inviteCode = strings.TrimSpace(inviteCode)
	if err := ValidateRegistrationEmail(email, policy); err != nil {
		return "", nil, err
	}
	if policy.RequireInvite && inviteCode == "" {
		return "", nil, errors.New("invite code is required")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, errors.New("密码加密失败")
	}

	user := &model.User{
		Email:    email,
		Password: string(hashedPassword),
		UUID:     uuid.New().String(),
		Token:    uuid.New().String(),
		IsAdmin:  0,
		Banned:   0,
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		var existingUser model.User
		if err := tx.Where("email = ?", email).First(&existingUser).Error; err == nil {
			return errors.New("该邮箱已被注册")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var invite *model.InviteCode
		if inviteCode != "" {
			var record model.InviteCode
			if err := tx.Where("code = ? AND status = 0", inviteCode).First(&record).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return errors.New("invalid or used invite code")
				}
				return err
			}
			if record.ExpiredAt != nil && record.ExpiredAt.Before(time.Now()) {
				return errors.New("invite code expired")
			}
			invite = &record
			if invite.UserID != nil {
				user.InviteUserID = invite.UserID
			}
		}

		if err := tx.Create(user).Error; err != nil {
			return errors.New("注册失败，请稍后重试")
		}

		if invite != nil {
			now := time.Now()
			invite.Status = 1
			invite.UsedBy = &user.ID
			invite.UsedAt = &now
			if err := tx.Save(invite).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return "", nil, err
	}

	token, err := utils.GenerateToken(user.ID, user.Email, false, cfg.JWT.Secret, cfg.JWT.Expire)
	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

// Login validates credentials and returns a JWT token.
func (s *AuthService) Login(email, password string, cfg *config.Config) (string, *model.User, error) {
	var user model.User
	if err := database.GetDB().Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, errors.New("用户不存在或密码错误")
		}
		return "", nil, err
	}

	if !checkPassword(password, user.Password) {
		return "", nil, errors.New("用户不存在或密码错误")
	}

	if user.Banned == 1 {
		return "", nil, errors.New("用户已被封禁")
	}
	if user.ExpiredAt != nil && *user.ExpiredAt > 0 && *user.ExpiredAt <= time.Now().Unix() {
		return "", nil, errors.New("用户已过期")
	}

	token, err := utils.GenerateToken(user.ID, user.Email, user.IsAdmin == 1, cfg.JWT.Secret, cfg.JWT.Expire)
	if err != nil {
		return "", nil, err
	}

	return token, &user, nil
}

// checkPassword verifies a bcrypt password hash.
func checkPassword(password, hashedPassword string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
