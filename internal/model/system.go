package model

import (
	"time"
)

// LoadBalancer 负载均衡器
type LoadBalancer struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Name          string    `gorm:"size:100" json:"name"`
	GroupID       uint      `gorm:"index" json:"group_id"`         // 节点组ID
	Strategy      string    `gorm:"size:20;default:round-robin" json:"strategy"` // round-robin/least-conn/latency/weight/random
	HealthCheck   bool      `gorm:"default:true" json:"health_check"`
	CheckInterval int       `gorm:"default:60" json:"check_interval"` // 检查间隔(秒)
	CheckTimeout  int       `gorm:"default:10" json:"check_timeout"`  // 超时时间(秒)
	NodeWeights   string    `gorm:"type:text" json:"node_weights"`    // JSON: {node_id: weight}
	Enabled       bool      `gorm:"default:true" json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// TableName 指定表名
func (LoadBalancer) TableName() string {
	return "v2_load_balancer"
}

// SystemConfig 系统配置
type SystemConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:100;uniqueIndex" json:"key"`
	Value     string    `gorm:"type:text" json:"value"`
	Type      string    `gorm:"size:20;default:string" json:"type"` // string/number/boolean/json
	Group     string    `gorm:"size:50;index" json:"group"`         // 配置分组
	Remark    string    `gorm:"size:255" json:"remark"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName 指定表名
func (SystemConfig) TableName() string {
	return "v2_system_config"
}

// BackupRecord 备份记录
type BackupRecord struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:100" json:"name"`
	Type        string    `gorm:"size:20" json:"type"`         // full/database/files
	Size        int64     `json:"size"`                        // 文件大小(字节)
	Path        string    `gorm:"size:255" json:"path"`        // 存储路径
	Status      int       `gorm:"default:0" json:"status"`     // 0=进行中 1=成功 2=失败
	Error       string    `gorm:"type:text" json:"error"`      // 错误信息
	Auto        bool      `gorm:"default:false" json:"auto"`   // 是否自动备份
	CreatedBy   *uint     `json:"created_by"`                  // 创建者ID

	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TableName 指定表名
func (BackupRecord) TableName() string {
	return "v2_backup_record"
}

// BackupConfig 备份配置
type BackupConfig struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	Enabled         bool      `gorm:"default:false" json:"enabled"`
	AutoBackup      bool      `gorm:"default:false" json:"auto_backup"`
	Schedule        string    `gorm:"size:50" json:"schedule"`      // cron表达式
	RetentionDays   int       `gorm:"default:7" json:"retention_days"` // 保留天数
	BackupDatabase  bool      `gorm:"default:true" json:"backup_database"`
	BackupFiles     bool      `gorm:"default:false" json:"backup_files"`
	StorageType     string    `gorm:"size:20;default:local" json:"storage_type"` // local/s3/ftp
	StoragePath     string    `gorm:"size:255" json:"storage_path"`
	S3Bucket        string    `gorm:"size:100" json:"s3_bucket"`
	S3Region        string    `gorm:"size:50" json:"s3_region"`
	S3Endpoint      string    `gorm:"size:255" json:"s3_endpoint"`
	S3AccessKey     string    `gorm:"size:100" json:"s3_access_key"`
	S3SecretKey     string    `gorm:"size:100" json:"s3_secret_key"`

	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// TableName 指定表名
func (BackupConfig) TableName() string {
	return "v2_backup_config"
}

// OperationLog 操作日志
type OperationLog struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      *uint     `gorm:"index" json:"user_id"`
	Username    string    `gorm:"size:100" json:"username"`
	Action      string    `gorm:"size:50;index" json:"action"`  // create/update/delete/login/logout
	Module      string    `gorm:"size:50;index" json:"module"`  // user/node/order/plan...
	TargetType  string    `gorm:"size:50" json:"target_type"`   // 目标类型
	TargetID    *uint     `json:"target_id"`                    // 目标ID
	Content     string    `gorm:"type:text" json:"content"`     // 操作内容
	IP          string    `gorm:"size:45" json:"ip"`
	UserAgent   string    `gorm:"size:255" json:"user_agent"`
	Status      int       `gorm:"default:1" json:"status"`      // 1=成功 2=失败

	CreatedAt   time.Time `gorm:"index" json:"created_at"`
}

// TableName 指定表名
func (OperationLog) TableName() string {
	return "v2_operation_log"
}