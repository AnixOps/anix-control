package model

import (
	"time"
)

// ServiceScope is an authorization namespace. A user may belong to multiple
// groups in each scope without leaking quota or grants into another service.
type ServiceScope struct {
	ID          string    `gorm:"primaryKey;size:64" json:"id"`
	Name        string    `gorm:"size:120;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	PluginID    string    `gorm:"size:120;index" json:"plugin_id"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (ServiceScope) TableName() string { return "v3_kernel_service_scope" }

type AccessGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	ScopeID     string    `gorm:"size:64;not null;uniqueIndex:ux_access_group_scope_name" json:"scope_id"`
	Name        string    `gorm:"size:120;not null;uniqueIndex:ux_access_group_scope_name" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Enabled     bool      `gorm:"not null" json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AccessGroup) TableName() string { return "v3_kernel_access_group" }

type AccessGroupUser struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	GroupID   uint      `gorm:"not null;uniqueIndex:ux_access_group_user" json:"group_id"`
	UserID    uint      `gorm:"not null;uniqueIndex:ux_access_group_user;index" json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (AccessGroupUser) TableName() string { return "v3_kernel_access_group_user" }

type AccessGroupPlan struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	GroupID   uint      `gorm:"not null;uniqueIndex:ux_access_group_plan" json:"group_id"`
	PlanID    uint      `gorm:"not null;uniqueIndex:ux_access_group_plan;index" json:"plan_id"`
	CreatedAt time.Time `json:"created_at"`
}

func (AccessGroupPlan) TableName() string { return "v3_kernel_access_group_plan" }

type ResourceGrant struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	GroupID      uint      `gorm:"not null;index;uniqueIndex:ux_resource_grant" json:"group_id"`
	ResourceType string    `gorm:"size:64;not null;uniqueIndex:ux_resource_grant" json:"resource_type"`
	ResourceID   string    `gorm:"size:160;not null;uniqueIndex:ux_resource_grant" json:"resource_id"`
	Permissions  string    `gorm:"type:text;not null" json:"permissions"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (ResourceGrant) TableName() string { return "v3_kernel_resource_grant" }

type QuotaPolicy struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	GroupID    uint      `gorm:"not null;index;uniqueIndex:ux_quota_group_key" json:"group_id"`
	Key        string    `gorm:"size:80;not null;uniqueIndex:ux_quota_group_key" json:"key"`
	PolicyJSON string    `gorm:"type:text;not null" json:"policy"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func (QuotaPolicy) TableName() string { return "v3_kernel_quota_policy" }

type Plugin struct {
	ID          string    `gorm:"primaryKey;size:120" json:"id"`
	Name        string    `gorm:"size:160;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Publisher   string    `gorm:"size:120;not null" json:"publisher"`
	Official    bool      `gorm:"not null;default:false" json:"official"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Plugin) TableName() string { return "v3_kernel_plugin" }

type PluginRelease struct {
	ID                   uint      `gorm:"primaryKey" json:"id"`
	PluginID             string    `gorm:"size:120;not null;uniqueIndex:ux_plugin_release" json:"plugin_id"`
	Version              string    `gorm:"size:64;not null;uniqueIndex:ux_plugin_release" json:"version"`
	APIVersion           string    `gorm:"size:32;not null" json:"api_version"`
	ManifestJSON         string    `gorm:"type:text;not null" json:"manifest"`
	ArtifactSHA256       string    `gorm:"size:64;not null" json:"artifact_sha256"`
	Signature            string    `gorm:"type:text;not null" json:"signature"`
	TrustRootFingerprint string    `gorm:"size:64;index" json:"trust_root_fingerprint"`
	TrustRootKeyID       string    `gorm:"size:32;index" json:"trust_root_key_id"`
	PublishedAt          time.Time `json:"published_at"`
	CreatedAt            time.Time `json:"created_at"`
}

func (PluginRelease) TableName() string { return "v3_kernel_plugin_release" }

type PluginTrustRoot struct {
	Fingerprint string     `gorm:"primaryKey;size:64" json:"fingerprint"`
	KeyID       string     `gorm:"size:32;not null;index" json:"key_id"`
	PublicKey   string     `gorm:"type:text;not null" json:"public_key"`
	Publisher   string     `gorm:"size:120;not null;index" json:"publisher"`
	Active      bool       `gorm:"not null;default:true;index" json:"active"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
	RetiredAt   *time.Time `json:"retired_at"`
}

