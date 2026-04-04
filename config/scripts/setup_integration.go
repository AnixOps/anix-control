package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/anixops/v2board/internal/database"
	"github.com/anixops/v2board/internal/model"
)

func hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func main() {
	cfg, err := config.Load("config/config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := database.Init(&cfg.Database); err != nil {
		log.Fatalf("Failed to init database: %v", err)
	}
	defer database.Close()

	db := database.Get()

	// 1. 确保有一个 NodeGroup
	var group model.NodeGroup
	if err := db.First(&group).Error; err != nil {
		group = model.NodeGroup{Name: "默认分组"}
		db.Create(&group)
		fmt.Printf("Created NodeGroup ID: %d\n", group.ID)
	} else {
		fmt.Printf("Existing NodeGroup ID: %d\n", group.ID)
	}

	// 2. 确保有一个 AuthKey
	var authKey model.AuthorizedKey
	fixedKey := "anixops-integration-test-key"
	keyHash := hashString(fixedKey)

	if err := db.Where("key_hash = ?", keyHash).First(&authKey).Error; err != nil {
		authKey = model.AuthorizedKey{
			Name:    "Integration Test Key",
			Key:     fixedKey,
			KeyHash: keyHash,
			Used:    0,
		}
		db.Create(&authKey)
		fmt.Printf("Generated Known AuthKey: %s\n", fixedKey)
	} else {
		fmt.Printf("AuthKey already exists: %s\n", fixedKey)
	}

	// 3. 确保用户 ID 2 有权限和流量
	var user model.User
	if err := db.First(&user, 2).Error; err == nil {
		user.GroupID = &group.ID
		user.TransferEnable = 1024 * 1024 * 1024 * 100 // 100GB
		expiredAt := time.Now().Add(time.Hour * 24 * 30).Unix()
		user.ExpiredAt = &expiredAt
		db.Save(&user)
		fmt.Printf("Updated User 2: GroupID=%d, Traffic=100GB, Token=%s\n", group.ID, user.Token)
	} else {
		fmt.Println("User 2 not found, skipping user update")
	}
}
