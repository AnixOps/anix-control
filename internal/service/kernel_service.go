package service

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/AnixOps/anix-control/sdk/telemetry/systemdreport"
	"github.com/AnixOps/anix-control/v5/internal/builtinpkg"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/pluginhost"
	"github.com/google/uuid"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var defaultServiceScopes = []model.ServiceScope{
	{ID: "subscription", Name: "Subscription", PluginID: "subscription"},
	{ID: "proxy", Name: "Proxy", PluginID: "protocol-runtime"},
	{ID: "forward", Name: "Forward", PluginID: "forward"},
	{ID: "monitoring", Name: "Monitoring", PluginID: "machine-telemetry"},
}

var officialPluginCatalog = []model.Plugin{
	{ID: "subscription", Name: "Subscription", Publisher: "AnixOps", Official: true},
	{ID: "forward", Name: "Forward Compatibility", Publisher: "AnixOps", Official: true},
	{ID: "machine-telemetry", Name: "Machine Telemetry", Publisher: "AnixOps", Official: true},
	{ID: "wireguard", Name: "WireGuard", Publisher: "AnixOps", Official: true},
	{ID: "protocol-runtime", Name: "Protocol Runtime", Publisher: "AnixOps", Official: true},
}

var (
	ErrInvalidKernelOperationID    = errors.New("operation_id must be a UUID")
	ErrKernelOperationNotPending   = errors.New("operation is not cancellable")
	ErrPluginTrustRootRequired     = errors.New("official plugin trust root is required")
	ErrExtensionCatalogIntegrity   = errors.New("extension catalog integrity check failed")
	ErrPluginConfigurationConflict = errors.New("plugin configuration revision conflict")
	ErrPluginDependencyUnsatisfied = errors.New("plugin dependency is not enabled")
	ErrPluginConflict              = errors.New("plugin conflict is enabled")
	ErrPluginArtifactRequired      = errors.New("verified plugin artifact is required")
	ErrPluginArtifactImmutable     = errors.New("plugin artifact is immutable")
	ErrPluginRouteNotFound         = errors.New("plugin control route is not registered")
	ErrPluginRouteForbidden        = errors.New("plugin control route permission denied")
	ErrPackageMigrationImmutable   = errors.New("package migration run is immutable")
	ErrValidationPrecondition      = errors.New("package validation precondition failed")
	ErrCohortTransition            = errors.New("package cohort transition is invalid")
	ErrRollbackPrecondition        = errors.New("package rollback precondition failed")
)

const KernelOperationEnvelopeVersion = "anixops.operation/v1"
const pluginManifestAPIVersionV1 = "v1"
const pluginManifestAPIVersionV2 = "v2"
const pluginManifestAPIVersion = pluginManifestAPIVersionV1
const agentRuntimeAPIVersionV110 = "anixops.agent.sdk/v1.1.0"
const MaxPluginArtifactBytes = 64 << 20
const maxPluginControlEntrypointBytes = 128 << 20
const maxPluginEntrypointPlatforms = 2
const PluginAPIGrantResourceType = "plugin_api"
const pluginEntrypointIndexFormat = "anixops.package-entrypoints/v1"
const pluginControlEntrypointIndexPath = "bin/control-entrypoints.json"

type pluginEntrypointIndex struct {
	Entries []pluginPlatformEntrypoint `json:"entries"`
	Format  string                     `json:"format"`
}

type pluginPlatformEntrypoint struct {
	Architecture string `json:"architecture"`
	Path         string `json:"path"`
	SHA256       string `json:"sha256"`
}

// MigrateKernelModelsForTest creates the kernel tables with GORM's
// AutoMigrate (and the tables of later migrations that a node's desired
// configuration and the subscriptions read: the egress routes, the node
// entries and the subscription groups' protocol layouts; and the forward
// latency history the reports write) and seeds the kernel defaults, on a test's own database.
// Control prepares its schema with internal/database/migrations and seeds
// with SeedKernelDefaults.
func MigrateKernelModelsForTest(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if err := db.AutoMigrate(append(model.KernelModels(), &model.EgressRoute{}, &model.NodeExitPolicy{}, &model.UserSS2022Key{}, &model.NodeEntry{}, &model.L2Standby{}, &model.EntryPoolPolicy{}, &model.UserEntryPool{}, &model.ClientFailureReport{}, &model.SubscriptionGroupProtocol{}, &model.ForwardRouteSubscription{}, &model.IWANLine{}, &model.KernelForwardLatency{}, &model.KernelForwardDNSVantage{})...); err != nil {
		return err
	}
	return SeedKernelDefaults(db)
}

