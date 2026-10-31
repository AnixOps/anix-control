package model

import (
	"time"
)

// SystemConfig 系统配置
type SystemConfig struct {
	ID     uint   `gorm:"primaryKey" json:"id"`
	Key    string `gorm:"size:100;uniqueIndex" json:"key"`
	Value  string `gorm:"type:text" json:"value"`
	Type   string `gorm:"size:20;default:string" json:"type"` // string/number/boolean/json
	Group  string `gorm:"size:50;index" json:"group"`         // 配置分组
	Remark string `gorm:"size:255" json:"remark"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (SystemConfig) TableName() string {
	return "v2_system_config"
}

// The former BackupRecord (v2_backup_record) and BackupConfig
// (v2_backup_config) models went with the backup settings nothing executed
// and the online restore: Control's backups are Backup (v5_backup,
// internal/backup). Both tables stay in the schema, unused, until a cleanup
// migration drops them; the migration tests keep frozen copies of the two
// models (internal/database/migrations, legacy_oracle_test.go).

// OperationLog 操作日志
type OperationLog struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	UserID     *uint  `gorm:"index" json:"user_id"`
	Username   string `gorm:"size:100" json:"username"`
	Action     string `gorm:"size:50;index" json:"action"` // create/update/delete/login/logout
	Module     string `gorm:"size:50;index" json:"module"` // user/node/order/plan...
	TargetType string `gorm:"size:50" json:"target_type"`  // 目标类型
	TargetID   *uint  `json:"target_id"`                   // 目标ID
	Content    string `gorm:"type:text" json:"content"`    // 操作内容
	IP         string `gorm:"size:45" json:"ip"`
	UserAgent  string `gorm:"size:255" json:"user_agent"`
	Status     int    `gorm:"default:1" json:"status"` // 1=成功 2=失败

	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名
func (OperationLog) TableName() string {
	return "v2_operation_log"
}
