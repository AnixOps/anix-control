package model

import "time"

// Maintenance tables are separate from customer support and its event pipeline.
type MaintenanceEvent struct {
	EventID    string    `gorm:"primaryKey;size:128" json:"event_id"`
	NodeID     uint      `gorm:"index;not null" json:"node_id"`
	StreamKey  string    `gorm:"size:64;index;not null" json:"-"`
	Digest     string    `gorm:"size:64;not null" json:"-"`
	Body       string    `gorm:"type:text;not null" json:"-"`
	OccurredAt time.Time `gorm:"index;not null" json:"occurred_at"`
	CreatedAt  time.Time `gorm:"index" json:"created_at"`
	TicketID   *uint     `gorm:"index" json:"ticket_id"`
	Processed  bool      `gorm:"index;not null" json:"processed"`
	Attempts   int       `json:"attempts"`
	RetryAt    time.Time `gorm:"index" json:"-"`
	LastError  string    `gorm:"size:64" json:"last_error"`
}

func (MaintenanceEvent) TableName() string { return "ops_event" }

type MaintenanceStream struct {
	Key         string `gorm:"primaryKey;size:64"`
	TicketID    *uint
	LastEventAt time.Time
}

func (MaintenanceStream) TableName() string { return "ops_stream" }

type MaintenanceTicket struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	Title            string     `gorm:"size:300" json:"title"`
	NodeID           uint       `gorm:"index" json:"node_id"`
	StreamKey        string     `gorm:"size:64;index" json:"-"`
	Environment      string     `gorm:"size:32" json:"environment"`
	PluginID         string     `gorm:"size:128" json:"plugin_id"`
	InstanceID       string     `gorm:"size:128" json:"instance_id"`
	PluginVersion    string     `gorm:"size:128" json:"plugin_version"`
	AgentVersion     string     `gorm:"size:128" json:"agent_version"`
	ConfigVersion    string     `gorm:"size:128" json:"config_version"`
	ErrorCode        string     `gorm:"size:128" json:"error_code"`
	Severity         string     `gorm:"size:8" json:"severity"`
	Status           string     `gorm:"size:16;index" json:"status"`
	FirstFailedAt    time.Time  `gorm:"index" json:"first_failed_at"`
	LastEventAt      time.Time  `json:"last_event_at"`
	ClaimedBy        *uint      `json:"claimed_by"`
	ClaimedAt        *time.Time `json:"claimed_at"`
	EscalatedAt      *time.Time `json:"escalated_at"`
	RecoveredAt      *time.Time `json:"recovered_at"`
	ClosedAt         *time.Time `gorm:"index" json:"closed_at"`
	PreviousTicketID *uint      `json:"previous_ticket_id"`
	Occurrences      int        `json:"occurrences"`
	Episode          int        `json:"episode"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (MaintenanceTicket) TableName() string { return "ops_ticket" }

type MaintenanceRecord struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	TicketID  uint      `gorm:"index" json:"ticket_id"`
	ActorID   uint      `json:"actor_id"`
	Kind      string    `gorm:"size:40" json:"kind"`
	Note      string    `gorm:"type:text" json:"note"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (MaintenanceRecord) TableName() string { return "ops_record" }

type MaintenanceContact struct {
	UserID         uint   `json:"user_id"`
	Email          string `json:"email"`
	TelegramChatID string `json:"telegram_chat_id"`
}
type MaintenanceSettings struct {
	ID                         uint                 `gorm:"primaryKey" json:"id"`
	Enabled                    bool                 `json:"enabled"`
	OwnerID                    uint                 `json:"owner_id"`
	TechnicianIDs              []uint               `gorm:"serializer:json;type:text" json:"technician_ids"`
	Contacts                   []MaintenanceContact `gorm:"serializer:json;type:text" json:"contacts"`
	Revision                   int64                `json:"revision"`
	OwnerEmailVerifiedAt       *time.Time           `json:"owner_email_verified_at"`
	OwnerTelegramVerifiedAt    *time.Time           `json:"owner_telegram_verified_at"`
	EmailChallengeHash         string               `json:"-"`
	TelegramChallengeHash      string               `json:"-"`
	EmailChallengeExpiresAt    *time.Time           `json:"-"`
	TelegramChallengeExpiresAt *time.Time           `json:"-"`
	UpdatedAt                  time.Time            `json:"updated_at"`
}

func (MaintenanceSettings) TableName() string { return "ops_settings" }

type MaintenanceDelivery struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	DedupeKey        string     `gorm:"size:200;uniqueIndex;not null" json:"-"`
	TicketID         uint       `gorm:"index" json:"ticket_id"`
	SettingsRevision int64      `json:"-"`
	Channel          string     `gorm:"size:16" json:"channel"`
	RecipientID      uint       `json:"recipient_id"`
	Destination      string     `gorm:"size:320" json:"-"`
	Reason           string     `gorm:"size:64" json:"reason"`
	Body             string     `gorm:"type:text" json:"-"`
	Status           string     `gorm:"size:16;index" json:"status"`
	Attempts         int        `json:"attempts"`
	LastError        string     `gorm:"size:64" json:"last_error"`
	NextAttemptAt    time.Time  `gorm:"index" json:"next_attempt_at"`
	LeaseUntil       *time.Time `gorm:"index" json:"-"`
	LeaseOwner       string     `gorm:"size:64" json:"-"`
	SentAt           *time.Time `json:"sent_at"`
	CreatedAt        time.Time  `gorm:"index" json:"created_at"`
}

func (MaintenanceDelivery) TableName() string { return "ops_delivery" }

// MaintenanceNodeState tracks explicit exclusions without changing proxy node status semantics.
type MaintenanceNodeState struct {
	NodeID      uint      `gorm:"primaryKey" json:"node_id"`
	Maintenance bool      `json:"maintenance"`
	Disabled    bool      `json:"disabled"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (MaintenanceNodeState) TableName() string { return "ops_node_state" }
func MaintenanceModels() []any {
	return []any{&MaintenanceEvent{}, &MaintenanceStream{}, &MaintenanceTicket{}, &MaintenanceRecord{}, &MaintenanceSettings{}, &MaintenanceDelivery{}, &MaintenanceNodeState{}}
}
