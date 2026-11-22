package migrations_test

import (
	"time"

	"gorm.io/plugin/soft_delete"
)

// The kernel models 0042_drop_topology_and_plugin_leftovers dropped with
// their tables (the 2026-11 slimming audit): the topology deployments and
// the plugin lifecycle plans (PR 2b), the plugin WebUI assets (2c) and the
// remote module runtime's enrollments, certificates and runtime choices
// (2d), as the baseline created them (retiredKernelModels).

type retiredPluginWebUIAsset struct {
	ID           uint   `gorm:"primaryKey"`
	ReleaseID    uint   `gorm:"not null;uniqueIndex"`
	PluginID     string `gorm:"size:120;not null;index;uniqueIndex:ux_plugin_webui_asset_addr"`
	Version      string `gorm:"size:64;not null;uniqueIndex:ux_plugin_webui_asset_addr"`
	BundlePath   string `gorm:"size:240;not null"`
	BundleSHA256 string `gorm:"size:64;not null;index;uniqueIndex:ux_plugin_webui_asset_addr"`
	SizeBytes    int64  `gorm:"not null"`
	ContentType  string `gorm:"size:120;not null"`
	StorageKey   string `gorm:"size:360;not null;uniqueIndex"`
	Data         []byte
	CreatedAt    time.Time
}

func (retiredPluginWebUIAsset) TableName() string { return "v3_kernel_plugin_webui_asset" }

