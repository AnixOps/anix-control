package model

import "time"

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

type PluginWebUIAsset struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ReleaseID    uint      `gorm:"not null;uniqueIndex" json:"release_id"`
	PluginID     string    `gorm:"size:120;not null;index;uniqueIndex:ux_plugin_webui_asset_addr" json:"plugin_id"`
	Version      string    `gorm:"size:64;not null;uniqueIndex:ux_plugin_webui_asset_addr" json:"version"`
	BundlePath   string    `gorm:"size:240;not null" json:"bundle_path"`
	BundleSHA256 string    `gorm:"size:64;not null;index;uniqueIndex:ux_plugin_webui_asset_addr" json:"bundle_sha256"`
	SizeBytes    int64     `gorm:"not null" json:"size_bytes"`
	ContentType  string    `gorm:"size:120;not null" json:"content_type"`
	StorageKey   string    `gorm:"size:360;not null;uniqueIndex" json:"storage_key"`
	Data         []byte    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

func (PluginWebUIAsset) TableName() string { return "v3_kernel_plugin_webui_asset" }

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

type Topology struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"size:160;not null;uniqueIndex" json:"name"`
	ServiceScope     string    `gorm:"size:64;not null;index" json:"service_scope"`
	Description      string    `gorm:"type:text" json:"description"`
	ActiveRevisionID *uint     `gorm:"index" json:"active_revision_id"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (Topology) TableName() string { return "v3_kernel_topology" }

type TopologyRevision struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	TopologyID  uint      `gorm:"not null;uniqueIndex:ux_topology_revision" json:"topology_id"`
	Revision    int64     `gorm:"not null;uniqueIndex:ux_topology_revision" json:"revision"`
	State       string    `gorm:"size:24;not null;index" json:"state"`
	ContentHash string    `gorm:"size:64;not null" json:"content_hash"`
	Message     string    `gorm:"type:text" json:"message"`
	CreatedBy   uint      `gorm:"not null" json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
}

func (TopologyRevision) TableName() string { return "v3_kernel_topology_revision" }

type TopologyVertex struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	RevisionID uint   `gorm:"not null;index;uniqueIndex:ux_topology_vertex_key" json:"revision_id"`
	Key        string `gorm:"size:120;not null;uniqueIndex:ux_topology_vertex_key" json:"key"`
	Kind       string `gorm:"size:48;not null" json:"kind"`
	NodeID     *uint  `gorm:"index" json:"node_id"`
	PluginID   string `gorm:"size:120;index" json:"plugin_id"`
	Role       string `gorm:"size:80" json:"role"`
	ConfigJSON string `gorm:"type:text;not null" json:"config"`
}

func (TopologyVertex) TableName() string { return "v3_kernel_topology_vertex" }

type TopologyEdge struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	RevisionID uint   `gorm:"not null;index" json:"revision_id"`
	SourceKey  string `gorm:"size:120;not null" json:"source_key"`
	TargetKey  string `gorm:"size:120;not null" json:"target_key"`
	Protocol   string `gorm:"size:48;not null" json:"protocol"`
	SecretID   string `gorm:"size:160" json:"secret_id"`
	ConfigJSON string `gorm:"type:text;not null" json:"config"`
}

func (TopologyEdge) TableName() string { return "v3_kernel_topology_edge" }

type TopologyDeployment struct {
	ID                 uint   `gorm:"primaryKey" json:"id"`
	TopologyID         uint   `gorm:"not null;index" json:"topology_id"`
	RevisionID         uint   `gorm:"not null;index" json:"revision_id"`
	PreviousRevisionID *uint  `json:"previous_revision_id"`
	RolloutGroup       string `gorm:"size:80" json:"rollout_group"`
	State              string `gorm:"size:32;not null;index" json:"state"`
	FailurePolicy      string `gorm:"size:32;not null" json:"failure_policy"`
	LastError          string `gorm:"type:text" json:"last_error"`
	// HealthGateDeadlineAt is a durable grace window for a terminal Agent
	// operation to be followed by its independent runtime observation.
	HealthGateDeadlineAt *time.Time `json:"health_gate_deadline_at"`
	RollbackStartedAt    *time.Time `json:"rollback_started_at"`
	RollbackCompletedAt  *time.Time `json:"rollback_completed_at"`
	CreatedBy            uint       `gorm:"not null" json:"created_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	CompletedAt          *time.Time `json:"completed_at"`
}

func (TopologyDeployment) TableName() string { return "v3_kernel_topology_deployment" }

