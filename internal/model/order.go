package model

import "time"

// Order 订单模型
type Order struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	InviteUserID      *uint     `gorm:"index" json:"invite_user_id"`
	UserID            uint      `gorm:"index" json:"user_id"`
	PlanID            uint      `gorm:"index" json:"plan_id"`
	CouponID          *uint     `gorm:"index" json:"coupon_id"`
	PaymentID         *uint     `gorm:"index" json:"payment_id"`
	Type              int       `json:"type"` // 1: new, 2: renew, 3: upgrade, 4: reset
	Period            string    `gorm:"size:20" json:"period"`
	TradeNo           string    `gorm:"size:36;uniqueIndex" json:"trade_no"`
	CallbackNo        *string   `gorm:"size:255" json:"callback_no"`
	TotalAmount       int64     `json:"total_amount"`
	DiscountAmount    *int64    `json:"discount_amount"`
	SurplusAmount     *int64    `json:"surplus_amount"`
	RefundAmount      *int64    `json:"refund_amount"`
	Balance           *int64    `json:"balance"`
	SurplusOrder      *string   `gorm:"type:text" json:"surplus_order"`
	Status            int       `gorm:"default:0" json:"status"` // 0: pending, 1: paid, 2: cancelled, 3: completed, 4: discounted
	CommissionStatus  int       `gorm:"default:0" json:"commission_status"`
	CommissionBalance int64     `gorm:"default:0" json:"commission_balance"`
	PaidAt            *int64    `json:"paid_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// Relations
	User *User `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Plan *Plan `gorm:"foreignKey:PlanID" json:"plan,omitempty"`
}

func (Order) TableName() string {
	return "v2_order"
}

// TrafficLog is one hourly bucket of the traffic one user used on one node:
// the bytes of every report in LogAt's hour, as reported (before Rate). A
// report adds to the row of its (node, hour, user, server type, rate) key
// with an upsert on TrafficLogBucketIndex, so a node's reports of an hour
// are one row per user instead of one row per report. Rows written before
// the buckets exist keep their own LogAt (a report's second): readers sum
// rows, so they stay valid next to the buckets.
//
// CreatedAt is the time of the report that last touched the row: the one that
// inserted it, or the latest that added to it, so the newest bucket's
// CreatedAt is the latest report's time even though LogAt is the hour.
//
// The raw log is kept for the retention of internal/retention.
type TrafficLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index;index:idx_server_log_log_at_user_id,priority:2;uniqueIndex:idx_v2_server_log_bucket,priority:3" json:"user_id"`
	ServerID   uint      `gorm:"index;uniqueIndex:idx_v2_server_log_bucket,priority:1" json:"server_id"`
	ServerType string    `gorm:"size:20;uniqueIndex:idx_v2_server_log_bucket,priority:4" json:"server_type"`
	U          int64     `json:"u"` // upload bytes
	D          int64     `json:"d"` // download bytes
	Rate       float64   `gorm:"default:1;uniqueIndex:idx_v2_server_log_bucket,priority:5" json:"rate"`
	LogAt      int64     `gorm:"index;index:idx_server_log_log_at_user_id,priority:1;uniqueIndex:idx_v2_server_log_bucket,priority:2" json:"log_at"` // start of the hour, Unix seconds
	CreatedAt  time.Time `json:"created_at"`
}

func (TrafficLog) TableName() string {
	return "v2_server_log"
}

const (
	// TrafficLogBucketSeconds is the width of a TrafficLog bucket. Readers
	// group rows by (log_at / 3600), so a bucket is one of their hours.
	TrafficLogBucketSeconds = 3600
	// TrafficLogBucketIndex is the unique index on the bucket key, the
	// conflict target of the upsert.
	TrafficLogBucketIndex = "idx_v2_server_log_bucket"
)

// TrafficLogBucketColumns are the columns of the bucket key, in the order of
// TrafficLogBucketIndex.
var TrafficLogBucketColumns = []string{"server_id", "log_at", "user_id", "server_type", "rate"}

// TrafficLogBucketStart returns the start of the bucket (the UTC hour) that
// contains the Unix time at.
func TrafficLogBucketStart(at int64) int64 {
	start := at - at%TrafficLogBucketSeconds
	if at < 0 && at%TrafficLogBucketSeconds != 0 {
		start -= TrafficLogBucketSeconds
	}
	return start
}

// OnlineLog 在线日志
type OnlineLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	ServerID   uint      `gorm:"index" json:"server_id"`
	ServerType string    `gorm:"size:20" json:"server_type"`
	Method     string    `gorm:"size:20" json:"method"`
	IP         string    `gorm:"size:45" json:"ip"`
	CreatedAt  time.Time `json:"created_at"`
}

func (OnlineLog) TableName() string {
	return "v2_online_log"
}

// StatServer 服务器统计
type StatServer struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	ServerID   uint   `gorm:"index" json:"server_id"`
	ServerType string `gorm:"size:20" json:"server_type"`
	U          int64  `json:"u"`
	D          int64  `json:"d"`
	RecordType string `gorm:"size:1" json:"record_type"` // d: day, m: month
	RecordAt   int64  `gorm:"index" json:"record_at"`
}

func (StatServer) TableName() string {
	return "v2_stat_server"
}

// StatUser 用户统计
type StatUser struct {
	ID         uint    `gorm:"primaryKey" json:"id"`
	UserID     uint    `gorm:"index" json:"user_id"`
	ServerRate float64 `json:"server_rate"`
	U          int64   `json:"u"`
	D          int64   `json:"d"`
	RecordType string  `gorm:"size:1" json:"record_type"`
	RecordAt   int64   `gorm:"index" json:"record_at"`
}

func (StatUser) TableName() string {
	return "v2_stat_user"
}