type retiredTopology struct {
	ID                uint   `gorm:"primaryKey"`
	Name              string `gorm:"size:160;not null;uniqueIndex:ux_topology_live_name,where:deleted = 0"`
	ServiceScope      string `gorm:"size:64;not null;index"`
	Description       string `gorm:"type:text"`
	ActiveRevisionID  *uint  `gorm:"index"`
	DeleteRequestedAt *time.Time
	Deleted           soft_delete.DeletedAt `gorm:"column:deleted;not null;default:0;softDelete:flag,DeletedAtField:DeletedAt"`
	DeletedAt         *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (retiredTopology) TableName() string { return "v3_kernel_topology" }

type retiredTopologyRevision struct {
	ID          uint   `gorm:"primaryKey"`
	TopologyID  uint   `gorm:"not null;uniqueIndex:ux_topology_revision"`
	Revision    int64  `gorm:"not null;uniqueIndex:ux_topology_revision"`
	State       string `gorm:"size:24;not null;index"`
	ContentHash string `gorm:"size:64;not null"`
	Message     string `gorm:"type:text"`
	CreatedBy   uint   `gorm:"not null"`
	CreatedAt   time.Time
}

func (retiredTopologyRevision) TableName() string { return "v3_kernel_topology_revision" }

type retiredTopologyVertex struct {
	ID         uint   `gorm:"primaryKey"`
	RevisionID uint   `gorm:"not null;index;uniqueIndex:ux_topology_vertex_key"`
	Key        string `gorm:"size:120;not null;uniqueIndex:ux_topology_vertex_key"`
	Kind       string `gorm:"size:48;not null"`
	NodeID     *uint  `gorm:"index"`
	PluginID   string `gorm:"size:120;index"`
	Role       string `gorm:"size:80"`
	ConfigJSON string `gorm:"type:text;not null"`
}

func (retiredTopologyVertex) TableName() string { return "v3_kernel_topology_vertex" }

type retiredTopologyEdge struct {
	ID         uint   `gorm:"primaryKey"`
	RevisionID uint   `gorm:"not null;index"`
	SourceKey  string `gorm:"size:120;not null"`
	TargetKey  string `gorm:"size:120;not null"`
	Protocol   string `gorm:"size:48;not null"`
	SecretID   string `gorm:"size:160"`
	ConfigJSON string `gorm:"type:text;not null"`
}

func (retiredTopologyEdge) TableName() string { return "v3_kernel_topology_edge" }

type retiredTopologyDeployment struct {
	ID                   uint `gorm:"primaryKey"`
	TopologyID           uint `gorm:"not null;index"`
	RevisionID           uint `gorm:"not null;index"`
	PreviousRevisionID   *uint
	RolloutGroup         string `gorm:"size:80"`
	State                string `gorm:"size:32;not null;index"`
	FailurePolicy        string `gorm:"size:32;not null"`
	LastError            string `gorm:"type:text"`
	Teardown             bool   `gorm:"not null;default:false"`
	HealthGateDeadlineAt *time.Time
	RollbackStartedAt    *time.Time
	RollbackCompletedAt  *time.Time
	CreatedBy            uint `gorm:"not null"`
	CreatedAt            time.Time
	UpdatedAt            time.Time
	CompletedAt          *time.Time
}

func (retiredTopologyDeployment) TableName() string { return "v3_kernel_topology_deployment" }

type retiredTopologyDeploymentStep struct {
	ID                           uint   `gorm:"primaryKey"`
	DeploymentID                 uint   `gorm:"not null;index;uniqueIndex:ux_topology_deployment_step"`
	VertexID                     uint   `gorm:"not null;uniqueIndex:ux_topology_deployment_step"`
	VertexKey                    string `gorm:"size:120;not null"`
	NodeID                       uint   `gorm:"not null;index"`
	PluginID                     string `gorm:"size:120;not null"`
	Role                         string `gorm:"size:80;not null"`
	TargetVersion                string `gorm:"size:64;not null"`
	ApplyOrder                   int    `gorm:"not null;index"`
	Removal                      bool   `gorm:"not null;default:false"`
	ApplyAction                  string `gorm:"size:32;not null"`
	ConfigJSON                   string `gorm:"type:text;not null;default:{}"`
	RollbackMode                 string `gorm:"size:32;not null"`
	RollbackConfigJSON           string `gorm:"type:text;not null;default:{}"`
	State                        string `gorm:"size:32;not null;index"`
	ConfigureOperationID         string `gorm:"size:64;index"`
	EnableOperationID            string `gorm:"size:64;index"`
	DisableOperationID           string `gorm:"size:64;index"`
	RollbackConfigureOperationID string `gorm:"size:64;index"`
	RollbackEnableOperationID    string `gorm:"size:64;index"`
	RollbackDisableOperationID   string `gorm:"size:64;index"`
	LastError                    string `gorm:"type:text"`
	CreatedAt                    time.Time
	UpdatedAt                    time.Time
}

func (retiredTopologyDeploymentStep) TableName() string { return "v3_kernel_topology_deployment_step" }

type retiredTopologyObservedState struct {
	ID               uint   `gorm:"primaryKey"`
	DeploymentID     uint   `gorm:"not null;index;uniqueIndex:ux_observed_deployment_node"`
	NodeID           uint   `gorm:"not null;uniqueIndex:ux_observed_deployment_node"`
	DesiredRevision  int64  `gorm:"not null"`
	ObservedRevision int64  `gorm:"not null"`
	State            string `gorm:"size:32;not null"`
	HealthJSON       string `gorm:"type:text"`
	LastError        string `gorm:"type:text"`
	UpdatedAt        time.Time
}

func (retiredTopologyObservedState) TableName() string { return "v3_kernel_topology_observed_state" }

type retiredPluginLifecyclePlan struct {
	ID              string `gorm:"primaryKey;size:64"`
	IdempotencyKey  string `gorm:"size:160;not null;uniqueIndex"`
	Target          string `gorm:"size:20;not null;index"`
	RootPluginID    string `gorm:"size:120;not null;index"`
	RootOperationID string `gorm:"size:64;not null;uniqueIndex"`
	State           string `gorm:"size:32;not null;index"`
	Outcome         string `gorm:"size:32"`
	LastError       string `gorm:"type:text"`
	CreatedAt       time.Time
	UpdatedAt       time.Time
	CompletedAt     *time.Time
}

func (retiredPluginLifecyclePlan) TableName() string { return "v3_kernel_plugin_lifecycle_plan" }

type retiredPluginLifecyclePlanStep struct {
	ID                      uint   `gorm:"primaryKey"`
	PlanID                  string `gorm:"size:64;not null;uniqueIndex:ux_plugin_lifecycle_plan_step"`
	Sequence                int    `gorm:"not null;uniqueIndex:ux_plugin_lifecycle_plan_step;index"`
	PluginID                string `gorm:"size:120;not null;index"`
	TargetVersion           string `gorm:"size:64;not null"`
	HadInstallation         bool   `gorm:"not null"`
	PreviousDesiredVersion  string `gorm:"size:64"`
	PreviousObservedVersion string `gorm:"size:64"`
	PreviousPreviousVersion string `gorm:"size:64"`
	PreviousState           string `gorm:"size:32"`
	PreviousEnabled         bool   `gorm:"not null"`
	PreviousDisabledAt      *time.Time
	PreviousLastError       string `gorm:"type:text"`
	Changed                 bool   `gorm:"not null"`
	ApplyState              string `gorm:"size:32;not null;default:pending;index"`
	RollbackState           string `gorm:"size:32;not null;default:pending;index"`
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (retiredPluginLifecyclePlanStep) TableName() string {
	return "v3_kernel_plugin_lifecycle_plan_step"
}

type retiredModuleEnrollment struct {
	ID             string    `gorm:"primaryKey;size:36"`
	PackageID      string    `gorm:"size:120;not null;index"`
	Cluster        string    `gorm:"size:63;not null"`
	CredentialHash string    `gorm:"size:64;not null;uniqueIndex"`
	Reusable       bool      `gorm:"not null"`
	ExpiresAt      time.Time `gorm:"not null"`
	UsedAt         *time.Time
	UseCount       int64 `gorm:"not null;default:0"`
	RevokedAt      *time.Time
	CreatedBy      uint
	CreatedAt      time.Time
}

func (retiredModuleEnrollment) TableName() string { return "v4_kernel_module_enrollment" }

type retiredModuleCertificate struct {
	Serial       string    `gorm:"primaryKey;size:40"`
	PackageID    string    `gorm:"size:120;not null;index"`
	Cluster      string    `gorm:"size:63;not null"`
	EnrollmentID string    `gorm:"size:36;not null;index"`
	IssuerKeyID  string    `gorm:"size:64;not null"`
	NotAfter     time.Time `gorm:"not null;index"`
	RevokedAt    *time.Time
	CreatedAt    time.Time
}

func (retiredModuleCertificate) TableName() string { return "v4_kernel_module_certificate" }

type retiredPluginRuntime struct {
	PluginID  string `gorm:"primaryKey;size:120"`
	Runtime   string `gorm:"size:16;not null"`
	UpdatedBy uint
	UpdatedAt time.Time
}

func (retiredPluginRuntime) TableName() string { return "v4_kernel_plugin_runtime" }

// baselineKernelOperationLinks are the v3_kernel_operation columns that
// 0042_drop_topology_and_plugin_leftovers dropped with model.KernelOperation's
// fields: the links to a topology deployment and to a lifecycle plan.
type baselineKernelOperationLinks struct {
	TopologyDeploymentID  *uint  `gorm:"index"`
	TopologyStepID        *uint  `gorm:"index"`
	TopologyRevision      int64  `gorm:"not null;default:0"`
	LifecyclePlanID       string `gorm:"size:64;index"`
	LifecyclePlanStepID   uint   `gorm:"index"`
	LifecyclePlanPhase    string `gorm:"size:16;index"`
	LifecyclePlanSequence int    `gorm:"not null;default:0;index"`
}

func (baselineKernelOperationLinks) TableName() string { return "v3_kernel_operation" }
