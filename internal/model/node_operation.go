package model

import "time"

// KernelNodeOperation is one KernelNodeOps operation
// (docs/architecture/node-ops-service.md section 3.4): the ledger row a
// package's SubmitOperation records, keyed by its request id, and the state
// the kernel moves it through. It never holds a credential: the operation is
// the canonical OperationSpec (sealed handles normalized), and results and
// errors are scrubbed before they are stored.
type KernelNodeOperation struct {
	// ID orders operations (newest first in listings); OperationID names them.
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"-"`
	OperationID string `gorm:"size:64;not null;uniqueIndex" json:"operation_id"`
	// RequestID is unique: a repeat with the same Digest and owner answers
	// this operation, anything else is refused.
	RequestID string `gorm:"size:128;not null;uniqueIndex" json:"request_id"`
	// Digest is the SHA-256 (hex) of the kind and the canonical operation.
	Digest string `gorm:"size:64;not null" json:"digest"`
	// PackageID owns the operation: only it reads, watches and cancels it.
	PackageID         string `gorm:"size:120;not null;index:idx_kernel_node_operation_owner,priority:1" json:"package_id"`
	PackageGeneration uint64 `gorm:"not null;default:0" json:"package_generation"`
	// SubmittedBy is "<package>@<generation>", or "kernel:<route id>" for
	// the kernel's own handlers.
	SubmittedBy string `gorm:"size:200;not null" json:"submitted_by"`
	// Family is forward, nodeconfig, diagnose, agents or credentials; Kind
	// the operation's stable name, for example "forward.apply".
	Family string `gorm:"size:32;not null;index" json:"family"`
	Kind   string `gorm:"size:64;not null;index" json:"kind"`
	// ResourceKey names what the operation changes ("forward:12",
	// "nodeconfig:proxy-3"); the kernel runs one operation per resource at a
	// time. Empty for operations that change no shared resource.
	ResourceKey string `gorm:"size:96;not null;default:'';index" json:"resource_key"`
	// State is pending, dispatching, running, or one of the terminal
	// states succeeded, failed, cancelled, timed_out and superseded, which
	// never change again.
	State        string `gorm:"size:16;not null;index;index:idx_kernel_node_operation_owner,priority:2" json:"state"`
	Channel      string `gorm:"size:32;not null;default:''" json:"channel"`
	Attempt      uint32 `gorm:"not null;default:0" json:"attempt"`
	NodeRevision uint64 `gorm:"not null;default:0" json:"node_revision"`
	// ParentOperationID is set on the operations a fan-out created; the
	// parent counts them in FanOut*.
	ParentOperationID string `gorm:"size:64;not null;default:'';index" json:"parent_operation_id"`
	FanOutTotal       uint32 `gorm:"not null;default:0" json:"fan_out_total"`
	FanOutSucceeded   uint32 `gorm:"not null;default:0" json:"fan_out_succeeded"`
	FanOutFailed      uint32 `gorm:"not null;default:0" json:"fan_out_failed"`
	FanOutPending     uint32 `gorm:"not null;default:0" json:"fan_out_pending"`
	// KernelOperationID links the durable Agent dispatch (v3_kernel_operation),
	// ForwardRuntimeJobID the forward runtime job (v2_forward_runtime_job).
	KernelOperationID   string `gorm:"size:64;not null;default:''" json:"kernel_operation_id"`
	ForwardRuntimeJobID uint64 `gorm:"not null;default:0" json:"forward_runtime_job_id"`
	// Operation is the canonical OperationSpec as protojson.
	Operation string `gorm:"type:text;not null" json:"operation"`
	Reason    string `gorm:"size:255;not null;default:''" json:"reason"`
	// Result is the OperationResult as protojson, scrubbed, at most 64 KiB.
	Result         string `gorm:"type:text;not null;default:''" json:"result"`
	ErrorCode      string `gorm:"size:40;not null;default:''" json:"error_code"`
	ErrorMessage   string `gorm:"type:text;not null;default:''" json:"error_message"`
	ErrorRetryable bool   `gorm:"not null;default:false" json:"error_retryable"`
	// Evidence is an outcome that arrived after the operation had ended (a
	// late result after its deadline): recorded, never applied.
	Evidence          string     `gorm:"type:text;not null;default:''" json:"evidence"`
	CancelRequestedAt *time.Time `json:"cancel_requested_at"`
	CancelReason      string     `gorm:"size:255;not null;default:''" json:"cancel_reason"`
	CreatedAt         time.Time  `gorm:"not null" json:"created_at"`
	AcceptedAt        *time.Time `json:"accepted_at"`
	FinishedAt        *time.Time `gorm:"index" json:"finished_at"`
	DeadlineAt        time.Time  `gorm:"not null;index" json:"deadline_at"`
	UpdatedAt         time.Time  `gorm:"not null" json:"updated_at"`
}

func (KernelNodeOperation) TableName() string { return "v4_kernel_node_operation" }

// KernelNodeOperationTarget is one node an operation acts on, so that the
// kernel can count each node's operations (its quota) and list the
// operations on a node.
type KernelNodeOperationTarget struct {
	ID          uint64 `gorm:"primaryKey;autoIncrement" json:"-"`
	OperationID string `gorm:"size:64;not null;index" json:"operation_id"`
	// NodeKind is proxy (v2_node), forward (v2_forward_node) or clean_agent
	// (v2_forward_clean_agent).
	NodeKind string `gorm:"size:16;not null;index:idx_kernel_node_operation_target_node,priority:1" json:"node_kind"`
	NodeID   uint64 `gorm:"not null;index:idx_kernel_node_operation_target_node,priority:2" json:"node_id"`
}

func (KernelNodeOperationTarget) TableName() string { return "v4_kernel_node_operation_target" }

// KernelNodeOperationEvent is the append-only change log WatchOperations
// follows: one row per state change, written in the change's transaction.
// ID is the cursor. Rows are kept 7 days; an older cursor resynchronizes.
type KernelNodeOperationEvent struct {
	ID          uint64    `gorm:"primaryKey;autoIncrement" json:"cursor"`
	OperationID string    `gorm:"size:64;not null;index" json:"operation_id"`
	PackageID   string    `gorm:"size:120;not null;index" json:"package_id"`
	Family      string    `gorm:"size:32;not null" json:"family"`
	State       string    `gorm:"size:16;not null" json:"state"`
	CreatedAt   time.Time `gorm:"not null;index" json:"created_at"`
}

func (KernelNodeOperationEvent) TableName() string { return "v4_kernel_node_operation_event" }

// KernelNodeOperationModels are the KernelNodeOps tables.
func KernelNodeOperationModels() []any {
	return []any{&KernelNodeOperation{}, &KernelNodeOperationTarget{}, &KernelNodeOperationEvent{}}
}