// SeedKernelDefaults creates the default service scopes, the official
// plugin catalog and the plugin target locks that do not exist. It runs at
// every start, after the schema migrations, so a removed default comes
// back as it always did.
func SeedKernelDefaults(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for i := range defaultServiceScopes {
			scope := defaultServiceScopes[i]
			if err := tx.FirstOrCreate(&scope, model.ServiceScope{ID: scope.ID}).Error; err != nil {
				return err
			}
		}
		for i := range officialPluginCatalog {
			plugin := officialPluginCatalog[i]
			if err := tx.FirstOrCreate(&plugin, model.Plugin{ID: plugin.ID}).Error; err != nil {
				return err
			}
		}
		for _, target := range []string{"control", "agent"} {
			lock := model.PluginTargetLock{Target: target}
			if err := tx.FirstOrCreate(&lock, model.PluginTargetLock{Target: target}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// LockPluginInstallationTarget must be called inside the transaction that
// validates and mutates plugin installation intent. The shared target row
// closes races where two packages could otherwise pass dependency/conflict
// checks against the same stale enabled set.
func LockPluginInstallationTarget(tx *gorm.DB, target string) error {
	if tx == nil {
		return errors.New("database is not initialized")
	}
	if target != "control" && target != "agent" {
		return fmt.Errorf("unsupported target %q", target)
	}
	lock := model.PluginTargetLock{Target: target}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&lock).Error; err != nil {
		return err
	}
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&lock, "target = ?", target).Error
}

type EffectiveAccess struct {
	ScopeID string                `json:"scope_id"`
	Groups  []model.AccessGroup   `json:"groups"`
	Grants  []model.ResourceGrant `json:"grants"`
	Quotas  []model.QuotaPolicy   `json:"quotas"`
}

// ResolveEffectiveAccess returns the allow-union of direct and plan-derived
// memberships for one scope. Calling it once per scope keeps counters and
// quota policy ownership isolated.
func ResolveEffectiveAccess(db *gorm.DB, userID uint, planID *uint, scopeID string) (*EffectiveAccess, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	scopeID = strings.TrimSpace(scopeID)
	if userID == 0 || scopeID == "" {
		return nil, errors.New("user_id and scope_id are required")
	}
	var groups []model.AccessGroup
	query := db.Model(&model.AccessGroup{}).
		Distinct("v3_kernel_access_group.*").
		Joins("LEFT JOIN v3_kernel_access_group_user agu ON agu.group_id = v3_kernel_access_group.id").
		Joins("LEFT JOIN v3_kernel_access_group_plan agp ON agp.group_id = v3_kernel_access_group.id").
		Where("v3_kernel_access_group.scope_id = ? AND v3_kernel_access_group.enabled = ?", scopeID, true)
	if planID == nil {
		query = query.Where("agu.user_id = ?", userID)
	} else {
		query = query.Where("agu.user_id = ? OR agp.plan_id = ?", userID, *planID)
	}
	if err := query.Order("v3_kernel_access_group.id").Find(&groups).Error; err != nil {
		return nil, err
	}

	result := &EffectiveAccess{ScopeID: scopeID, Groups: groups, Grants: []model.ResourceGrant{}, Quotas: []model.QuotaPolicy{}}
	if len(groups) == 0 {
		return result, nil
	}
	ids := make([]uint, 0, len(groups))
	for _, group := range groups {
		ids = append(ids, group.ID)
	}
	if err := db.Where("group_id IN ?", ids).Order("resource_type, resource_id, id").Find(&result.Grants).Error; err != nil {
		return nil, err
	}
	if err := db.Where("group_id IN ?", ids).Order("key, id").Find(&result.Quotas).Error; err != nil {
		return nil, err
	}
	return result, nil
}

type PluginManifest struct {
	ID                  string                     `json:"id"`
	Name                string                     `json:"name"`
	Version             string                     `json:"version"`
	APIVersion          string                     `json:"api_version"`
	Publisher           string                     `json:"publisher"`
	Targets             []string                   `json:"targets"`
	Architectures       []string                   `json:"architectures"`
	ArtifactSHA256      string                     `json:"artifact_sha256"`
	Capabilities        []string                   `json:"capabilities"`
	Dependencies        []string                   `json:"dependencies"`
	Conflicts           []string                   `json:"conflicts"`
	Permissions         []string                   `json:"permissions"`
	ConfigSchema        json.RawMessage            `json:"config_schema"`
	SecretFields        []string                   `json:"secret_fields"`
	Entrypoints         map[string]string          `json:"entrypoints"`
	Migration           int64                      `json:"migration_version"`
	ControlRoutes       []string                   `json:"control_routes"`
	FrontendSHA256      string                     `json:"frontend_sha256"`
	WebUI               *PluginWebUI               `json:"webui,omitempty"`
	ControlEntrypoint   *PluginEntrypoint          `json:"control_entrypoint,omitempty"`
	AgentEntrypoint     *PluginEntrypoint          `json:"agent_entrypoint,omitempty"`
	Migrations          *PluginMigrations          `json:"migrations,omitempty"`
	CompatibilityRoutes *PluginCompatibilityRoutes `json:"compatibility_routes,omitempty"`
	RouteContractDigest string                     `json:"route_contract_digest,omitempty"`
	RuntimeAPIVersion   string                     `json:"runtime_api_version,omitempty"`
}

// PluginEntrypoint is a host or Agent plugin entrypoint: a file of the
// artifact (Path, SHA256), or, for a Control entrypoint only, the host linked
// into the Control binary (Builtin, the package id; packages/builtin). The
// fields are omitted when empty, so the signed bytes of a file entrypoint are
// unchanged: {"path":...,"sha256":...}; a built-in one is {"builtin":"<id>"}.
type PluginEntrypoint struct {
	Path    string `json:"path,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Builtin string `json:"builtin,omitempty"`
}

// BuiltinHost reports a Control entrypoint served by the host linked into
// the Control binary.
func (e *PluginEntrypoint) BuiltinHost() bool { return e != nil && e.Builtin != "" }

type PluginMigrations struct {
	Index  string `json:"index"`
	SHA256 string `json:"sha256"`
}

type PluginCompatibilityRoutes struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

// PluginWebUI describes a signed, declarative administrator extension. Bundle
// paths are relative to the verified plugin artifact; they are not URLs and
// are never fetched directly by the control kernel.
type PluginWebUI struct {
	Bundle      PluginWebUIBundle  `json:"bundle"`
	Permissions []string           `json:"permissions"`
	Menus       []PluginWebUIMenu  `json:"menus"`
	Routes      []PluginWebUIRoute `json:"routes"`
}

type PluginWebUIBundle struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type PluginWebUIMenu struct {
	ID         string `json:"id"`
	Parent     string `json:"parent"`
	Label      string `json:"label"`
	Icon       string `json:"icon"`
	Route      string `json:"route"`
	Permission string `json:"permission"`
	Order      int    `json:"order"`
}

// WebUI menu parents are a stable shell contract. Unknown, but otherwise
// valid, package metadata is kept visible in the explicit extensions bucket
// instead of creating an unregistered top-level navigation section.
const (
	PluginWebUIMenuParentServices   = "services"
	PluginWebUIMenuParentOperations = "operations"
	PluginWebUIMenuParentSystem     = "system"
	PluginWebUIMenuParentExtensions = "extensions"
)

type PluginWebUIRoute struct {
	ID         string `json:"id"`
	Path       string `json:"path"`
	Export     string `json:"export"`
	Permission string `json:"permission"`
}

func (m PluginManifest) Validate() error {
	if !safePluginSegment(m.ID) || !safePluginSegment(m.Version) {
		return errors.New("manifest id and version must be safe path segments")
	}
	if strings.TrimSpace(m.Name) == "" {
		return errors.New("manifest name is required")
	}
	if m.APIVersion != pluginManifestAPIVersionV1 && m.APIVersion != pluginManifestAPIVersionV2 {
		return fmt.Errorf("unsupported plugin API version %q", m.APIVersion)
	}
	if m.Publisher != "AnixOps" {
		return fmt.Errorf("publisher %q is not trusted", m.Publisher)
	}
	if len(m.ArtifactSHA256) != sha256.Size*2 {
		return errors.New("artifact_sha256 must be a SHA-256 hex digest")
	}
	if _, err := hex.DecodeString(m.ArtifactSHA256); err != nil {
		return errors.New("artifact_sha256 must be hexadecimal")
	}
	if len(m.Targets) == 0 {
		return errors.New("at least one target is required")
	}
	seenTargets := make(map[string]struct{}, len(m.Targets))
	for _, target := range m.Targets {
		if target != "control" && target != "agent" {
			return fmt.Errorf("unsupported target %q", target)
		}
		if _, exists := seenTargets[target]; exists {
			return fmt.Errorf("duplicate target %q", target)
		}
		seenTargets[target] = struct{}{}
	}
	if err := validateManifestArchitectures(m.Architectures); err != nil {
		return err
	}
	if err := validateManifestRelationships(m.ID, m.Dependencies, m.Conflicts); err != nil {
		return err
	}
	if err := validateManifestPermissions(m.ID, m.Permissions); err != nil {
		return err
	}
	if err := validateManifestCapabilities(m.Capabilities); err != nil {
		return err
	}
	if err := m.validateControlRoutes(); err != nil {
		return err
	}
	for name, entrypoint := range m.Entrypoints {
		if !safePluginSegment(name) {
			return fmt.Errorf("entrypoint name %q is invalid", name)
		}
		if !safePluginRelativePath(entrypoint) {
			return fmt.Errorf("entrypoint %q must be a canonical relative path", name)
		}
	}
	if len(m.ConfigSchema) > 0 {
		trimmed := bytes.TrimSpace(m.ConfigSchema)
		if len(trimmed) > 0 && string(trimmed) != "null" && (trimmed[0] != '{' || !json.Valid(trimmed)) {
			return errors.New("config_schema must be a JSON object")
		}
	}
	if m.FrontendSHA256 != "" && !validSHA256Hex(m.FrontendSHA256) {
		return errors.New("frontend_sha256 must be a SHA-256 hex digest")
	}
	if m.Migration < 0 {
		return errors.New("migration_version must not be negative")
	}
	if m.WebUI != nil {
		if err := m.validateWebUI(); err != nil {
			return err
		}
	}
	if m.APIVersion == pluginManifestAPIVersionV2 {
		if err := m.validateV2Contract(); err != nil {
			return err
		}
	}
	return nil
}

func (m PluginManifest) validateV2Contract() error {
	if manifestSupportsTarget(m, "control") {
		if m.ControlEntrypoint.BuiltinHost() {
			if m.ControlEntrypoint.Builtin != m.ID || m.ControlEntrypoint.Path != "" || m.ControlEntrypoint.SHA256 != "" {
				return errors.New("control_entrypoint must name the package's own built-in host and nothing else")
			}
			if manifestSupportsTarget(m, "agent") {
				return errors.New("a built-in Control host package must not target the agent")
			}
		} else if err := validatePluginEntrypoint(m.ControlEntrypoint, "control_entrypoint"); err != nil {
			return err
		}
	} else if m.ControlEntrypoint != nil {
		return errors.New("control_entrypoint requires the control target")
	}
	if m.Migrations == nil || !safePluginRelativePath(m.Migrations.Index) || !validSHA256Hex(m.Migrations.SHA256) {
		return errors.New("migrations must declare a canonical index and SHA-256 digest")
	}
	if m.CompatibilityRoutes == nil || !safePluginRelativePath(m.CompatibilityRoutes.Path) || !validSHA256Hex(m.CompatibilityRoutes.SHA256) {
		return errors.New("compatibility_routes must declare a canonical path and SHA-256 digest")
	}
	if !validSHA256Hex(m.RouteContractDigest) {
		return errors.New("route_contract_digest must be a SHA-256 digest")
	}
	if manifestSupportsTarget(m, "agent") {
		if err := validatePluginEntrypoint(m.AgentEntrypoint, "agent_entrypoint"); err != nil {
			return err
		}
		if m.RuntimeAPIVersion != agentRuntimeAPIVersionV110 {
			return fmt.Errorf("runtime_api_version must match %q", agentRuntimeAPIVersionV110)
		}
	} else if m.AgentEntrypoint != nil || m.RuntimeAPIVersion != "" {
		return errors.New("agent runtime metadata requires the agent target")
	}
	return nil
}

func validatePluginEntrypoint(value *PluginEntrypoint, field string) error {
	if value == nil || value.Builtin != "" || !safePluginRelativePath(value.Path) || !validSHA256Hex(value.SHA256) {
		return fmt.Errorf("%s must declare a canonical path and SHA-256 digest", field)
	}
	return nil
}

func (m PluginManifest) validateControlRoutes() error {
	if len(m.ControlRoutes) == 0 {
		return nil
	}
	if !manifestSupportsTarget(m, "control") {
		return errors.New("control routes require the control target")
	}
	requiredPermission := PluginAPIPermission(m.ID)
	if !containsPluginString(m.Permissions, requiredPermission) {
		return fmt.Errorf("control routes require permission %q", requiredPermission)
	}
	seen := make(map[string]bool, len(m.ControlRoutes))
	for _, route := range m.ControlRoutes {
		if err := validatePluginControlRoute(route, m.ID); err != nil {
			return err
		}
		if seen[route] {
			return fmt.Errorf("duplicate control route %q", route)
		}
		seen[route] = true
	}
	return nil
}

func (m PluginManifest) validateWebUI() error {
	if !safePluginExtensionID(m.ID) {
		return errors.New("webui plugin id must be a lowercase package identifier")
	}
	if !manifestSupportsTarget(m, "control") {
		return errors.New("webui requires the control target")
	}
	if err := validatePluginBundlePath(m.WebUI.Bundle.Path); err != nil {
		return err
	}
	if !validSHA256Hex(m.WebUI.Bundle.SHA256) {
		return errors.New("webui bundle sha256 must be a SHA-256 hex digest")
	}
	if m.FrontendSHA256 != "" && !strings.EqualFold(m.FrontendSHA256, m.WebUI.Bundle.SHA256) {
		return errors.New("frontend_sha256 does not match webui bundle sha256")
	}

	manifestPermissions := make(map[string]bool, len(m.Permissions))
	for _, permission := range m.Permissions {
		manifestPermissions[permission] = true
	}
	webPermissions := make(map[string]bool, len(m.WebUI.Permissions))
	for _, permission := range m.WebUI.Permissions {
		if !safeNamespacedExtensionID(permission, m.ID) {
			return fmt.Errorf("webui permission %q is outside the plugin namespace", permission)
		}
		if !manifestPermissions[permission] {
			return fmt.Errorf("webui permission %q is not declared by the plugin", permission)
		}
		if webPermissions[permission] {
			return fmt.Errorf("duplicate webui permission %q", permission)
		}
		webPermissions[permission] = true
	}
	if len(m.WebUI.Routes) == 0 {
		return errors.New("webui must declare at least one route")
	}

	routeIDs := make(map[string]bool, len(m.WebUI.Routes))
	routePaths := make(map[string]bool, len(m.WebUI.Routes))
	for _, route := range m.WebUI.Routes {
		if !safeNamespacedExtensionID(route.ID, m.ID) {
			return fmt.Errorf("webui route id %q is outside the plugin namespace", route.ID)
		}
		if routeIDs[route.ID] {
			return fmt.Errorf("duplicate webui route id %q", route.ID)
		}
		if err := validatePluginAdminRoute(route.Path, m.ID); err != nil {
			return err
		}
		if routePaths[route.Path] {
			return fmt.Errorf("duplicate webui route path %q", route.Path)
		}
		if !safeJavaScriptExport(route.Export) {
			return fmt.Errorf("webui route %q has an invalid export", route.ID)
		}
		if !webPermissions[route.Permission] {
			return fmt.Errorf("webui route %q references an undeclared permission", route.ID)
		}
		routeIDs[route.ID] = true
		routePaths[route.Path] = true
	}

	menuIDs := make(map[string]bool, len(m.WebUI.Menus))
	for _, menu := range m.WebUI.Menus {
		if !safeNamespacedExtensionID(menu.ID, m.ID) {
			return fmt.Errorf("webui menu id %q is outside the plugin namespace", menu.ID)
		}
		if menuIDs[menu.ID] {
			return fmt.Errorf("duplicate webui menu id %q", menu.ID)
		}
		if !safeExtensionMetadataText(menu.Label, 160) {
			return fmt.Errorf("webui menu %q has an invalid label", menu.ID)
		}
		if menu.Parent != "" && !safeExtensionToken(menu.Parent) {
			return fmt.Errorf("webui menu %q has an invalid parent", menu.ID)
		}
		if menu.Icon != "" && !safeExtensionToken(menu.Icon) {
			return fmt.Errorf("webui menu %q has an invalid icon", menu.ID)
		}
		if !routePaths[menu.Route] {
			return fmt.Errorf("webui menu %q references an undeclared route", menu.ID)
		}
		if !webPermissions[menu.Permission] {
			return fmt.Errorf("webui menu %q references an undeclared permission", menu.ID)
		}
		menuIDs[menu.ID] = true
	}
	return nil
}

func VerifyPluginRelease(manifestJSON, signature string, publicKey ed25519.PublicKey) (*PluginManifest, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, errors.New("invalid publisher public key")
	}
	var manifest PluginManifest
	decoder := json.NewDecoder(strings.NewReader(manifestJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid manifest: multiple JSON values")
	}
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	canonical, err := CanonicalPluginManifest(manifest)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return nil, errors.New("signature must be base64")
	}
	if !ed25519.Verify(publicKey, canonical, sig) {
		return nil, errors.New("plugin signature verification failed")
	}
	return &manifest, nil
}

// CanonicalPluginManifest is the exact byte representation that AnixOps signs
// and verifies. Schema JSON is recursively parsed and re-encoded so whitespace
// or object-key ordering cannot change a release signature.
func CanonicalPluginManifest(manifest PluginManifest) ([]byte, error) {
	if len(manifest.ConfigSchema) > 0 {
		var schema any
		if err := json.Unmarshal(manifest.ConfigSchema, &schema); err != nil {
			return nil, fmt.Errorf("invalid config_schema: %w", err)
		}
		canonicalSchema, err := json.Marshal(schema)
		if err != nil {
			return nil, err
		}
		manifest.ConfigSchema = bytes.TrimSpace(canonicalSchema)
	}
	return json.Marshal(manifest)
}

func ParseOfficialPluginPublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, errors.New("official plugin public key must be base64")
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("official plugin public key has invalid length")
	}
	return ed25519.PublicKey(decoded), nil
}

func PluginTrustRootFingerprint(publicKey ed25519.PublicKey) string {
	digest := sha256.Sum256(publicKey)
	return hex.EncodeToString(digest[:])
}

func PluginTrustRootKeyID(publicKey ed25519.PublicKey) string {
	fingerprint := PluginTrustRootFingerprint(publicKey)
	if len(fingerprint) <= 16 {
		return fingerprint
	}
	return fingerprint[:16]
}

func EnsurePluginTrustRoot(db *gorm.DB, publicKey ed25519.PublicKey) (*model.PluginTrustRoot, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, ErrPluginTrustRootRequired
	}
	var root *model.PluginTrustRoot
	err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
		var err error
		root, err = ensurePluginTrustRoot(tx, publicKey)
		return err
	})
	if err != nil {
		return nil, err
	}
	return root, nil
}

func ensurePluginTrustRoot(db *gorm.DB, publicKey ed25519.PublicKey) (*model.PluginTrustRoot, error) {
	now := time.Now()
	fingerprint := PluginTrustRootFingerprint(publicKey)
	if err := db.Model(&model.PluginTrustRoot{}).
		Where("fingerprint <> ? AND active = ?", fingerprint, true).
		Updates(map[string]any{"active": false, "retired_at": now}).Error; err != nil {
		return nil, err
	}
	root := model.PluginTrustRoot{
		Fingerprint: fingerprint,
		KeyID:       PluginTrustRootKeyID(publicKey),
		PublicKey:   base64.StdEncoding.EncodeToString(publicKey),
		Publisher:   "AnixOps",
		Active:      true,
		LastSeenAt:  now,
	}
	var existing model.PluginTrustRoot
	if err := db.First(&existing, "fingerprint = ?", fingerprint).Error; err == nil {
		existing.KeyID = root.KeyID
		existing.PublicKey = root.PublicKey
		existing.Publisher = root.Publisher
		existing.Active = true
		existing.LastSeenAt = now
		existing.RetiredAt = nil
		if err := db.Save(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	root.CreatedAt = now
	if err := db.Create(&root).Error; err != nil {
		return nil, err
	}
	return &root, nil
}

func publicKeyForStoredRelease(db *gorm.DB, release model.PluginRelease, fallback ed25519.PublicKey) (ed25519.PublicKey, error) {
	if strings.TrimSpace(release.TrustRootFingerprint) == "" {
		if len(fallback) != ed25519.PublicKeySize {
			return nil, ErrPluginTrustRootRequired
		}
		return fallback, nil
	}
	var root model.PluginTrustRoot
	if err := db.First(&root, "fingerprint = ? AND active = ?", release.TrustRootFingerprint, true).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginTrustRootRequired
		}
		return nil, err
	}
	return ParseOfficialPluginPublicKey(root.PublicKey)
}

func VerifyStoredPluginRelease(db *gorm.DB, release model.PluginRelease, fallback ed25519.PublicKey) (*PluginManifest, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	publicKey, err := publicKeyForStoredRelease(db, release, fallback)
	if err != nil {
		return nil, err
	}
	manifest, err := VerifyPluginRelease(release.ManifestJSON, release.Signature, publicKey)
	if err != nil {
		return nil, err
	}
	if manifest.ID != release.PluginID || manifest.Version != release.Version || manifest.APIVersion != release.APIVersion || !strings.EqualFold(manifest.ArtifactSHA256, release.ArtifactSHA256) {
		return nil, errors.New("stored plugin release metadata does not match its signed manifest")
	}
	return manifest, nil
}

// RegisterPluginRelease admits only a manifest signed by the configured
// AnixOps trust root and records the current exclusive trust-root fingerprint.
// Artifact bytes are uploaded separately and must match the signed manifest
// hash before an install can be enabled.
func RegisterPluginRelease(db *gorm.DB, manifestJSON, signature string, publicKey ed25519.PublicKey) (*model.PluginRelease, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	manifest, err := VerifyPluginRelease(manifestJSON, signature, publicKey)
	if err != nil {
		return nil, err
	}
	canonical, err := CanonicalPluginManifest(*manifest)
	if err != nil {
		return nil, err
	}
	release := &model.PluginRelease{
		PluginID: manifest.ID, Version: manifest.Version, APIVersion: manifest.APIVersion,
		ManifestJSON: string(canonical), ArtifactSHA256: strings.ToLower(manifest.ArtifactSHA256),
		Signature: strings.TrimSpace(signature), PublishedAt: time.Now(),
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		trustRoot, err := ensurePluginTrustRoot(tx, publicKey)
		if err != nil {
			return err
		}
		release.TrustRootFingerprint = trustRoot.Fingerprint
		release.TrustRootKeyID = trustRoot.KeyID
		var plugin model.Plugin
		err = tx.First(&plugin, "id = ?", manifest.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			plugin = model.Plugin{ID: manifest.ID, Name: manifest.Name, Publisher: manifest.Publisher, Official: true}
			if err := tx.Create(&plugin).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if !plugin.Official || plugin.Publisher != manifest.Publisher {
			return errors.New("plugin catalog entry is not an official AnixOps plugin")
		}
		return tx.Create(release).Error
	})
	if err != nil {
		return nil, err
	}
	return release, nil
}

// ValidatePluginReleaseTarget verifies the internally persisted release
// metadata before it is used to create desired state. Releases are immutable,
// so a mismatch here indicates corrupt or manually modified kernel data.
func DecodePluginReleaseManifest(release model.PluginRelease) (*PluginManifest, error) {
	var manifest PluginManifest
	decoder := json.NewDecoder(strings.NewReader(release.ManifestJSON))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid stored plugin manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errors.New("invalid stored plugin manifest: multiple JSON values")
	}
	if err := manifest.Validate(); err != nil {
		return nil, fmt.Errorf("invalid stored plugin manifest: %w", err)
	}
	if manifest.ID != release.PluginID || manifest.Version != release.Version || manifest.APIVersion != release.APIVersion || !strings.EqualFold(manifest.ArtifactSHA256, release.ArtifactSHA256) {
		return nil, errors.New("stored plugin release metadata does not match its signed manifest")
	}
	return &manifest, nil
}

func ValidatePluginReleaseTarget(release model.PluginRelease, target string) error {
	manifest, err := DecodePluginReleaseManifest(release)
	if err != nil {
		return err
	}
	if target != "control" && target != "agent" {
		return fmt.Errorf("unsupported target %q", target)
	}
	if !manifestSupportsTarget(*manifest, target) {
		return fmt.Errorf("plugin release does not support %s target", target)
	}
	return nil
}

func ValidatePluginInstallationPlan(db *gorm.DB, release model.PluginRelease, target string, enabled bool, installationID uint) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if err := ValidatePluginReleaseTarget(release, target); err != nil {
		return err
	}
	manifest, err := DecodePluginReleaseManifest(release)
	if err != nil {
		return err
	}
	if enabled {
		if err := requireVerifiedPluginArtifact(db, release); err != nil {
			return err
		}
		// Resolve the complete desired-release graph before applying the
		// historical "dependency must already be enabled" rule below. This
		// catches missing releases, cycles, and conflicts in transitive
		// dependencies without changing the existing enable order or intent
		// semantics.
		if _, err := ResolvePluginInstallationDependencyGraph(db, release, target); err != nil {
			return err
		}
		return validateEnabledPluginInstallationPlan(db, *manifest, target, installationID)
	}
	if installationID != 0 {
		return validateDisabledPluginInstallationPlan(db, *manifest, target, installationID)
	}
	return nil
}

func validateEnabledPluginInstallationPlan(db *gorm.DB, manifest PluginManifest, target string, installationID uint) error {
	for _, dependency := range manifest.Dependencies {
		var installation model.PluginInstallation
		if err := db.First(&installation, "plugin_id = ? AND target = ? AND enabled = ?", dependency, target, true).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("%w: %s requires %s on %s", ErrPluginDependencyUnsatisfied, manifest.ID, dependency, target)
			}
			return err
		}
		dependencyManifest, err := loadReleaseManifestForInstallation(db, installation)
		if err != nil {
			return fmt.Errorf("installed dependency %s is invalid: %w", dependency, err)
		}
		if !manifestSupportsTarget(*dependencyManifest, target) {
			return fmt.Errorf("%w: dependency %s does not support %s", ErrPluginDependencyUnsatisfied, dependency, target)
		}
	}

	for _, conflict := range manifest.Conflicts {
		query := db.Where("plugin_id = ? AND target = ? AND enabled = ?", conflict, target, true)
		if installationID != 0 {
			query = query.Where("id <> ?", installationID)
		}
		var count int64
		if err := query.Model(&model.PluginInstallation{}).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return fmt.Errorf("%w: %s conflicts with %s on %s", ErrPluginConflict, manifest.ID, conflict, target)
		}
	}

	var enabledInstallations []model.PluginInstallation
	query := db.Where("target = ? AND enabled = ?", target, true)
	if installationID != 0 {
		query = query.Where("id <> ?", installationID)
	}
	if err := query.Order("plugin_id").Find(&enabledInstallations).Error; err != nil {
		return err
	}
	for _, installation := range enabledInstallations {
		otherManifest, err := loadReleaseManifestForInstallation(db, installation)
		if err != nil {
			return fmt.Errorf("enabled plugin %s is invalid: %w", installation.PluginID, err)
		}
		if pluginIDInList(manifest.ID, otherManifest.Conflicts) {
			return fmt.Errorf("%w: %s conflicts with enabled %s on %s", ErrPluginConflict, installation.PluginID, manifest.ID, target)
		}
	}
	return nil
}

func validateDisabledPluginInstallationPlan(db *gorm.DB, manifest PluginManifest, target string, installationID uint) error {
	var enabledInstallations []model.PluginInstallation
	if err := db.Where("target = ? AND enabled = ? AND id <> ?", target, true, installationID).Order("plugin_id").Find(&enabledInstallations).Error; err != nil {
		return err
	}
	for _, installation := range enabledInstallations {
		otherManifest, err := loadReleaseManifestForInstallation(db, installation)
		if err != nil {
			return fmt.Errorf("enabled plugin %s is invalid: %w", installation.PluginID, err)
		}
		if pluginIDInList(manifest.ID, otherManifest.Dependencies) {
			return fmt.Errorf("%w: enabled %s depends on %s on %s", ErrPluginDependencyUnsatisfied, installation.PluginID, manifest.ID, target)
		}
	}
	return nil
}

func loadReleaseManifestForInstallation(db *gorm.DB, installation model.PluginInstallation) (*PluginManifest, error) {
	var release model.PluginRelease
	if err := db.First(&release, "plugin_id = ? AND version = ?", installation.PluginID, installation.DesiredVersion).Error; err != nil {
		return nil, err
	}
	if err := ValidatePluginReleaseTarget(release, installation.Target); err != nil {
		return nil, err
	}
	return DecodePluginReleaseManifest(release)
}

func pluginIDInList(pluginID string, candidates []string) bool {
	for _, candidate := range candidates {
		if candidate == pluginID {
			return true
		}
	}
	return false
}

// VerifyPluginArtifact checks the bytes fetched from the signed repository
// before they are handed to an installer. The manifest signature alone is not
// sufficient when an artifact is replaced at the transport layer.
func VerifyPluginArtifact(manifest PluginManifest, artifact []byte) error {
	if err := validatePluginArtifactSize(len(artifact)); err != nil {
		return err
	}
	digest := sha256.Sum256(artifact)
	actual := hex.EncodeToString(digest[:])
	if !strings.EqualFold(actual, manifest.ArtifactSHA256) {
		return fmt.Errorf("plugin artifact hash mismatch: expected %s, got %s", manifest.ArtifactSHA256, actual)
	}
	return nil
}

func validatePluginArtifactSize(size int) error {
	if size > MaxPluginArtifactBytes {
		return fmt.Errorf("plugin artifact exceeds %d bytes", MaxPluginArtifactBytes)
	}
	return nil
}

func PluginArtifactStorageKey(pluginID, version, artifactSHA256 string) string {
	return path.Join("plugins", pluginID, version, strings.ToLower(artifactSHA256)+".artifact")
}

func PluginAPIPermission(pluginID string) string {
	return pluginID + ".api"
}

// PluginControlBridgeRouteID turns a signed Control route into a stable,
// package-bridge-safe operation name. The original route remains kernel-owned
// request metadata; package processes never choose it themselves.
func PluginControlBridgeRouteID(pluginID, route string) string {
	if !safePluginSegment(pluginID) || validatePluginControlRoute(route, pluginID) != nil {
		return ""
	}
	digest := sha256.Sum256([]byte(pluginID + "\x00" + route))
	return pluginID + ".control." + hex.EncodeToString(digest[:])
}

type PluginControlRouteResolution struct {
	PluginID       string `json:"plugin_id"`
	Version        string `json:"version"`
	InstallationID uint   `json:"installation_id"`
	Generation     uint64 `json:"generation"`
	Route          string `json:"route"`
	MatchedRoute   string `json:"matched_route"`
	Permission     string `json:"permission"`
}

func ResolvePluginControlRoute(db *gorm.DB, publicKey ed25519.PublicKey, pluginID, requestPath string, actorID uint, legacyAdmin bool) (*PluginControlRouteResolution, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if !safePluginSegment(pluginID) {
		return nil, ErrPluginRouteNotFound
	}
	if err := validatePluginControlRoute(requestPath, pluginID); err != nil {
		return nil, ErrPluginRouteNotFound
	}
	var installation model.PluginInstallation
	if err := db.First(&installation, "plugin_id = ? AND target = ? AND enabled = ? AND state IN ?", pluginID, "control", true, []string{"enabled", "healthy"}).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginRouteNotFound
		}
		return nil, err
	}
	if strings.TrimSpace(installation.DesiredVersion) == "" || installation.ObservedVersion != installation.DesiredVersion {
		return nil, fmt.Errorf("%w: plugin %s observed version does not match desired version", ErrExtensionCatalogIntegrity, pluginID)
	}
	var plugin model.Plugin
	if err := db.First(&plugin, "id = ? AND official = ? AND publisher = ?", pluginID, true, "AnixOps").Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginRouteNotFound
		}
		return nil, err
	}
	var release model.PluginRelease
	if err := db.First(&release, "plugin_id = ? AND version = ?", pluginID, installation.ObservedVersion).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPluginRouteNotFound
		}
		return nil, err
	}
	manifest, err := VerifyStoredPluginRelease(db, release, publicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: plugin %s release signature: %v", ErrExtensionCatalogIntegrity, pluginID, err)
	}
	if manifest.ID != pluginID || manifest.Version != installation.ObservedVersion || !manifestSupportsTarget(*manifest, "control") {
		return nil, fmt.Errorf("%w: plugin %s release metadata is not version-bound", ErrExtensionCatalogIntegrity, pluginID)
	}
	if err := requireVerifiedPluginArtifact(db, release); err != nil {
		return nil, err
	}
	matchedRoute, ok := matchPluginControlRoute(manifest.ControlRoutes, requestPath)
	if !ok {
		return nil, ErrPluginRouteNotFound
	}
	permission := PluginAPIPermission(pluginID)
	if !containsPluginString(manifest.Permissions, permission) {
		return nil, fmt.Errorf("%w: plugin %s manifest does not declare %s", ErrExtensionCatalogIntegrity, pluginID, permission)
	}
	allowed, err := userHasPluginAPIPermission(db, actorID, pluginID, permission, legacyAdmin)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPluginRouteForbidden
	}
	if installation.LifecycleGeneration <= 0 {
		return nil, fmt.Errorf("%w: plugin %s lifecycle generation is invalid", ErrExtensionCatalogIntegrity, pluginID)
	}
	generation := uint64(installation.LifecycleGeneration) // #nosec G115 -- value is checked positive immediately above.
	return &PluginControlRouteResolution{
		PluginID: installation.PluginID, Version: installation.ObservedVersion, InstallationID: installation.ID,
		Generation: generation,
		Route:      requestPath, MatchedRoute: matchedRoute, Permission: permission,
	}, nil
}

func StorePluginArtifact(db *gorm.DB, releaseID uint, artifact []byte) (*model.PluginArtifact, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if releaseID == 0 {
		return nil, errors.New("release_id is required")
	}
	if len(artifact) == 0 {
		return nil, errors.New("plugin artifact is empty")
	}
	var result *model.PluginArtifact
	err := WithRetryableTransaction(db, func(tx *gorm.DB) error {
		var release model.PluginRelease
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&release, releaseID).Error; err != nil {
			return err
		}
		manifest, err := DecodePluginReleaseManifest(release)
		if err != nil {
			return err
		}
		if err := VerifyPluginArtifact(*manifest, artifact); err != nil {
			return err
		}
		var existing model.PluginArtifact
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&existing, "release_id = ?", release.ID).Error; err == nil {
			if existing.PluginID == release.PluginID &&
				existing.Version == release.Version &&
				strings.EqualFold(existing.ArtifactSHA256, release.ArtifactSHA256) &&
				existing.SizeBytes == int64(len(artifact)) &&
				bytes.Equal(existing.Data, artifact) {
				result = &existing
				return nil
			}
			return ErrPluginArtifactImmutable
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		artifactRow := model.PluginArtifact{
			ReleaseID: release.ID, PluginID: release.PluginID, Version: release.Version,
			ArtifactSHA256: strings.ToLower(release.ArtifactSHA256),
			SizeBytes:      int64(len(artifact)),
			StorageKey:     PluginArtifactStorageKey(release.PluginID, release.Version, release.ArtifactSHA256),
			Data:           append([]byte(nil), artifact...),
		}
		if err := tx.Create(&artifactRow).Error; err != nil {
			return err
		}
		result = &artifactRow
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func GetPluginArtifact(db *gorm.DB, releaseID uint) (*model.PluginArtifact, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if releaseID == 0 {
		return nil, errors.New("release_id is required")
	}
	var artifact model.PluginArtifact
	if err := db.First(&artifact, "release_id = ?", releaseID).Error; err != nil {
		return nil, err
	}
	return &artifact, nil
}

func requireVerifiedPluginArtifact(db *gorm.DB, release model.PluginRelease) error {
	var artifact model.PluginArtifact
	if err := db.First(&artifact, "release_id = ?", release.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPluginArtifactRequired
		}
		return err
	}
	if artifact.PluginID != release.PluginID || artifact.Version != release.Version || !strings.EqualFold(artifact.ArtifactSHA256, release.ArtifactSHA256) {
		return fmt.Errorf("%w: artifact metadata does not match release", ErrPluginArtifactRequired)
	}
	if artifact.SizeBytes != int64(len(artifact.Data)) || len(artifact.Data) == 0 {
		return fmt.Errorf("%w: artifact blob is missing or truncated", ErrPluginArtifactRequired)
	}
	manifest, err := DecodePluginReleaseManifest(release)
	if err != nil {
		return err
	}
	if err := VerifyPluginArtifact(*manifest, artifact.Data); err != nil {
		return fmt.Errorf("%w: %v", ErrPluginArtifactRequired, err)
	}
	return nil
}

// LoadVerifiedPluginCompatibilityRoutes returns the immutable compatibility
// declaration embedded in a signed v2 Control artifact. Callers still decide
// whether the corresponding installation is active; this helper establishes
// that the declaration itself is bound to the stored release and artifact.
func LoadVerifiedPluginCompatibilityRoutes(db *gorm.DB, release model.PluginRelease, publicKey ed25519.PublicKey) ([]byte, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	manifest, err := VerifyStoredPluginRelease(db, release, publicKey)
	if err != nil {
		return nil, fmt.Errorf("verify plugin release: %w", err)
	}
	if manifest.APIVersion != pluginManifestAPIVersionV2 || !manifestSupportsTarget(*manifest, "control") || manifest.CompatibilityRoutes == nil {
		return nil, errors.New("plugin release does not provide v2 compatibility routes")
	}
	if err := requireVerifiedPluginArtifact(db, release); err != nil {
		return nil, err
	}
	artifact, err := GetPluginArtifact(db, release.ID)
	if err != nil {
		return nil, err
	}
	routes, err := extractPluginArtifactFile(artifact.Data, manifest.CompatibilityRoutes.Path, maxPluginControlEntrypointBytes)
	if err != nil {
		return nil, fmt.Errorf("extract compatibility routes: %w", err)
	}
	if !strings.EqualFold(sha256Bytes(routes), manifest.CompatibilityRoutes.SHA256) {
		return nil, errors.New("compatibility routes digest does not match the package artifact")
	}
	if !strings.EqualFold(sha256Bytes(routes), manifest.RouteContractDigest) {
		return nil, errors.New("route contract digest does not match the package artifact")
	}
	return append([]byte(nil), routes...), nil
}

func resolvePluginControlEntrypoint(manifest PluginManifest, artifact []byte) ([]byte, string, error) {
	if manifest.ControlEntrypoint == nil {
		return nil, "", errors.New("plugin release does not provide a control entrypoint")
	}
	if manifest.ControlEntrypoint.BuiltinHost() {
		return nil, "", errors.New("plugin release names a built-in host, not an entrypoint file")
	}
	if manifest.ControlEntrypoint.Path != pluginControlEntrypointIndexPath {
		entrypoint, err := extractPluginArtifactFile(artifact, manifest.ControlEntrypoint.Path, maxPluginControlEntrypointBytes)
		if err != nil {
			return nil, "", fmt.Errorf("extract control entrypoint: %w", err)
		}
		if !strings.EqualFold(sha256Bytes(entrypoint), manifest.ControlEntrypoint.SHA256) {
			return nil, "", errors.New("control entrypoint digest does not match the package artifact")
		}
		return entrypoint, manifest.ControlEntrypoint.SHA256, nil
	}

	indexBytes, err := extractPluginArtifactFile(artifact, manifest.ControlEntrypoint.Path, maxPluginControlEntrypointBytes)
	if err != nil {
		return nil, "", fmt.Errorf("extract control entrypoint index: %w", err)
	}
	if !strings.EqualFold(sha256Bytes(indexBytes), manifest.ControlEntrypoint.SHA256) {
		return nil, "", errors.New("control entrypoint digest does not match the package artifact")
	}
	decoder := json.NewDecoder(bytes.NewReader(indexBytes))
	decoder.DisallowUnknownFields()
	var index pluginEntrypointIndex
	if err := decoder.Decode(&index); err != nil {
		return nil, "", fmt.Errorf("decode control entrypoint index: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, "", errors.New("control entrypoint index must contain one JSON value")
	}
	if index.Format != pluginEntrypointIndexFormat || len(index.Entries) == 0 || len(index.Entries) > maxPluginEntrypointPlatforms {
		return nil, "", errors.New("control entrypoint index is invalid")
	}

	manifestArchitectures := make(map[string]struct{}, len(manifest.Architectures))
	for _, architecture := range manifest.Architectures {
		manifestArchitectures[architecture] = struct{}{}
	}
	selectedArchitecture := runtime.GOOS + "/" + runtime.GOARCH
	var selected *pluginPlatformEntrypoint
	previousArchitecture := ""
	for indexPosition := range index.Entries {
		entry := &index.Entries[indexPosition]
		if entry.Architecture == "" || entry.Architecture <= previousArchitecture || !validSHA256Hex(entry.SHA256) {
			return nil, "", errors.New("control entrypoint index is invalid")
		}
		previousArchitecture = entry.Architecture
		if _, ok := manifestArchitectures[entry.Architecture]; !ok {
			return nil, "", errors.New("control entrypoint index architecture is not declared by the manifest")
		}
		if entry.Path != "bin/control-host-"+strings.ReplaceAll(entry.Architecture, "/", "-") || !safePluginRelativePath(entry.Path) {
			return nil, "", errors.New("control entrypoint index path is invalid")
		}
		if entry.Architecture == selectedArchitecture {
			selected = entry
		}
	}
	if len(index.Entries) != len(manifestArchitectures) {
		return nil, "", errors.New("control entrypoint index does not cover every manifest architecture")
	}
	if selected == nil {
		return nil, "", fmt.Errorf("control entrypoint does not support %s", selectedArchitecture)
	}
	entrypoint, err := extractPluginArtifactFile(artifact, selected.Path, maxPluginControlEntrypointBytes)
	if err != nil {
		return nil, "", fmt.Errorf("extract control entrypoint %q: %w", selected.Path, err)
	}
	if !strings.EqualFold(sha256Bytes(entrypoint), selected.SHA256) {
		return nil, "", errors.New("control entrypoint digest does not match the package artifact")
	}
	return entrypoint, selected.SHA256, nil
}

// RequireEmbeddedBuiltinRelease admits a built-in host only for the release
// this Control binary carries: the release's canonical manifest must be
// byte-identical to the manifest embedded for the package
// (internal/builtinpkg). The host code is the binary's own, so a built-in
// entrypoint of any other release (an older or newer one, imported through
// the API) has nothing to run here.
func RequireEmbeddedBuiltinRelease(manifest PluginManifest) error {
	if !manifest.ControlEntrypoint.BuiltinHost() {
		return errors.New("plugin release does not name a built-in host")
	}
	set, err := builtinpkg.Default()
	if err != nil {
		return fmt.Errorf("read the built-in packages: %w", err)
	}
	embedded, ok := set.Lookup(manifest.ID)
	if !ok || embedded.Version != manifest.Version {
		return fmt.Errorf("%w: %s %s names a built-in host, but this Control binary does not carry that release", ErrBuiltinReleaseNotEmbedded, manifest.ID, manifest.Version)
	}
	canonical, err := CanonicalPluginManifest(manifest)
	if err != nil {
		return err
	}
	if !bytes.Equal(canonical, embedded.Manifest) {
		return fmt.Errorf("%w: %s %s differs from the manifest this Control binary carries", ErrBuiltinReleaseNotEmbedded, manifest.ID, manifest.Version)
	}
	return nil
}

// ErrBuiltinReleaseNotEmbedded rejects a built-in host release this binary
// does not carry.
var ErrBuiltinReleaseNotEmbedded = errors.New("built-in package release is not embedded in this Control binary")

// PluginArtifactCopyPrefix names the per-process package copies created
// under the Control plugin artifact directory.
const PluginArtifactCopyPrefix = ".anix-package-"

// MaterializePluginControlArtifact writes the verified immutable release bytes
// and the signed v2 Control entrypoint into a private caller-owned directory.
// Task 3's lifecycle dispatcher consumes the resulting ArtifactRef; it still
// owns process supervision and never interprets package metadata itself.
func MaterializePluginControlArtifact(db *gorm.DB, publicKey ed25519.PublicKey, pluginID, version, destinationRoot string) (pluginhost.ArtifactRef, error) {
	if db == nil {
		return pluginhost.ArtifactRef{}, errors.New("database is not initialized")
	}
	if !safePluginSegment(pluginID) || !safePluginSegment(version) {
		return pluginhost.ArtifactRef{}, errors.New("plugin artifact identity is invalid")
	}
	if strings.TrimSpace(destinationRoot) == "" {
		return pluginhost.ArtifactRef{}, errors.New("plugin artifact destination is required")
	}
	absoluteRoot, err := filepath.Abs(destinationRoot)
	if err != nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("resolve plugin artifact destination: %w", err)
	}

	var release model.PluginRelease
	if err := db.First(&release, "plugin_id = ? AND version = ?", pluginID, version).Error; err != nil {
		return pluginhost.ArtifactRef{}, err
	}
	manifest, err := VerifyStoredPluginRelease(db, release, publicKey)
	if err != nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("verify plugin release: %w", err)
	}
	if manifest.APIVersion != pluginManifestAPIVersionV2 || !manifestSupportsTarget(*manifest, "control") || manifest.ControlEntrypoint == nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("plugin release does not provide a v2 control entrypoint")
	}
	if err := requireVerifiedPluginArtifact(db, release); err != nil {
		return pluginhost.ArtifactRef{}, err
	}
	artifact, err := GetPluginArtifact(db, release.ID)
	if err != nil {
		return pluginhost.ArtifactRef{}, err
	}
	builtinHost := manifest.ControlEntrypoint.BuiltinHost()
	var entrypoint []byte
	entrypointDigest := ""
	if builtinHost {
		if err := RequireEmbeddedBuiltinRelease(*manifest); err != nil {
			return pluginhost.ArtifactRef{}, err
		}
	} else {
		entrypoint, entrypointDigest, err = resolvePluginControlEntrypoint(*manifest, artifact.Data)
		if err != nil {
			return pluginhost.ArtifactRef{}, err
		}
	}
	routes, err := extractPluginArtifactFile(artifact.Data, manifest.CompatibilityRoutes.Path, maxPluginControlEntrypointBytes)
	if err != nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("extract compatibility routes: %w", err)
	}
	if !strings.EqualFold(sha256Bytes(routes), manifest.CompatibilityRoutes.SHA256) {
		return pluginhost.ArtifactRef{}, errors.New("compatibility routes digest does not match the package artifact")
	}
	if !strings.EqualFold(sha256Bytes(routes), manifest.RouteContractDigest) {
		return pluginhost.ArtifactRef{}, errors.New("route contract digest does not match the package artifact")
	}
	migrations, err := ParsePluginMigrationIndex(*manifest, artifact.Data)
	if err != nil {
		return pluginhost.ArtifactRef{}, err
	}
	canonicalManifest, err := CanonicalPluginManifest(*manifest)
	if err != nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("canonicalize plugin manifest: %w", err)
	}

	if err := os.MkdirAll(absoluteRoot, 0o700); err != nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("create plugin artifact destination: %w", err)
	}
	directory, err := os.MkdirTemp(absoluteRoot, PluginArtifactCopyPrefix)
	if err != nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("create plugin artifact directory: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(directory)
		}
	}()
	artifactPath := filepath.Join(directory, "package.anxp")
	manifestPath := filepath.Join(directory, "manifest.json")
	entrypointPath := ""
	if !builtinHost {
		entrypointPath = filepath.Join(directory, "control-host")
	}
	materializedRoot, err := os.OpenRoot(directory)
	if err != nil {
		return pluginhost.ArtifactRef{}, fmt.Errorf("open materialized plugin directory: %w", err)
	}
	defer func() { _ = materializedRoot.Close() }()
	if err := writeMaterializedPluginFile(materializedRoot, "package.anxp", artifact.Data, 0o600); err != nil {
		return pluginhost.ArtifactRef{}, err
	}
	if err := writeMaterializedPluginFile(materializedRoot, "manifest.json", canonicalManifest, 0o600); err != nil {
		return pluginhost.ArtifactRef{}, err
	}
	if !builtinHost {
		if err := writeMaterializedPluginFile(materializedRoot, "control-host", entrypoint, 0o700); err != nil {
			return pluginhost.ArtifactRef{}, err
		}
	}
	manifestDigest := sha256.Sum256(canonicalManifest)
	ref := pluginhost.ArtifactRef{
		PackageID:        manifest.ID,
		Version:          manifest.Version,
		ArtifactPath:     artifactPath,
		ArtifactSHA256:   manifest.ArtifactSHA256,
		Builtin:          builtinHost,
		EntrypointPath:   entrypointPath,
		EntrypointSHA256: entrypointDigest,
		ManifestPath:     manifestPath,
		ManifestSHA256:   hex.EncodeToString(manifestDigest[:]),

		MigrationIndexSHA256: strings.ToLower(manifest.Migrations.SHA256),
		Migrations:           migrations,
		Storage:              StorageGrants(*manifest).Storage,
	}
	cleanup = false
	return ref, nil
}

func sha256Bytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func writeMaterializedPluginFile(root *os.Root, name string, data []byte, mode os.FileMode) error {
	if root == nil || !safePluginRelativePath(name) {
		return errors.New("materialized plugin file path is invalid")
	}
	file, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return fmt.Errorf("create materialized plugin file: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return fmt.Errorf("write materialized plugin file: %w", err)
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync materialized plugin file: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close materialized plugin file: %w", err)
	}
	return nil
}

func extractPluginArtifactFile(artifact []byte, memberPath string, maximum int64) ([]byte, error) {
	if !safePluginRelativePath(memberPath) || maximum <= 0 {
		return nil, errors.New("plugin artifact member path is invalid")
	}
	if contents, found, err := extractPluginArtifactFileFromZip(artifact, memberPath, maximum); err != nil {
		return nil, err
	} else if found {
		return contents, nil
	}
	if contents, found, err := extractPluginArtifactFileFromTarGzip(artifact, memberPath, maximum); err != nil {
		return nil, err
	} else if found {
		return contents, nil
	}
	if contents, found, err := extractPluginArtifactFileFromTar(bytes.NewReader(artifact), memberPath, maximum); err != nil {
		return nil, err
	} else if found {
		return contents, nil
	}
	return nil, fmt.Errorf("plugin artifact member %q was not found", memberPath)
}

func extractPluginArtifactFileFromZip(artifact []byte, memberPath string, maximum int64) ([]byte, bool, error) {
	if maximum <= 0 {
		return nil, false, errors.New("plugin artifact maximum must be positive")
	}
	maximumSize := uint64(maximum) // #nosec G115 -- maximum is checked positive immediately above.
	reader, err := zip.NewReader(bytes.NewReader(artifact), int64(len(artifact)))
	if err != nil {
		return nil, false, nil
	}
	var contents []byte
	found := false
	for _, file := range reader.File {
		if strings.TrimPrefix(file.Name, "./") != memberPath {
			continue
		}
		if found {
			return nil, true, fmt.Errorf("plugin artifact member %q appears more than once", memberPath)
		}
		found = true
		if file.FileInfo().IsDir() || !file.Mode().IsRegular() || file.UncompressedSize64 > maximumSize {
			return nil, true, fmt.Errorf("plugin artifact member %q is not a bounded regular file", memberPath)
		}
		opened, err := file.Open()
		if err != nil {
			return nil, true, err
		}
		contents, err = io.ReadAll(io.LimitReader(opened, maximum+1))
		closeErr := opened.Close()
		if err != nil {
			return nil, true, err
		}
		if closeErr != nil {
			return nil, true, closeErr
		}
		if int64(len(contents)) > maximum || len(contents) == 0 {
			return nil, true, fmt.Errorf("plugin artifact member %q is empty or exceeds its size limit", memberPath)
		}
	}
	return contents, found, nil
}

func extractPluginArtifactFileFromTarGzip(artifact []byte, memberPath string, maximum int64) ([]byte, bool, error) {
	gzipReader, err := gzip.NewReader(bytes.NewReader(artifact))
	if err != nil {
		return nil, false, nil
	}
	contents, found, extractErr := extractPluginArtifactFileFromTar(gzipReader, memberPath, maximum)
	closeErr := gzipReader.Close()
	if extractErr != nil {
		return nil, found, extractErr
	}
	if closeErr != nil {
		return nil, found, closeErr
	}
	return contents, found, nil
}

func extractPluginArtifactFileFromTar(reader io.Reader, memberPath string, maximum int64) ([]byte, bool, error) {
	tarReader := tar.NewReader(reader)
	var contents []byte
	found := false
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			return contents, found, nil
		}
		if err != nil {
			return nil, false, nil
		}
		if strings.TrimPrefix(header.Name, "./") != memberPath {
			continue
		}
		if found {
			return nil, true, fmt.Errorf("plugin artifact member %q appears more than once", memberPath)
		}
		found = true
		if (header.Typeflag != tar.TypeReg && header.Typeflag != 0) || header.Size < 0 || header.Size > maximum {
			return nil, true, fmt.Errorf("plugin artifact member %q is not a bounded regular file", memberPath)
		}
		contents, err = io.ReadAll(io.LimitReader(tarReader, maximum+1))
		if err != nil {
			return nil, true, err
		}
		if int64(len(contents)) > maximum || len(contents) == 0 {
			return nil, true, fmt.Errorf("plugin artifact member %q is empty or exceeds its size limit", memberPath)
		}
	}
}

var allowedKernelOperationKinds = map[string]bool{
	"plugin.inspect": true, "plugin.install": true, "plugin.enable": true,
	"plugin.disable": true, "plugin.update": true, "plugin.rollback": true,
	"plugin.health": true, "plugin.configure": true, "topology.plan": true,
	"topology.apply": true, "topology.rollback": true, "topology.status": true,
	"topology.diagnose": true, "agent.ping": true, "node.reload": true,
	"users.reload": true,
}

var agentPluginOperationKinds = map[string]bool{
	"plugin.inspect": true, "plugin.install": true, "plugin.configure": true, "plugin.enable": true,
	"plugin.disable": true, "plugin.update": true, "plugin.rollback": true,
	"plugin.health": true,
}

// IsAgentPluginOperation reports whether an operation has a concrete Agent
// Supervisor implementation. Topology operations remain mediated through
// their concrete plugin lifecycle operations.
func IsAgentPluginOperation(kind string) bool {
	return agentPluginOperationKinds[kind]
}

// CanonicalKernelOperationConfig normalizes a JSON config before it becomes
// part of an idempotency identity and Agent envelope. This makes hash and
// retry behavior independent of request whitespace or object key ordering.
func CanonicalKernelOperationConfig(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		raw = `{}`
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", fmt.Errorf("config must be valid JSON: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return "", errors.New("config must contain one JSON value")
	}
	canonical, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("canonicalize config: %w", err)
	}
	return string(canonical), nil
}

// HashKernelOperationConfig returns the deterministic SHA-256 digest used in
// operation envelopes and persisted plugin configuration rows. The input is
// canonicalized first so semantically identical JSON documents share a hash.
func HashKernelOperationConfig(raw string) (string, error) {
	canonical, err := CanonicalKernelOperationConfig(raw)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(digest[:]), nil
}

// GetPluginConfiguration returns the persisted document for one installation.
// An installation with no document has the deterministic empty-object default.
func GetPluginConfiguration(db *gorm.DB, installationID uint) (*model.PluginConfiguration, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if installationID == 0 {
		return nil, errors.New("installation_id is required")
	}
	var installation model.PluginInstallation
	if err := db.First(&installation, installationID).Error; err != nil {
		return nil, err
	}
	var configuration model.PluginConfiguration
	result := db.Limit(1).Find(&configuration, "installation_id = ?", installationID)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected > 0 {
		return &configuration, nil
	}
	if result.RowsAffected < 0 {
		// GORM should never report a negative row count, but fail closed instead
		// of silently manufacturing a default if a driver violates the contract.
		return nil, errors.New("plugin configuration query returned an invalid row count")
	}
	canonical, err := CanonicalKernelOperationConfig(`{}`)
	if err != nil {
		return nil, err
	}
	configHash, err := HashKernelOperationConfig(canonical)
	if err != nil {
		return nil, err
	}
	return &model.PluginConfiguration{
		InstallationID: installationID, Revision: 0, ConfigJSON: canonical,
		ConfigHash: configHash,
	}, nil
}

// PluginConfigurationSemanticValidator supplies optional, version-bound
// package semantics that cannot be expressed safely in JSON Schema.
type PluginConfigurationSemanticValidator func(pluginID, version string, canonicalConfig json.RawMessage) error

type PluginConfigurationTxHook func(tx *gorm.DB, installation model.PluginInstallation, configuration model.PluginConfiguration) error

func UpdatePluginConfigurationWithValidatorAndHook(db *gorm.DB, publicKey ed25519.PublicKey, installationID uint, rawConfig string, expectedRevision *int64, actorID uint, validator PluginConfigurationSemanticValidator, hook PluginConfigurationTxHook) (*model.PluginConfiguration, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, ErrPluginTrustRootRequired
	}
	if installationID == 0 {
		return nil, errors.New("installation_id is required")
	}
	canonical, err := CanonicalKernelOperationConfig(rawConfig)
	if err != nil {
		return nil, err
	}
	var result *model.PluginConfiguration
	err = WithAgentLifecycleTransaction(db, func(tx *gorm.DB) error {
		result, err = updatePluginConfigurationTx(tx, publicKey, installationID, canonical, expectedRevision, actorID, validator, hook)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// updatePluginConfigurationTx is UpdatePluginConfigurationWithValidatorAndHook
// inside tx, for a canonical document.
func updatePluginConfigurationTx(tx *gorm.DB, publicKey ed25519.PublicKey, installationID uint, canonical string, expectedRevision *int64, actorID uint, validator PluginConfigurationSemanticValidator, hook PluginConfigurationTxHook) (*model.PluginConfiguration, error) {
	var result *model.PluginConfiguration
	err := func() error {
		var targetIdentity struct {
			Target string
		}
		if err := tx.Model(&model.PluginInstallation{}).Select("target").First(&targetIdentity, installationID).Error; err != nil {
			return err
		}
		if err := LockPluginInstallationTarget(tx, targetIdentity.Target); err != nil {
			return err
		}
		var installation model.PluginInstallation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&installation, installationID).Error; err != nil {
			return err
		}
		var plugin model.Plugin
		if err := tx.First(&plugin, "id = ? AND official = ? AND publisher = ?", installation.PluginID, true, "AnixOps").Error; err != nil {
			return errors.New("installation plugin is not an official AnixOps package")
		}
		var release model.PluginRelease
		if err := tx.First(&release, "plugin_id = ? AND version = ?", installation.PluginID, installation.DesiredVersion).Error; err != nil {
			return errors.New("installation desired release is not registered")
		}
		manifest, err := VerifyStoredPluginRelease(tx, release, publicKey)
		if err != nil {
			return fmt.Errorf("installation release verification failed: %w", err)
		}
		if manifest.ID != installation.PluginID || manifest.Version != installation.DesiredVersion || !manifestSupportsTarget(*manifest, installation.Target) {
			return errors.New("installation release is not version-bound to its target")
		}
		packageConfig := canonical
		if installation.Target == "control" {
			if err := rejectRemovedRouteModes(canonical); err != nil {
				return err
			}
		}
		if err := validatePluginConfigurationSchema(*manifest, packageConfig); err != nil {
			return err
		}
		if err := validatePluginConfigurationCapabilities(*manifest, packageConfig); err != nil {
			return err
		}
		if validator != nil {
			if err := validator(manifest.ID, manifest.Version, json.RawMessage(packageConfig)); err != nil {
				return fmt.Errorf("plugin configuration semantic validation failed: %w", err)
			}
		}

		var configuration model.PluginConfiguration
		found := true
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&configuration, "installation_id = ?", installationID).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			found = false
			configuration = model.PluginConfiguration{InstallationID: installationID}
		}
		if expectedRevision != nil && configuration.Revision != *expectedRevision {
			return fmt.Errorf("%w: expected %d, current %d", ErrPluginConfigurationConflict, *expectedRevision, configuration.Revision)
		}
		configHash, err := HashKernelOperationConfig(canonical)
		if err != nil {
			return err
		}
		configuration.Revision++
		configuration.ConfigJSON = canonical
		configuration.ConfigHash = configHash
		configuration.UpdatedBy = actorID
		if found {
			if err := tx.Save(&configuration).Error; err != nil {
				return err
			}
		} else if err := tx.Create(&configuration).Error; err != nil {
			return err
		}
		if err := tx.Model(&installation).Update("config_revision", configuration.Revision).Error; err != nil {
			return err
		}
		installation.ConfigRevision = configuration.Revision
		if hook != nil {
			if err := hook(tx, installation, configuration); err != nil {
				return err
			}
		}
		result = &configuration
		return nil
	}()
	if err != nil {
		return nil, err
	}
	return result, nil
}

// validatePluginConfigurationCapabilities checks the settings a capability
// gives a package, which JSON Schema cannot express: for a release that
// declares telemetry.systemd.read, the per-node systemd_services settings,
// with the glob syntax the Agent's collector applies
// (sdk/telemetry/systemdreport.ParseConfig).
func validatePluginConfigurationCapabilities(manifest PluginManifest, canonicalConfig string) error {
	if !containsPluginCapability(manifest.Capabilities, systemdreport.Capability) {
		return nil
	}
	if _, err := systemdreport.ParseConfig([]byte(canonicalConfig)); err != nil {
		return fmt.Errorf("plugin config does not satisfy its schema: %w", err)
	}
	return nil
}

func validatePluginConfigurationSchema(manifest PluginManifest, canonicalConfig string) error {
	schema := bytes.TrimSpace(manifest.ConfigSchema)
	if len(schema) == 0 || string(schema) == "null" {
		return nil
	}
	compiler := jsonschema.NewCompiler()
	compiler.LoadURL = func(reference string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external JSON Schema reference %q is not allowed", reference)
	}
	resourceURL := "https://anixops.invalid/plugins/" + url.PathEscape(manifest.ID) + "/" + url.PathEscape(manifest.Version) + "/config-schema.json"
	if err := compiler.AddResource(resourceURL, bytes.NewReader(schema)); err != nil {
		return fmt.Errorf("compile plugin config schema: %w", err)
	}
	compiled, err := compiler.Compile(resourceURL)
	if err != nil {
		return fmt.Errorf("compile plugin config schema: %w", err)
	}
	var value any
	decoder := json.NewDecoder(strings.NewReader(canonicalConfig))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("decode plugin config: %w", err)
	}
	if err := compiled.Validate(value); err != nil {
		return fmt.Errorf("plugin config does not satisfy its schema: %w", err)
	}
	return nil
}

// CreateKernelOperation persists a requested operation before it can be sent
// to an agent. A repeated idempotency key returns the original operation,
// including after a control-plane restart.
func CreateKernelOperation(db *gorm.DB, operation model.KernelOperation) (*model.KernelOperation, bool, error) {
	if db == nil {
		return nil, false, errors.New("database is not initialized")
	}
	if strings.TrimSpace(operation.ID) == "" || strings.TrimSpace(operation.IdempotencyKey) == "" {
		return nil, false, errors.New("operation_id and idempotency_key are required")
	}
	if _, err := uuid.Parse(operation.ID); err != nil {
		return nil, false, errors.New("operation_id must be a UUID")
	}
	if !allowedKernelOperationKinds[operation.Kind] {
		return nil, false, fmt.Errorf("unsupported operation kind %q", operation.Kind)
	}
	if strings.TrimSpace(operation.PluginID) == "" || strings.TrimSpace(operation.TargetVersion) == "" {
		return nil, false, errors.New("plugin_id and target_version are required")
	}
	if operation.EnvelopeVersion == "" {
		operation.EnvelopeVersion = KernelOperationEnvelopeVersion
	}
	if operation.EnvelopeVersion != KernelOperationEnvelopeVersion {
		return nil, false, fmt.Errorf("unsupported operation envelope version %q", operation.EnvelopeVersion)
	}
	canonicalConfig, err := CanonicalKernelOperationConfig(operation.ConfigJSON)
	if err != nil {
		return nil, false, err
	}
	operation.ConfigJSON = canonicalConfig
	computedHash, err := HashKernelOperationConfig(operation.ConfigJSON)
	if err != nil {
		return nil, false, err
	}
	if operation.ConfigHash != "" && !strings.EqualFold(operation.ConfigHash, computedHash) {
		return nil, false, errors.New("config_hash does not match canonical config")
	}
	operation.ConfigHash = computedHash
	if operation.DeadlineAt == nil || !operation.DeadlineAt.After(time.Now()) {
		return nil, false, errors.New("deadline_at must be in the future")
	}
	if operation.State == "" {
		operation.State = "pending"
	}
	if operation.State != "pending" {
		return nil, false, errors.New("new operations must start pending")
	}
	if operation.DependsOnOperationID != "" {
		if operation.NodeID == nil {
			return nil, false, errors.New("operation dependencies are supported only for Agent operations")
		}
		if _, err := uuid.Parse(operation.DependsOnOperationID); err != nil {
			return nil, false, errors.New("depends_on_operation_id must be a UUID")
		}
	}

	var result *model.KernelOperation
	reused := false
	err = db.Transaction(func(tx *gorm.DB) error {
		var existing model.KernelOperation
		if err := tx.Where("idempotency_key = ?", operation.IdempotencyKey).First(&existing).Error; err == nil {
			if !sameKernelOperation(existing, operation) {
				return errors.New("idempotency_key is already bound to a different operation")
			}
			result = &existing
			reused = true
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var plugin model.Plugin
		if err := tx.First(&plugin, "id = ? AND official = ?", operation.PluginID, true).Error; err != nil {
			return errors.New("operation plugin is not an official catalog plugin")
		}
		var release model.PluginRelease
		if err := tx.First(&release, "plugin_id = ? AND version = ?", operation.PluginID, operation.TargetVersion).Error; err != nil {
			return errors.New("operation target version is not a registered plugin release")
		}
		requiredTarget := "control"
		if operation.NodeID != nil {
			if !IsAgentPluginOperation(operation.Kind) {
				return fmt.Errorf("operation kind %q is not dispatchable to the Agent Supervisor", operation.Kind)
			}
			requiredTarget = "agent"
		}
		if err := ValidatePluginReleaseTarget(release, requiredTarget); err != nil {
			return fmt.Errorf("operation plugin release is invalid: %w", err)
		}

		var cursor *model.NodeOperationRevision
		var dependencyRevision int64
		if operation.NodeID != nil {
			var nodeCount int64
			if err := tx.Model(&model.Node{}).Where("id = ?", *operation.NodeID).Count(&nodeCount).Error; err != nil {
				return err
			}
			if nodeCount == 0 {
				return errors.New("operation node does not exist")
			}
			if operation.DependsOnOperationID != "" {
				var dependency model.KernelOperation
				if err := tx.First(&dependency, "id = ?", operation.DependsOnOperationID).Error; err != nil {
					return fmt.Errorf("load operation dependency: %w", err)
				}
				if dependency.NodeID == nil || *dependency.NodeID != *operation.NodeID || dependency.PluginID != operation.PluginID {
					return errors.New("operation dependency must target the same node and plugin")
				}
				dependencyRevision = dependency.Revision
			}
			seed := model.NodeOperationRevision{NodeID: *operation.NodeID}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
				return err
			}
			cursor = &model.NodeOperationRevision{}
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(cursor, "node_id = ?", *operation.NodeID).Error; err != nil {
				return err
			}
			if operation.Revision == 0 {
				operation.Revision = cursor.DesiredRevision + 1
			} else if operation.Revision != cursor.DesiredRevision+1 {
				return fmt.Errorf("revision %d must be the next node desired revision %d", operation.Revision, cursor.DesiredRevision+1)
			}
			if dependencyRevision >= operation.Revision {
				return errors.New("operation dependency must have an earlier revision")
			}
		} else if operation.Revision <= 0 {
			operation.Revision = 1
		}

		created := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "idempotency_key"}}, DoNothing: true}).Create(&operation)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			if err := tx.Where("idempotency_key = ?", operation.IdempotencyKey).First(&existing).Error; err != nil {
				return err
			}
			if !sameKernelOperation(existing, operation) {
				return errors.New("idempotency_key is already bound to a different operation")
			}
			result = &existing
			reused = true
			return nil
		}
		if cursor != nil {
			if err := tx.Model(cursor).Update("desired_revision", operation.Revision).Error; err != nil {
				return err
			}
		}
		result = &operation
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, reused, nil
}

func manifestSupportsTarget(manifest PluginManifest, target string) bool {
	for _, candidate := range manifest.Targets {
		if candidate == target {
			return true
		}
	}
	return false
}

func validateManifestArchitectures(architectures []string) error {
	seen := make(map[string]struct{}, len(architectures))
	for _, architecture := range architectures {
		if architecture == "" || architecture != strings.TrimSpace(architecture) || architecture != strings.ToLower(architecture) {
			return fmt.Errorf("invalid plugin architecture %q", architecture)
		}
		if _, exists := seen[architecture]; exists {
			return fmt.Errorf("duplicate plugin architecture %q", architecture)
		}
		seen[architecture] = struct{}{}
		if architecture == "any" || architecture == "*" {
			continue
		}
		parts := strings.FieldsFunc(architecture, func(char rune) bool { return char == '/' || char == '-' })
		if len(parts) == 0 || len(parts) > 2 {
			return fmt.Errorf("invalid plugin architecture %q", architecture)
		}
		for _, part := range parts {
			if !safePluginSegment(part) {
				return fmt.Errorf("invalid plugin architecture %q", architecture)
			}
		}
	}
	return nil
}

func validateManifestRelationships(pluginID string, dependencies, conflicts []string) error {
	dependencySet := make(map[string]struct{}, len(dependencies))
	for _, dependency := range dependencies {
		if !safePluginSegment(dependency) {
			return fmt.Errorf("invalid plugin dependency %q", dependency)
		}
		if dependency == pluginID {
			return errors.New("plugin cannot depend on itself")
		}
		if _, exists := dependencySet[dependency]; exists {
			return fmt.Errorf("duplicate plugin dependency %q", dependency)
		}
		dependencySet[dependency] = struct{}{}
	}
	conflictSet := make(map[string]struct{}, len(conflicts))
	for _, conflict := range conflicts {
		if !safePluginSegment(conflict) {
			return fmt.Errorf("invalid plugin conflict %q", conflict)
		}
		if conflict == pluginID {
			return errors.New("plugin cannot conflict with itself")
		}
		if _, exists := conflictSet[conflict]; exists {
			return fmt.Errorf("duplicate plugin conflict %q", conflict)
		}
		if _, dependency := dependencySet[conflict]; dependency {
			return fmt.Errorf("plugin %q cannot be both a dependency and a conflict", conflict)
		}
		conflictSet[conflict] = struct{}{}
	}
	return nil
}

func validSHA256Hex(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func safePluginSegment(value string) bool {
	if value == "" || len(value) > 120 || value != strings.TrimSpace(value) || value == "." || value == ".." || strings.ContainsAny(value, `\/`) {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || strings.ContainsRune("._+-", char) {
			continue
		}
		return false
	}
	return true
}

func safePluginRelativePath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\\x00") || path.IsAbs(value) {
		return false
	}
	cleaned := path.Clean(value)
	if cleaned != value || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return false
	}
	for _, segment := range strings.Split(cleaned, "/") {
		if !safePluginSegment(segment) {
			return false
		}
	}
	return true
}

func safePluginExtensionID(value string) bool {
	if len(value) == 0 || len(value) > 120 || value != strings.ToLower(value) || !asciiAlphaNumeric(value[0]) || !asciiAlphaNumeric(value[len(value)-1]) || strings.Contains(value, "..") {
		return false
	}
	for i := range value {
		if !asciiAlphaNumeric(value[i]) && value[i] != '.' && value[i] != '_' && value[i] != '-' {
			return false
		}
	}
	return true
}

func safeNamespacedExtensionID(value, pluginID string) bool {
	if !strings.HasPrefix(value, pluginID+".") || len(value) > 180 || !safeExtensionToken(value) {
		return false
	}
	return !strings.Contains(value, "..")
}

func safeExtensionToken(value string) bool {
	if value == "" {
		return false
	}
	for i := range value {
		if !asciiAlphaNumeric(value[i]) && value[i] != '.' && value[i] != '_' && value[i] != '-' {
			return false
		}
	}
	return true
}

func asciiAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9'
}

func safeExtensionMetadataText(value string, maxLength int) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxLength {
		return false
	}
	for i := range value {
		if value[i] < 0x20 {
			return false
		}
		if value[i] == 0x7f {
			return false
		}
	}
	return true
}

func safeJavaScriptExport(value string) bool {
	if value == "" || len(value) > 120 {
		return false
	}
	for i := range value {
		character := value[i]
		if i == 0 {
			if asciiAlpha(character) || character == '_' || character == '$' {
				continue
			}
			return false
		}
		if !asciiAlphaNumeric(character) && character != '_' && character != '$' {
			return false
		}
	}
	return true
}

func asciiAlpha(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z'
}

func validateManifestPermissions(pluginID string, permissions []string) error {
	seen := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		if !safeNamespacedExtensionID(permission, pluginID) {
			return fmt.Errorf("plugin permission %q is outside the plugin namespace", permission)
		}
		if seen[permission] {
			return fmt.Errorf("duplicate plugin permission %q", permission)
		}
		seen[permission] = true
	}
	return nil
}

func validatePluginBundlePath(value string) error {
	if value == "" || value != strings.TrimSpace(value) || !strings.HasPrefix(value, "webui/") {
		return errors.New("webui bundle path must be relative to the webui directory")
	}
	if strings.ContainsAny(value, "\\%?#\r\n\t ") || strings.HasPrefix(value, "/") || path.Clean(value) != value || strings.Contains(value, "//") {
		return errors.New("webui bundle path is unsafe")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != value {
		return errors.New("webui bundle path is unsafe")
	}
	for i := range value {
		character := value[i]
		if !asciiAlphaNumeric(character) && character != '.' && character != '_' && character != '-' && character != '/' {
			return errors.New("webui bundle path contains unsupported characters")
		}
	}
	extension := strings.ToLower(path.Ext(value))
	if extension != ".js" && extension != ".mjs" {
		return errors.New("webui bundle must be a JavaScript module")
	}
	return nil
}

func validatePluginControlRoute(value, pluginID string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return errors.New("control route is required")
	}
	if strings.Count(value, "*") > 1 || (strings.Contains(value, "*") && !strings.HasSuffix(value, "/*")) {
		return fmt.Errorf("control route %q has an invalid wildcard", value)
	}
	routePath := strings.TrimSuffix(value, "/*")
	if routePath == "" {
		return fmt.Errorf("control route %q is unsafe", value)
	}
	namespaces := []string{
		"/api/v3/plugins/" + pluginID,
		"/api/v4/plugins/" + pluginID,
	}
	insideNamespace := false
	for _, namespace := range namespaces {
		if routePath == namespace || strings.HasPrefix(routePath, namespace+"/") {
			insideNamespace = true
			break
		}
	}
	if !insideNamespace {
		return fmt.Errorf("control route %q is outside the plugin gateway namespace", value)
	}
	if strings.ContainsAny(routePath, "\\%?#\r\n\t ") || path.Clean(routePath) != routePath || strings.Contains(routePath, "//") {
		return fmt.Errorf("control route %q is unsafe", value)
	}
	parsed, err := url.Parse(routePath)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != routePath {
		return fmt.Errorf("control route %q is unsafe", value)
	}
	return nil
}

func validatePluginAdminRoute(value, pluginID string) error {
	namespace := "/admin/extensions/" + pluginID
	if value == "" || value != strings.TrimSpace(value) || (value != namespace && !strings.HasPrefix(value, namespace+"/")) {
		return fmt.Errorf("webui route %q is outside %s", value, namespace)
	}
	if strings.ContainsAny(value, "\\%?#\r\n\t ") || path.Clean(value) != value || strings.Contains(value, "//") {
		return fmt.Errorf("webui route %q is unsafe", value)
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != value {
		return fmt.Errorf("webui route %q is unsafe", value)
	}
	return nil
}

func matchPluginControlRoute(routes []string, requestPath string) (string, bool) {
	for _, route := range routes {
		if strings.HasSuffix(route, "/*") {
			prefix := strings.TrimSuffix(route, "/*")
			if requestPath == prefix || strings.HasPrefix(requestPath, prefix+"/") {
				return route, true
			}
			continue
		}
		if requestPath == route {
			return route, true
		}
	}
	return "", false
}

func userHasPluginAPIPermission(db *gorm.DB, actorID uint, pluginID, permission string, legacyAdmin bool) (bool, error) {
	access, err := ResolveActorPluginAccess(db, actorID, legacyAdmin)
	if err != nil {
		return false, err
	}
	return access.Allows(pluginID, permission), nil
}

func containsPluginString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func sameKernelOperation(existing, requested model.KernelOperation) bool {
	if existing.ID != requested.ID || existing.EnvelopeVersion != requested.EnvelopeVersion || existing.PluginID != requested.PluginID || existing.TargetVersion != requested.TargetVersion || existing.Kind != requested.Kind || existing.Revision != requested.Revision || existing.ConfigHash != requested.ConfigHash || existing.ConfigJSON != requested.ConfigJSON || existing.DependsOnOperationID != requested.DependsOnOperationID {
		return false
	}
	if existing.NodeID == nil || requested.NodeID == nil {
		if existing.NodeID != nil || requested.NodeID != nil {
			return false
		}
	} else if *existing.NodeID != *requested.NodeID {
		return false
	}
	if existing.DeadlineAt == nil || requested.DeadlineAt == nil {
		return existing.DeadlineAt == nil && requested.DeadlineAt == nil
	}
	return existing.DeadlineAt.Equal(*requested.DeadlineAt)
}

// ExpireKernelOperations marks undispatched or in-flight work that has passed
// its deadline. A future supervisor worker may call this on a timer; read APIs
// also invoke it so stale intent is never presented as pending.
func ExpireKernelOperations(db *gorm.DB, now time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("database is not initialized")
	}
	if now.IsZero() {
		now = time.Now()
	}
	result := db.Model(&model.KernelOperation{}).
		Where("state IN ? AND deadline_at IS NOT NULL AND deadline_at <= ?", []string{"pending", "dispatching", "running", "cancel_requested"}, now).
		Updates(map[string]any{"state": "timed_out", "last_error": "operation deadline exceeded"})
	return result.RowsAffected, result.Error
}

func CancelKernelOperation(db *gorm.DB, operationID string, at time.Time) (*model.KernelOperation, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if _, err := uuid.Parse(operationID); err != nil {
		return nil, ErrInvalidKernelOperationID
	}
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := ExpireKernelOperations(db, at); err != nil {
		return nil, err
	}
	var operation model.KernelOperation
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&operation, "id = ?", operationID).Error; err != nil {
			return err
		}
		switch operation.State {
		case "pending":
			operation.State, operation.CancelAt, operation.LastError = "cancelled", &at, "operation cancelled before dispatch"
		case "dispatching", "running":
			operation.State, operation.CancelAt, operation.LastError = "cancel_requested", &at, "operation cancellation requested"
		case "cancel_requested", "cancelled":
			return nil
		default:
			return fmt.Errorf("%w: current state is %s", ErrKernelOperationNotPending, operation.State)
		}
		return tx.Save(&operation).Error
	})
	if err != nil {
		return nil, err
	}
	return &operation, nil
}
