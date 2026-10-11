package service

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ErrAdminPasswordRequired means the first administrator must be created but no
// password is configured. A password is never generated or printed: set
// ANIX_CONTROL_ADMIN_PASSWORD or ANIX_CONTROL_ADMIN_PASSWORD_FILE before the first start.
var ErrAdminPasswordRequired = errors.New("no administrator exists and no admin password is configured: set ANIX_CONTROL_ADMIN_PASSWORD or ANIX_CONTROL_ADMIN_PASSWORD_FILE before the first start")

// InitAdmin creates the first administrator when the database has none. It creates
// nothing and returns ErrAdminPasswordRequired when no password is configured.
func InitAdmin(cfg *config.Config) error {
	return initAdmin(database.GetDB(), cfg)
}

func initAdmin(db *gorm.DB, cfg *config.Config) error {
	// 确保表存在
	if !db.Migrator().HasTable(&model.User{}) {
		if err := db.AutoMigrate(&model.User{}); err != nil {
			return fmt.Errorf("migrate admin user table: %w", err)
		}
	}

	// Once identity is authoritative a v2_user row alone cannot log in:
	// administrators are created through identity.
	if db.Migrator().HasTable(&model.IdentityAuthority{}) {
		state, err := IdentityAuthorityState(db)
		if err != nil {
			return fmt.Errorf("read the identity authority: %w", err)
		}
		if IdentityAuthoritative(state) {
			log.Println("Identity is authoritative; skipping the default admin (create administrators through identity).")
			return nil
		}
	}

	var count int64
	if err := db.Model(&model.User{}).Where("is_admin = ?", 1).Count(&count).Error; err != nil {
		return fmt.Errorf("check admin account: %w", err)
	}

	if count > 0 {
		log.Println("Admin account already exists, skipping creation.")
		return nil
	}

	email := cfg.Admin.Email
	password := cfg.Admin.Password

	if password == "" {
		return ErrAdminPasswordRequired
	}
	if email == "" {
		email = "admin@anixops.local"
	}

	log.Println("No admin account found, creating the first administrator...")

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
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

	if err := db.Create(&admin).Error; err != nil {
		return fmt.Errorf("create admin account: %w", err)
	}

	log.Printf("Admin account created: %s (the password is the one configured by the operator)\n", email)
	return nil
}
