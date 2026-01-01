package service

import (
	"log"
	"math/rand"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// InitAdmin 初始化管理员账号
func InitAdmin(cfg *config.Config) {
	// 确保表存在
	if !database.GetDB().Migrator().HasTable(&model.User{}) {
		database.GetDB().AutoMigrate(&model.User{})
	}

	var count int64
	database.GetDB().Model(&model.User{}).Where("is_admin = ?", 1).Count(&count)

	if count > 0 {
		log.Println("Admin account already exists, skipping creation.")
		return
	}

	log.Println("No admin account found, creating default admin...")

	email := cfg.Admin.Email
	password := cfg.Admin.Password

	if email == "" {
		email = "admin@v2board.com"
	}

	if password == "" {
		password = "password" // Default for dev if not configured
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash password: %v", err)
	}

	admin := model.User{
		Email:          email,
		Password:       string(hashedPassword),
		IsAdmin:        1,
		UUID:           uuid.New().String(),
		Token:          uuid.New().String(),
		Balance:        0,
		CommissionType: 0,
		Banned:         0,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := database.GetDB().Create(&admin).Error; err != nil {
		log.Fatalf("Failed to create admin account: %v", err)
	}

	log.Println("====================================================")
	log.Println("Admin account created successfully!")
	log.Printf("Email:    %s\n", email)
	log.Printf("Password: %s\n", password)
	log.Println("====================================================")
}

func generateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