func (PluginTrustRoot) TableName() string { return "v3_kernel_plugin_trust_root" }

type PluginArtifact struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	ReleaseID      uint      `gorm:"not null;uniqueIndex" json:"release_id"`
	PluginID       string    `gorm:"size:120;not null;index;uniqueIndex:ux_plugin_artifact_key" json:"plugin_id"`
	Version        string    `gorm:"size:64;not null;uniqueIndex:ux_plugin_artifact_key" json:"version"`
	ArtifactSHA256 string    `gorm:"size:64;not null;uniqueIndex:ux_plugin_artifact_key" json:"artifact_sha256"`
	SizeBytes      int64     `gorm:"not null" json:"size_bytes"`
	StorageKey     string    `gorm:"size:320;not null;uniqueIndex" json:"storage_key"`
	Data           []byte    `json:"-"`
	CreatedAt      time.Time `json:"created_at"`
}

func (PluginArtifact) TableName() string { return "v3_kernel_plugin_artifact" }

type PluginInstallation struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	PluginID            string     `gorm:"size:120;not null;uniqueIndex:ux_plugin_install_target" json:"plugin_id"`
	Target              string     `gorm:"size:20;not null;uniqueIndex:ux_plugin_install_target" json:"target"`
	DesiredVersion      string     `gorm:"size:64;not null" json:"desired_version"`
	ObservedVersion     string     `gorm:"size:64" json:"observed_version"`
	PreviousVersion     string     `gorm:"size:64" json:"previous_version"`
	State               string     `gorm:"size:32;not null;index" json:"state"`
	Enabled             bool       `gorm:"not null;default:false" json:"enabled"`
	LifecycleGeneration int64      `gorm:"not null;default:0" json:"lifecycle_generation"`
	ConfigRevision      int64      `gorm:"not null;default:0" json:"config_revision"`
	LastError           string     `gorm:"type:text" json:"last_error"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	DisabledAt          *time.Time `json:"disabled_at"`
}

func (PluginInstallation) TableName() string { return "v3_kernel_plugin_installation" }

// PluginTargetLock serializes dependency/conflict validation and installation
// intent changes for one runtime target across concurrent Control processes.
type PluginTargetLock struct {
	Target    string    `gorm:"primaryKey;size:20" json:"target"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (PluginTargetLock) TableName() string { return "v3_kernel_plugin_target_lock" }

