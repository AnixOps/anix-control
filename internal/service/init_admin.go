package service

import (
	"crypto/rand"
	"log"
	"math/big"
	"time"

	"github.com/AnixOps/anix-control/v3/internal/config"
	"github.com/AnixOps/anix-control/v3/internal/database"
	"github.com/AnixOps/anix-control/v3/internal/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// InitAdmin 初始化管理员账号
func InitAdmin(cfg *config.Config) {
	// 确保表存在
	if !database.GetDB().Migrator().HasTable(&model.User{}) {
		if err := database.GetDB().AutoMigrate(&model.User{}); err != nil {
			log.Fatalf("Failed to migrate admin user table: %v", err)
		}
	}

	var count int64
	if err := database.GetDB().Model(&model.User{}).Where("is_admin = ?", 1).Count(&count).Error; err != nil {
		log.Fatalf("Failed to check admin account: %v", err)
	}

	if count > 0 {
		log.Println("Admin account already exists, skipping creation.")
		return
	}

	log.Println("No admin account found, creating default admin...")

	email := cfg.Admin.Email
	password := cfg.Admin.Password

	if email == "" {
		email = "admin@anixops.local"
	}

	if password == "" {
		generatedPassword, err := generateRandomPassword(32)
		if err != nil {
			log.Fatalf("Failed to generate admin password: %v", err)
		}
		password = generatedPassword
		log.Println("Admin password is empty; generated a random bootstrap password.")
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

func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	max := big.NewInt(int64(len(charset)))
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}
