package service

import (
	"errors"
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

// Register 用户注册
func (s *AuthService) Register(email, password string, cfg *config.Config) (string, *model.User, error) {
	db := database.GetDB()

	// 检查邮箱是否已存在
	var existingUser model.User
	if err := db.Where("email = ?", email).First(&existingUser).Error; err == nil {
		return "", nil, errors.New("该邮箱已被注册")
	}

	// 密码加密
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", nil, errors.New("密码加密失败")
	}

	// 生成 UUID 和 Token
	userUUID := uuid.New().String()
	userToken := uuid.New().String()

	// 创建用户
	user := &model.User{
		Email:    email,
		Password: string(hashedPassword),
		UUID:     userUUID,
		Token:    userToken,
		IsAdmin:  0,
		Banned:   0,
	}

	if err := db.Create(user).Error; err != nil {
		return "", nil, errors.New("注册失败，请稍后重试")
	}

	// 生成 JWT Token
	token, err := utils.GenerateToken(user.ID, user.Email, false, cfg.JWT.Secret, cfg.JWT.Expire)
	if err != nil {
		return "", nil, err
	}

	return token, user, nil
}

// Login 用户登录
func (s *AuthService) Login(email, password string, cfg *config.Config) (string, *model.User, error) {
	var user model.User
	if err := database.GetDB().Where("email = ?", email).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", nil, errors.New("用户不存在或密码错误")
		}
		return "", nil, err
	}

	// 验证密码
	if !checkPassword(password, user.Password) {
		return "", nil, errors.New("用户不存在或密码错误")
	}

	// 检查是否被封禁
	if user.Banned == 1 {
		return "", nil, errors.New("用户已被封禁")
	}
	if user.ExpiredAt != nil && *user.ExpiredAt > 0 && *user.ExpiredAt <= time.Now().Unix() {
		return "", nil, errors.New("用户已过期")
	}

	// 生成Token
	token, err := utils.GenerateToken(user.ID, user.Email, user.IsAdmin == 1, cfg.JWT.Secret, cfg.JWT.Expire)
	if err != nil {
		return "", nil, err
	}

	return token, &user, nil
}

// checkPassword 验证密码
func checkPassword(password, hashedPassword string) bool {
	// 这里假设使用bcrypt
	// 如果是旧系统迁移，可能需要支持其他算法
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	return err == nil
}