// TopologyDeploymentStep is the durable, per-vertex execution plan for one
// immutable topology revision. It stores operation identities rather than an
// in-memory work queue so a Control restart can resume or compensate a
// partially applied deployment deterministically.
//
// ApplyAction is either configure_enable or disable. RollbackMode is restore
// (configure the previous active revision and enable it) or disable (there was
// no previous vertex to restore). States are advanced only by the topology
// deployment executor.
type TopologyDeploymentStep struct {
	ID                           uint      `gorm:"primaryKey" json:"id"`
	DeploymentID                 uint      `gorm:"not null;index;uniqueIndex:ux_topology_deployment_step" json:"deployment_id"`
	VertexID                     uint      `gorm:"not null;uniqueIndex:ux_topology_deployment_step" json:"vertex_id"`
	VertexKey                    string    `gorm:"size:120;not null" json:"vertex_key"`
	NodeID                       uint      `gorm:"not null;index" json:"node_id"`
	PluginID                     string    `gorm:"size:120;not null" json:"plugin_id"`
	Role                         string    `gorm:"size:80;not null" json:"role"`
	TargetVersion                string    `gorm:"size:64;not null" json:"target_version"`
	ApplyOrder                   int       `gorm:"not null;index" json:"apply_order"`
	Removal                      bool      `gorm:"not null;default:false" json:"removal"`
	ApplyAction                  string    `gorm:"size:32;not null" json:"apply_action"`
	ConfigJSON                   string    `gorm:"type:text;not null;default:{}" json:"config"`
	RollbackMode                 string    `gorm:"size:32;not null" json:"rollback_mode"`
	RollbackConfigJSON           string    `gorm:"type:text;not null;default:{}" json:"rollback_config"`
	State                        string    `gorm:"size:32;not null;index" json:"state"`
	ConfigureOperationID         string    `gorm:"size:64;index" json:"configure_operation_id"`
	EnableOperationID            string    `gorm:"size:64;index" json:"enable_operation_id"`
	DisableOperationID           string    `gorm:"size:64;index" json:"disable_operation_id"`
	RollbackConfigureOperationID string    `gorm:"size:64;index" json:"rollback_configure_operation_id"`
	RollbackEnableOperationID    string    `gorm:"size:64;index" json:"rollback_enable_operation_id"`
	RollbackDisableOperationID   string    `gorm:"size:64;index" json:"rollback_disable_operation_id"`
	LastError                    string    `gorm:"type:text" json:"last_error"`
	CreatedAt                    time.Time `json:"created_at"`
	UpdatedAt                    time.Time `json:"updated_at"`
}

func (TopologyDeploymentStep) TableName() string { return "v3_kernel_topology_deployment_step" }

type TopologyObservedState struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	DeploymentID     uint      `gorm:"not null;index;uniqueIndex:ux_observed_deployment_node" json:"deployment_id"`
	NodeID           uint      `gorm:"not null;uniqueIndex:ux_observed_deployment_node" json:"node_id"`
	DesiredRevision  int64     `gorm:"not null" json:"desired_revision"`
	ObservedRevision int64     `gorm:"not null" json:"observed_revision"`
	State            string    `gorm:"size:32;not null" json:"state"`
	HealthJSON       string    `gorm:"type:text" json:"health"`
	LastError        string    `gorm:"type:text" json:"last_error"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (TopologyObservedState) TableName() string { return "v3_kernel_topology_observed_state" }

type KernelOperation struct {
	ID              string `gorm:"primaryKey;size:64" json:"id"`
	IdempotencyKey  string `gorm:"size:160;not null;uniqueIndex" json:"idempotency_key"`
	EnvelopeVersion string `gorm:"size:32;not null;default:anixops.operation/v1" json:"envelope_version"`
	// SessionID is written by the dispatcher from the active Agent connection.
	// It must never be trusted from an HTTP request.
	SessionID            string `gorm:"size:120;index" json:"session_id"`
	NodeID               *uint  `gorm:"index" json:"node_id"`
	PluginID             string `gorm:"size:120;index" json:"plugin_id"`
	TargetVersion        string `gorm:"size:64" json:"target_version"`
	TopologyDeploymentID *uint  `gorm:"index" json:"topology_deployment_id"`
	TopologyStepID       *uint  `gorm:"index" json:"topology_step_id"`
	TopologyRevision     int64  `gorm:"not null;default:0" json:"topology_revision"`
	Kind                 string `gorm:"size:80;not null" json:"kind"`
	Revision             int64  `gorm:"not null" json:"revision"`
	ConfigJSON           string `gorm:"type:text;not null;default:{}" json:"config"`
	ConfigHash           string `gorm:"size:64" json:"config_hash"`
	DependsOnOperationID string `gorm:"size:64;index" json:"depends_on_operation_id,omitempty"`
	// LifecyclePlanID and the following fields group Control-target package
	// operations that must be executed as one dependency-aware lifecycle plan.
	// They are intentionally empty for legacy and Agent operations.
	LifecyclePlanID       string     `gorm:"size:64;index" json:"lifecycle_plan_id,omitempty"`
	LifecyclePlanStepID   uint       `gorm:"index" json:"lifecycle_plan_step_id,omitempty"`
	LifecyclePlanPhase    string     `gorm:"size:16;index" json:"lifecycle_plan_phase,omitempty"`
	LifecyclePlanSequence int        `gorm:"not null;default:0;index" json:"lifecycle_plan_sequence"`
	State                 string     `gorm:"size:32;not null;index" json:"state"`
	DeadlineAt            *time.Time `json:"deadline_at"`
	DispatchedAt          *time.Time `json:"dispatched_at"`
	AcknowledgedAt        *time.Time `json:"acknowledged_at"`
	ObservedAt            *time.Time `json:"observed_at"`
	CancelAt              *time.Time `json:"cancel_at"`
	CancelDispatchedAt    *time.Time `json:"cancel_dispatched_at"`
	ClaimedBy             string     `gorm:"size:96;index" json:"claimed_by"`
	LeaseExpiresAt        *time.Time `gorm:"index" json:"lease_expires_at"`
	Attempt               int        `gorm:"not null;default:0" json:"attempt"`
	ResultJSON            string     `gorm:"type:text" json:"result"`
	LastError             string     `gorm:"type:text" json:"last_error"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