// PluginConfiguration is the kernel-owned, revisioned configuration document
// for one installation. Plugin packages receive a copy through an operation;
// they never write the shared configuration table directly.
type PluginConfiguration struct {
	InstallationID uint      `gorm:"primaryKey" json:"installation_id"`
	Revision       int64     `gorm:"not null;default:0" json:"revision"`
	ConfigJSON     string    `gorm:"type:text;not null;default:{}" json:"config"`
	ConfigHash     string    `gorm:"size:64;not null" json:"config_hash"`
	UpdatedBy      uint      `gorm:"not null" json:"updated_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (PluginConfiguration) TableName() string { return "v3_kernel_plugin_configuration" }

type NodeServiceAssignment struct {
	ID                    uint      `gorm:"primaryKey" json:"id"`
	NodeID                uint      `gorm:"not null;index;uniqueIndex:ux_node_service_assignment" json:"node_id"`
	ServiceScope          string    `gorm:"size:64;not null;uniqueIndex:ux_node_service_assignment" json:"service_scope"`
	PluginID              string    `gorm:"size:120;not null;uniqueIndex:ux_node_service_assignment" json:"plugin_id"`
	Role                  string    `gorm:"size:80;not null;uniqueIndex:ux_node_service_assignment" json:"role"`
	DesiredVersion        string    `gorm:"size:64" json:"desired_version"`
	DesiredConfigRevision int64     `gorm:"not null;default:0" json:"desired_config_revision"`
	Enabled               bool      `gorm:"not null" json:"enabled"`
	LifecycleGeneration   int64     `gorm:"not null;default:0" json:"lifecycle_generation"`
	DeletePending         bool      `gorm:"not null;default:false;index" json:"delete_pending"`
	RolloutGroup          string    `gorm:"size:80;index" json:"rollout_group"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

func (NodeServiceAssignment) TableName() string { return "v3_kernel_node_service_assignment" }

// NodePluginLifecycle is the aggregate desired/observed boundary for all
// roles of one plugin on one physical node. The row is also the transaction
// lock that serializes assignment, configuration, retry and deletion intent.
type NodePluginLifecycle struct {
	NodeID                uint       `gorm:"primaryKey" json:"node_id"`
	PluginID              string     `gorm:"primaryKey;size:120" json:"plugin_id"`
	DesiredGeneration     int64      `gorm:"not null;default:0" json:"desired_generation"`
	RetryEpoch            int        `gorm:"not null;default:0" json:"retry_epoch"`
	RetryAfter            *time.Time `gorm:"index" json:"retry_after"`
	RetryExhausted        bool       `gorm:"not null;default:false" json:"retry_exhausted"`
	DesiredVersion        string     `gorm:"size:64" json:"desired_version"`
	DesiredConfigRevision int64      `gorm:"not null;default:0" json:"desired_config_revision"`
	DesiredEnabled        bool       `gorm:"not null;default:false" json:"desired_enabled"`
	ActiveVersion         string     `gorm:"size:64" json:"active_version"`
	ActiveEnabled         bool       `gorm:"not null;default:false" json:"active_enabled"`
	ActiveRevision        int64      `gorm:"not null;default:0" json:"active_revision"`
	LastError             string     `gorm:"type:text" json:"last_error"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (NodePluginLifecycle) TableName() string { return "v3_kernel_node_plugin_lifecycle" }

type KernelOperation struct {
	ID              string `gorm:"primaryKey;size:64" json:"id"`
	IdempotencyKey  string `gorm:"size:160;not null;uniqueIndex" json:"idempotency_key"`
	EnvelopeVersion string `gorm:"size:32;not null;default:anixops.operation/v1" json:"envelope_version"`
	// SessionID is written by the dispatcher from the active Agent connection.
	// It must never be trusted from an HTTP request.
	SessionID            string     `gorm:"size:120;index" json:"session_id"`
	NodeID               *uint      `gorm:"index" json:"node_id"`
	PluginID             string     `gorm:"size:120;index" json:"plugin_id"`
	TargetVersion        string     `gorm:"size:64" json:"target_version"`
	Kind                 string     `gorm:"size:80;not null" json:"kind"`
	Revision             int64      `gorm:"not null" json:"revision"`
	ConfigJSON           string     `gorm:"type:text;not null;default:{}" json:"config"`
	ConfigHash           string     `gorm:"size:64" json:"config_hash"`
	DependsOnOperationID string     `gorm:"size:64;index" json:"depends_on_operation_id,omitempty"`
	State                string     `gorm:"size:32;not null;index" json:"state"`
	DeadlineAt           *time.Time `json:"deadline_at"`
	DispatchedAt         *time.Time `json:"dispatched_at"`
	AcknowledgedAt       *time.Time `json:"acknowledged_at"`
	ObservedAt           *time.Time `json:"observed_at"`
	CancelAt             *time.Time `json:"cancel_at"`
	CancelDispatchedAt   *time.Time `json:"cancel_dispatched_at"`
	ClaimedBy            string     `gorm:"size:96;index" json:"claimed_by"`
	LeaseExpiresAt       *time.Time `gorm:"index" json:"lease_expires_at"`
	Attempt              int        `gorm:"not null;default:0" json:"attempt"`
	ResultJSON           string     `gorm:"type:text" json:"result"`
	LastError            string     `gorm:"type:text" json:"last_error"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (KernelOperation) TableName() string { return "v3_kernel_operation" }

// NodeOperationRevision is the durable desired/observed revision cursor for a
// physical node. It is intentionally separate from any plugin-specific state.
type NodeOperationRevision struct {
	NodeID           uint      `gorm:"primaryKey" json:"node_id"`
	DesiredRevision  int64     `gorm:"not null;default:0" json:"desired_revision"`
	ObservedRevision int64     `gorm:"not null;default:0" json:"observed_revision"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (NodeOperationRevision) TableName() string { return "v3_kernel_node_operation_revision" }

// PluginTelemetryState stores the latest scalar snapshot received from one
// enabled Agent plugin. Metrics are namespaced by plugin at the transport
// boundary and persisted as JSON so the kernel does not need to know every
// plugin-specific field in advance.
type PluginTelemetryState struct {
	NodeID      uint      `gorm:"primaryKey" json:"node_id"`
	PluginID    string    `gorm:"primaryKey;size:120" json:"plugin_id"`
	MetricsJSON string    `gorm:"type:text;not null" json:"metrics"`
	ObservedAt  time.Time `gorm:"not null;index" json:"observed_at"`
	ReceivedAt  time.Time `gorm:"not null;index" json:"received_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (PluginTelemetryState) TableName() string { return "v3_kernel_plugin_telemetry_state" }

// NodePluginObservedState is a bounded, non-secret snapshot emitted by an
// Agent plugin. It is intentionally separate from generic telemetry because
// topology promotion needs a cryptographically bound config/ruleset view,
// rather than a plugin-defined metric bag.
type NodePluginObservedState struct {
	NodeID           uint      `gorm:"primaryKey" json:"node_id"`
	PluginID         string    `gorm:"primaryKey;size:120" json:"plugin_id"`
	Version          string    `gorm:"size:64;not null" json:"version"`
	DesiredRevision  int64     `gorm:"not null" json:"desired_revision"`
	ObservedRevision int64     `gorm:"not null" json:"observed_revision"`
	ConfigHash       string    `gorm:"size:64;not null" json:"config_hash"`
	Health           string    `gorm:"size:32;not null;index" json:"health"`
	RulesetSHA256    string    `gorm:"size:64;not null" json:"ruleset_sha256"`
	CountersJSON     string    `gorm:"type:text;not null;default:[]" json:"rule_counters"`
	ObservedAt       time.Time `gorm:"not null;index" json:"observed_at"`
	ReceivedAt       time.Time `gorm:"not null;index" json:"received_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (NodePluginObservedState) TableName() string { return "v3_kernel_node_plugin_observed_state" }

func KernelModels() []any {
	return []any{
		&ServiceScope{}, &AccessGroup{}, &AccessGroupUser{}, &AccessGroupPlan{},
		&ResourceGrant{}, &QuotaPolicy{}, &Plugin{}, &PluginRelease{},
		&PluginTrustRoot{}, &PluginArtifact{}, &PluginInstallation{},
		&PluginTargetLock{}, &PluginConfiguration{}, &NodeServiceAssignment{}, &NodePluginLifecycle{}, &KernelOperation{},
		&NodeOperationRevision{}, &PluginTelemetryState{}, &NodePluginObservedState{},
		&PackageMigrationRun{}, &PackageValidationResult{}, &PackageRouteGeneration{}, &PackageBackupReference{}, &PackageRolloutLock{},
		&PackageStorage{}, &ServiceCA{},
		&IdentityRevocation{}, &IdentitySessionRevocation{}, &IdentityAccountLink{}, &IdentityAuthority{},
		&IdentityTokenKey{}, &SubscriberRequest{}, &SubscriberChange{}, &UserActivity{}, &SettingsRequest{},
		&ProtocolSecret{}, &NodeSecretSplit{},
		&KernelNodeOperation{}, &KernelNodeOperationTarget{}, &KernelNodeOperationEvent{},
		&AgentEnrollment{}, &AgentCertificate{}, &ForwardLinkCA{}, &ForwardLinkCertificate{}, &AgentReportBatch{}, &KernelNodeDesiredConfig{}, &KernelNodeConfigStatus{},
		&AgentTransport{}, &PackageReportState{},
		&KernelForwardAllocation{}, &KernelForwardNode{}, &KernelForwardNodeState{},
		&KernelForwardNodeReport{}, &KernelForwardCounter{}, &KernelForwardTraffic{}, &KernelForwardRequest{},
		&KernelForwardPlan{}, &AgentUpgradeCampaign{}, &AgentUpgradeNode{},
		&KernelForwardDNSProvider{}, &KernelForwardDNSBinding{}, &KernelForwardDNSNode{},
		&AdminAPIToken{},
		&KernelAlert{},
		&NodeGroup{},
	}
}