func (KernelOperation) TableName() string { return "v3_kernel_operation" }

// PluginLifecyclePlan is the durable Control-side transaction boundary for a
// dependency closure. Individual KernelOperations remain independently
// observable and cancellable, while the plan supplies dependency ordering and
// reverse rollback after a failed or cancelled apply phase.
type PluginLifecyclePlan struct {
	ID              string     `gorm:"primaryKey;size:64" json:"id"`
	IdempotencyKey  string     `gorm:"size:160;not null;uniqueIndex" json:"idempotency_key"`
	Target          string     `gorm:"size:20;not null;index" json:"target"`
	RootPluginID    string     `gorm:"size:120;not null;index" json:"root_plugin_id"`
	RootOperationID string     `gorm:"size:64;not null;uniqueIndex" json:"root_operation_id"`
	State           string     `gorm:"size:32;not null;index" json:"state"`
	Outcome         string     `gorm:"size:32" json:"outcome"`
	LastError       string     `gorm:"type:text" json:"last_error"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	CompletedAt     *time.Time `json:"completed_at"`
}

func (PluginLifecyclePlan) TableName() string { return "v3_kernel_plugin_lifecycle_plan" }

// PluginLifecyclePlanStep stores the old desired state separately from the
// runtime operation. That lets a restarted worker restore intent precisely
// after a dependency graph failure without deleting package data.
type PluginLifecyclePlanStep struct {
	ID                      uint       `gorm:"primaryKey" json:"id"`
	PlanID                  string     `gorm:"size:64;not null;uniqueIndex:ux_plugin_lifecycle_plan_step" json:"plan_id"`
	Sequence                int        `gorm:"not null;uniqueIndex:ux_plugin_lifecycle_plan_step;index" json:"sequence"`
	PluginID                string     `gorm:"size:120;not null;index" json:"plugin_id"`
	TargetVersion           string     `gorm:"size:64;not null" json:"target_version"`
	HadInstallation         bool       `gorm:"not null" json:"had_installation"`
	PreviousDesiredVersion  string     `gorm:"size:64" json:"previous_desired_version"`
	PreviousObservedVersion string     `gorm:"size:64" json:"previous_observed_version"`
	PreviousPreviousVersion string     `gorm:"size:64" json:"previous_previous_version"`
	PreviousState           string     `gorm:"size:32" json:"previous_state"`
	PreviousEnabled         bool       `gorm:"not null" json:"previous_enabled"`
	PreviousDisabledAt      *time.Time `json:"previous_disabled_at"`
	PreviousLastError       string     `gorm:"type:text" json:"previous_last_error"`
	Changed                 bool       `gorm:"not null" json:"changed"`
	ApplyState              string     `gorm:"size:32;not null;default:pending;index" json:"apply_state"`
	RollbackState           string     `gorm:"size:32;not null;default:pending;index" json:"rollback_state"`
	CreatedAt               time.Time  `json:"created_at"`
	UpdatedAt               time.Time  `json:"updated_at"`
}

func (PluginLifecyclePlanStep) TableName() string {
	return "v3_kernel_plugin_lifecycle_plan_step"
}

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
		&PluginTrustRoot{}, &PluginArtifact{}, &PluginWebUIAsset{}, &PluginInstallation{},
		&PluginTargetLock{}, &PluginConfiguration{}, &NodeServiceAssignment{}, &NodePluginLifecycle{}, &Topology{},
		&TopologyRevision{}, &TopologyVertex{}, &TopologyEdge{},
		&TopologyDeployment{}, &TopologyDeploymentStep{}, &TopologyObservedState{}, &KernelOperation{},
		&PluginLifecyclePlan{}, &PluginLifecyclePlanStep{},
		&NodeOperationRevision{}, &PluginTelemetryState{}, &NodePluginObservedState{},
		&PackageMigrationRun{}, &PackageValidationResult{}, &PackageRouteGeneration{}, &PackageBackupReference{}, &PackageRolloutLock{},
	}
}
