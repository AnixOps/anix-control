package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"gorm.io/gorm"
)

// TopologyDeploymentPreviewInput identifies a revision and the optional
// rollout slice to inspect. Preview is deliberately read-only: it never
// creates a deployment, operation, or observed-state row.
type TopologyDeploymentPreviewInput struct {
	TopologyID    uint   `json:"topology_id"`
	RevisionID    uint   `json:"revision_id"`
	RolloutGroup  string `json:"rollout_group"`
	FailurePolicy string `json:"failure_policy"`
}

// TopologyDeploymentDiagnosticIssue is stable, machine-readable preflight
// output. Code and path intentionally mirror ValidateTopology where possible
// so the editor can point at the same field that the planner would reject.
type TopologyDeploymentDiagnosticIssue struct {
	Code     string `json:"code"`
	Path     string `json:"path"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

type TopologyDeploymentDiagnosticCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// TopologyDeploymentPreviewStep describes the durable operations that a real
// plan would create. Operation IDs are intentionally absent because preview
// must not reserve identities or mutate the operation stream.
type TopologyDeploymentPreviewStep struct {
	Order         int      `json:"order"`
	VertexID      uint     `json:"vertex_id"`
	VertexKey     string   `json:"vertex_key"`
	NodeID        uint     `json:"node_id"`
	PluginID      string   `json:"plugin_id"`
	Role          string   `json:"role"`
	TargetVersion string   `json:"target_version"`
	Removal       bool     `json:"removal"`
	ApplyAction   string   `json:"apply_action"`
	RollbackMode  string   `json:"rollback_mode"`
	Dependencies  []string `json:"dependencies,omitempty"`
	Operations    []string `json:"operations"`
	ConfigHash    string   `json:"config_hash,omitempty"`
}

type TopologyDeploymentPreview struct {
	TopologyID         uint                                `json:"topology_id"`
	RevisionID         uint                                `json:"revision_id"`
	PreviousRevisionID *uint                               `json:"previous_revision_id,omitempty"`
	RolloutGroup       string                              `json:"rollout_group,omitempty"`
	FailurePolicy      string                              `json:"failure_policy"`
	Valid              bool                                `json:"valid"`
	Issues             []TopologyDeploymentDiagnosticIssue `json:"issues"`
	Checks             []TopologyDeploymentDiagnosticCheck `json:"checks"`
	Steps              []TopologyDeploymentPreviewStep     `json:"steps"`
}

// PreviewTopologyDeployment performs all checks that are safe to perform
// before PlanTopologyDeployment. It intentionally does not call the planner,
// since the planner persists a deployment row by design.
func PreviewTopologyDeployment(db *gorm.DB, input TopologyDeploymentPreviewInput) (*TopologyDeploymentPreview, error) {
	if db == nil {
		return nil, errors.New("database is not initialized")
	}
	if input.TopologyID == 0 || input.RevisionID == 0 {
		return nil, errors.New("topology_id and revision_id are required")
	}
	input.RolloutGroup = strings.TrimSpace(input.RolloutGroup)
	input.FailurePolicy = strings.TrimSpace(input.FailurePolicy)
	if input.FailurePolicy == "" {
		input.FailurePolicy = "stop_and_rollback"
	}
	preview := &TopologyDeploymentPreview{
		TopologyID: input.TopologyID, RevisionID: input.RevisionID,
		RolloutGroup: input.RolloutGroup, FailurePolicy: input.FailurePolicy,
		Issues: []TopologyDeploymentDiagnosticIssue{}, Checks: []TopologyDeploymentDiagnosticCheck{},
		Steps: []TopologyDeploymentPreviewStep{},
	}
	issueKeys := make(map[string]struct{})

	var topology model.Topology
	if err := db.First(&topology, input.TopologyID).Error; err != nil {
		return nil, err
	}
	var revision model.TopologyRevision
	if err := db.First(&revision, "id = ? AND topology_id = ?", input.RevisionID, input.TopologyID).Error; err != nil {
		return nil, err
	}
	preview.PreviousRevisionID = topology.ActiveRevisionID

	var vertices []model.TopologyVertex
	var edges []model.TopologyEdge
	if err := db.Where("revision_id = ?", revision.ID).Order("key").Find(&vertices).Error; err != nil {
		return nil, err
	}
	if err := db.Where("revision_id = ?", revision.ID).Order("id").Find(&edges).Error; err != nil {
		return nil, err
	}

	appendIssue := func(code, path, message string) {
		key := code + "\x00" + path + "\x00" + message
		if _, exists := issueKeys[key]; exists {
			return
		}
		issueKeys[key] = struct{}{}
		preview.Issues = append(preview.Issues, TopologyDeploymentDiagnosticIssue{
			Code: code, Path: path, Message: message, Severity: "error",
		})
	}
	appendValidationIssues := func(issues []TopologyValidationIssue) {
		for _, issue := range issues {
			appendIssue(issue.Code, issue.Path, issue.Message)
		}
	}

	// Graph validation is always performed on the complete immutable revision,
	// even for a canary. A canary may select fewer vertices, but it must not hide
	// a malformed edge or cycle in the revision it is previewing.
	graphIssues := ValidateTopology(TopologyRevisionInput{Vertices: vertices, Edges: edges})
	if len(vertices) == 0 && len(edges) == 0 && input.RolloutGroup != "" {
		// Empty canary revisions are the explicit pure-removal form supported by
		// the durable planner; do not report the normal full-rollout
		// vertices_required issue for that case.
		graphIssues = nil
	}
	appendValidationIssues(graphIssues)
	if len(graphIssues) == 0 {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "dag", Status: "passed", Message: "revision is a directed acyclic graph"})
	} else {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "dag", Status: "failed", Message: "revision graph validation failed"})
	}

	order, orderErr := topologyVertexApplyOrder(vertices, edges)
	if len(vertices) == 0 && len(edges) == 0 && input.RolloutGroup != "" {
		// A canary may intentionally remove every vertex in its rollout group.
		// The previous active revision supplies the removal steps below.
		orderErr = nil
		order = []string{}
	}
	if orderErr != nil {
		appendIssue("dag_invalid", "edges", orderErr.Error())
		// Keep a deterministic step skeleton for the editor even when graph
		// validation fails. Apply remains blocked by the issue; the skeleton
		// makes the affected vertices and intended operations visible.
		order = make([]string, 0, len(vertices))
		for _, vertex := range vertices {
			order = append(order, vertex.Key)
		}
		sort.Strings(order)
	}

	assignmentByKey := make(map[string]model.NodeServiceAssignment, len(vertices))
	selected := make(map[string]bool, len(vertices))
	selectedPlugin := make(map[string]string)
	assignmentIssues := 0
	for index := range vertices {
		vertex := vertices[index]
		if vertex.NodeID == nil || strings.TrimSpace(vertex.PluginID) == "" {
			if vertex.Kind == "plugin" || vertex.Role != "" {
				appendIssue("vertex_not_deployable", fmt.Sprintf("vertices[%d]", index), "plugin vertex requires a physical node and plugin ID")
				assignmentIssues++
			}
			continue
		}
		var nodeCount int64
		if err := db.Model(&model.Node{}).Where("id = ?", *vertex.NodeID).Count(&nodeCount).Error; err != nil {
			return nil, err
		}
		if nodeCount == 0 {
			appendIssue("node_not_found", fmt.Sprintf("vertices[%d].node_id", index), "physical node does not exist")
			assignmentIssues++
			continue
		}
		var assignment model.NodeServiceAssignment
		err := db.Where("node_id = ? AND service_scope = ? AND plugin_id = ? AND role = ? AND enabled = ? AND delete_pending = ?",
			*vertex.NodeID, topology.ServiceScope, vertex.PluginID, vertex.Role, true, false).First(&assignment).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			appendIssue("assignment_missing", fmt.Sprintf("vertices[%d]", index), "node has no enabled assignment for this plugin role in the topology scope")
			assignmentIssues++
			if input.RolloutGroup == "" {
				// A full preview still exposes the intended operation even when
				// its assignment is missing, so the operator can fix it without
				// losing the rest of the diagnostic context.
				selected[vertex.Key] = true
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		assignmentByKey[vertex.Key] = assignment
		if input.RolloutGroup == "" || strings.TrimSpace(assignment.RolloutGroup) == input.RolloutGroup {
			selected[vertex.Key] = true
			pluginKey := topologyDeploymentPluginKey(*vertex.NodeID, vertex.PluginID, vertex.Role)
			if previous, exists := selectedPlugin[pluginKey]; exists {
				appendIssue("duplicate_plugin_instance", "vertices["+strconv.Itoa(index)+"]", fmt.Sprintf("plugin instance duplicates vertex %q on node %d", previous, *vertex.NodeID))
			} else {
				selectedPlugin[pluginKey] = vertex.Key
			}
		}
	}
	if assignmentIssues == 0 {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "assignments", Status: "passed", Message: "all selected plugin vertices have enabled assignments"})
	} else {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "assignments", Status: "failed", Message: "one or more selected vertices have no usable assignment"})
	}

	// Runtime constraints supplement ValidateTopology with the fields used by
	// forwarding package schemas (listen_port/target_port and nested rules).
	runtimeIssues := validateTopologyRuntimeConstraints(vertices, edges)
	for _, issue := range runtimeIssues {
		appendIssue(issue.Code, issue.Path, issue.Message)
	}
	for index, vertex := range vertices {
		if vertex.PluginID != "nftables-forward" {
			continue
		}
		for _, issue := range validateNftablesForwardTopologyConfig(vertex.ConfigJSON, fmt.Sprintf("vertices[%d].config", index)) {
			appendIssue(issue.Code, issue.Path, issue.Message)
		}
	}
	if len(runtimeIssues) == 0 {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "runtime_constraints", Status: "passed", Message: "ports, address families, MTU and secret references are valid"})
	} else {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "runtime_constraints", Status: "failed", Message: "runtime port/address/MTU/secret checks failed"})
	}

	// Verify each selected release and its complete dependency closure. Cache by
	// plugin@version so a topology with repeated roles does not re-hash an
	// artifact repeatedly.
	releaseCache := make(map[string]releaseResultCompat)
	releaseErrors := 0
	for key, assignment := range assignmentByKey {
		if !selected[key] {
			continue
		}
		vertex := findTopologyVertex(vertices, key)
		version := strings.TrimSpace(assignment.DesiredVersion)
		path := "vertices[" + strconv.Itoa(vertexIndex(vertices, key)) + "].plugin_id"
		if version == "" {
			appendIssue("desired_version_missing", path, "assignment has no desired plugin version")
			releaseErrors++
			continue
		}
		cacheKey := assignment.PluginID + "@" + version
		result, cached := releaseCache[cacheKey]
		if !cached {
			result = diagnoseTopologyRelease(db, assignment.PluginID, version)
			releaseCache[cacheKey] = result
		}
		if result.err != nil {
			code, message := classifyTopologyReleaseError(result.err)
			appendIssue(code, path, message)
			releaseErrors++
			continue
		}
		if err := validatePluginConfigurationSchema(*result.manifest, canonicalTopologyConfig(vertex.ConfigJSON)); err != nil {
			appendIssue("config_schema_invalid", path+".config", err.Error())
			releaseErrors++
		}
	}
	if releaseErrors == 0 {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "agent_releases", Status: "passed", Message: "selected Agent releases, signatures, artifacts and dependencies are verified"})
	} else {
		preview.Checks = append(preview.Checks, TopologyDeploymentDiagnosticCheck{Name: "agent_releases", Status: "failed", Message: "one or more Agent release checks failed"})
	}

	// A canary cannot select a dependent vertex without selecting its source.
	if input.RolloutGroup != "" {
		for _, edge := range edges {
			if selected[edge.TargetKey] && !selected[edge.SourceKey] {
				appendIssue("rollout_dependency_missing", "edges", fmt.Sprintf("rollout group %q selects vertex %q but not its dependency %q", input.RolloutGroup, edge.TargetKey, edge.SourceKey))
			}
		}
	}
	if input.FailurePolicy != "stop_and_rollback" {
		appendIssue("unsupported_failure_policy", "failure_policy", "topology deployment failure_policy must be stop_and_rollback")
	}

	// Build preview steps from the same selected graph shape as the durable
	// compiler. Missing assignments/releases still produce a visible step with
	// an empty target version, while issues explain why apply is blocked.
	previous, previousErr := loadPreviewPreviousVertices(db, topology, input.RolloutGroup)
	if previousErr != nil {
		return nil, previousErr
	}
	selectedKeys := make(map[string]struct{})
	for _, key := range order {
		if !selected[key] {
			continue
		}
		vertex := findTopologyVertex(vertices, key)
		assignment := assignmentByKey[key]
		config := canonicalTopologyConfig(vertex.ConfigJSON)
		step := TopologyDeploymentPreviewStep{
			Order: len(preview.Steps) + 1, VertexID: vertex.ID, VertexKey: vertex.Key,
			NodeID: valueOrZero(vertex.NodeID), PluginID: vertex.PluginID, Role: vertex.Role,
			TargetVersion: assignment.DesiredVersion, ApplyAction: topologyApplyActionConfigureEnable,
			RollbackMode: topologyRollbackModeDisable, Operations: []string{"plugin.configure", "plugin.enable"},
			ConfigHash: configHash(config), Dependencies: topologyPreviewDependencies(vertex.Key, edges, selected),
		}
		pluginKey := topologyDeploymentPluginKey(step.NodeID, step.PluginID, step.Role)
		if _, exists := previous[pluginKey]; exists {
			step.RollbackMode = topologyRollbackModeRestore
		}
		preview.Steps = append(preview.Steps, step)
		selectedKeys[pluginKey] = struct{}{}
	}
	removed := make([]string, 0)
	for key := range previous {
		if _, retained := selectedKeys[key]; !retained {
			removed = append(removed, key)
		}
	}
	sort.Strings(removed)
	for _, key := range removed {
		vertex := previous[key]
		assignment := assignmentByKey[vertex.Key]
		if assignment.ID == 0 {
			// Previous vertices may not occur in the new revision. Resolve their
			// assignment separately so removals remain previewable.
			_ = db.Where("node_id = ? AND service_scope = ? AND plugin_id = ? AND role = ? AND enabled = ?",
				valueOrZero(vertex.NodeID), topology.ServiceScope, vertex.PluginID, vertex.Role, true).First(&assignment).Error
		}
		config := canonicalTopologyConfig(vertex.ConfigJSON)
		preview.Steps = append(preview.Steps, TopologyDeploymentPreviewStep{
			Order: len(preview.Steps) + 1, VertexID: vertex.ID, VertexKey: vertex.Key,
			NodeID: valueOrZero(vertex.NodeID), PluginID: vertex.PluginID, Role: vertex.Role,
			TargetVersion: assignment.DesiredVersion, Removal: true, ApplyAction: topologyApplyActionDisable,
			RollbackMode: topologyRollbackModeRestore, Operations: []string{"plugin.disable"}, ConfigHash: configHash(config),
		})
	}

	preview.Valid = len(preview.Issues) == 0 && len(preview.Steps) > 0
	if len(preview.Steps) == 0 {
		appendIssue("no_deployable_steps", "steps", "topology revision has no deployable plugin vertices for rollout group")
		preview.Valid = false
	}
	return preview, nil
}

func diagnoseTopologyRelease(db *gorm.DB, pluginID, version string) releaseResultCompat {
	var plugin model.Plugin
	if err := db.First(&plugin, "id = ?", pluginID).Error; err != nil {
		return releaseResultCompat{err: fmt.Errorf("official plugin catalog entry is missing: %w", err)}
	}
	if !plugin.Official || plugin.Publisher != "AnixOps" {
		return releaseResultCompat{err: errors.New("plugin catalog entry is not an official AnixOps plugin")}
	}
	var release model.PluginRelease
	if err := db.First(&release, "plugin_id = ? AND version = ?", pluginID, version).Error; err != nil {
		return releaseResultCompat{err: fmt.Errorf("desired Agent release is not registered: %w", err)}
	}
	if err := ValidatePluginReleaseTarget(release, "agent"); err != nil {
		return releaseResultCompat{err: fmt.Errorf("agent release is not compatible: %w", err)}
	}
	manifest, err := VerifyStoredPluginRelease(db, release, nil)
	if err != nil {
		return releaseResultCompat{err: fmt.Errorf("agent release signature is invalid: %w", err)}
	}
	if err := requireVerifiedPluginArtifact(db, release); err != nil {
		return releaseResultCompat{err: fmt.Errorf("agent artifact is not verified: %w", err)}
	}
	plan, err := ResolvePluginDependencyExecutionPlan(db, release, "agent")
	if err != nil {
		return releaseResultCompat{err: fmt.Errorf("dependency graph is invalid: %w", err)}
	}
	for _, dependency := range plan.Steps {
		var dependencyPlugin model.Plugin
		if err := db.First(&dependencyPlugin, "id = ?", dependency.PluginID).Error; err != nil {
			return releaseResultCompat{err: fmt.Errorf("dependency %s official plugin catalog entry is missing: %w", dependency.PluginID, err)}
		}
		if !dependencyPlugin.Official || dependencyPlugin.Publisher != "AnixOps" {
			return releaseResultCompat{err: fmt.Errorf("dependency %s is not an official AnixOps plugin", dependency.PluginID)}
		}
		if _, err := VerifyStoredPluginRelease(db, dependency.Release, nil); err != nil {
			return releaseResultCompat{err: fmt.Errorf("dependency %s signature is invalid: %w", dependency.PluginID, err)}
		}
	}
	if err := ValidatePluginDependencyExecutionPlan(db, plan); err != nil {
		return releaseResultCompat{err: fmt.Errorf("dependency artifacts or conflicts are invalid: %w", err)}
	}
	return releaseResultCompat{manifest: manifest}
}

type releaseResultCompat struct {
	manifest *PluginManifest
	err      error
}

func classifyTopologyReleaseError(err error) (string, string) {
	message := err.Error()
	switch {
	case strings.Contains(message, "not an official") || strings.Contains(message, "official plugin catalog"):
		return "plugin_not_official", message
	case strings.Contains(message, "dependency"):
		return "dependency_invalid", message
	case strings.Contains(message, "not registered"):
		return "release_missing", message
	case strings.Contains(message, "signature"):
		return "release_signature_invalid", message
	case strings.Contains(message, "artifact"):
		return "artifact_unverified", message
	case strings.Contains(message, "dependency") || strings.Contains(message, "conflict"):
		return "dependency_invalid", message
	default:
		return "release_invalid", message
	}
}

func loadPreviewPreviousVertices(db *gorm.DB, topology model.Topology, rolloutGroup string) (map[string]model.TopologyVertex, error) {
	previous := make(map[string]model.TopologyVertex)
	if topology.ActiveRevisionID == nil {
		return previous, nil
	}
	var vertices []model.TopologyVertex
	if err := db.Where("revision_id = ?", *topology.ActiveRevisionID).Order("key").Find(&vertices).Error; err != nil {
		return nil, err
	}
	for _, vertex := range vertices {
		if vertex.NodeID == nil || strings.TrimSpace(vertex.PluginID) == "" {
			continue
		}
		var assignment model.NodeServiceAssignment
		err := db.Where("node_id = ? AND service_scope = ? AND plugin_id = ? AND role = ? AND enabled = ?",
			*vertex.NodeID, topology.ServiceScope, vertex.PluginID, vertex.Role, true).First(&assignment).Error
		if err != nil {
			continue
		}
		if rolloutGroup != "" && strings.TrimSpace(assignment.RolloutGroup) != rolloutGroup {
			continue
		}
		previous[topologyDeploymentPluginKey(*vertex.NodeID, vertex.PluginID, vertex.Role)] = vertex
	}
	return previous, nil
}

func findTopologyVertex(vertices []model.TopologyVertex, key string) model.TopologyVertex {
	for _, vertex := range vertices {
		if vertex.Key == key {
			return vertex
		}
	}
	return model.TopologyVertex{Key: key}
}

func vertexIndex(vertices []model.TopologyVertex, key string) int {
	for index, vertex := range vertices {
		if vertex.Key == key {
			return index
		}
	}
	return 0
}

func valueOrZero(value *uint) uint {
	if value == nil {
		return 0
	}
	return *value
}

func canonicalTopologyConfig(raw string) string {
	canonical, err := CanonicalKernelOperationConfig(raw)
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return canonical
}

func configHash(raw string) string {
	hash, err := HashKernelOperationConfig(raw)
	if err != nil {
		return ""
	}
	return hash
}

func topologyPreviewDependencies(key string, edges []model.TopologyEdge, selected map[string]bool) []string {
	result := make([]string, 0)
	for _, edge := range edges {
		if edge.TargetKey == key && selected[edge.SourceKey] {
			result = append(result, edge.SourceKey)
		}
	}
	sort.Strings(result)
	return result
}

// validateTopologyRuntimeConstraints covers package-specific nested config
// fields without coupling the kernel to one plugin's full schema.
func validateTopologyRuntimeConstraints(vertices []model.TopologyVertex, edges []model.TopologyEdge) []TopologyValidationIssue {
	issues := make([]TopologyValidationIssue, 0)
	ports := make(map[string]string)
	for index, vertex := range vertices {
		path := fmt.Sprintf("vertices[%d].config", index)
		var value any
		if strings.TrimSpace(vertex.ConfigJSON) == "" {
			continue
		}
		if err := json.Unmarshal([]byte(vertex.ConfigJSON), &value); err != nil {
			continue
		}
		inspectTopologyRuntimeValue(value, path, vertex.NodeID, ports, &issues)
	}
	for index, edge := range edges {
		path := fmt.Sprintf("edges[%d]", index)
		if strings.TrimSpace(edge.SecretID) == "" && topologySecureProtocol(edge.Protocol) {
			issues = append(issues, TopologyValidationIssue{"secret_required", path + ".secret_id", "secure protocol requires a secret reference"})
		}
		if strings.TrimSpace(edge.SecretID) != "" && strings.ContainsAny(edge.SecretID, "\r\n") {
			issues = append(issues, TopologyValidationIssue{"secret_reference_invalid", path + ".secret_id", "secret reference must not contain control characters"})
		}
		var value any
		if strings.TrimSpace(edge.ConfigJSON) != "" && json.Unmarshal([]byte(edge.ConfigJSON), &value) == nil {
			inspectTopologyRuntimeValue(value, path+".config", nil, ports, &issues)
		}
	}
	return issues
}

func inspectTopologyRuntimeValue(value any, path string, nodeID *uint, ports map[string]string, issues *[]TopologyValidationIssue) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			childPath := path + "." + key
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			switch {
			case normalized == "address_family":
				if text, ok := child.(string); ok && !validTopologyAddressFamily(text) {
					*issues = append(*issues, TopologyValidationIssue{"invalid_address_family", childPath, "address_family must be ipv4, ipv6 or dual"})
				}
			case normalized == "mtu":
				if number, ok := topologyNumber(child); ok && (number < 576 || number > 9000) {
					*issues = append(*issues, TopologyValidationIssue{"invalid_mtu", childPath, "MTU must be between 576 and 9000"})
				}
			case normalized == "port" || strings.HasSuffix(normalized, "_port"):
				number, ok := topologyNumber(child)
				if !ok || number < 1 || number > 65535 {
					*issues = append(*issues, TopologyValidationIssue{"invalid_port", childPath, "port must be between 1 and 65535"})
				} else if nodeID != nil && normalized != "target_port" {
					protocol := topologyStringField(typed, "protocol")
					if protocol == "" {
						protocol = normalized
					}
					for _, transport := range topologyPortTransports(protocol) {
						portKey := fmt.Sprintf("%d/%s/%d", *nodeID, transport, number)
						if previous, exists := ports[portKey]; exists && previous != path {
							*issues = append(*issues, TopologyValidationIssue{"port_conflict", childPath, "port conflicts with " + previous})
						} else {
							ports[portKey] = path
						}
					}
				}
			}
			inspectTopologyRuntimeValue(child, childPath, nodeID, ports, issues)
		}
	case []any:
		for index, child := range typed {
			inspectTopologyRuntimeValue(child, fmt.Sprintf("%s[%d]", path, index), nodeID, ports, issues)
		}
	}
}

func topologyNumber(value any) (int, bool) {
	switch typed := value.(type) {
	case float64:
		return int(typed), typed == float64(int(typed))
	case json.Number:
		parsed, err := typed.Int64()
		return int(parsed), err == nil
	default:
		return 0, false
	}
}

func topologyStringField(value map[string]any, key string) string {
	for candidate, child := range value {
		if strings.EqualFold(strings.ReplaceAll(candidate, "-", "_"), key) {
			if text, ok := child.(string); ok {
				return text
			}
		}
	}
	return ""
}

func topologySecureProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "wss", "tls", "tuic", "quic", "https", "grpc+tls":
		return true
	default:
		return false
	}
}

// validateNftablesForwardTopologyConfig mirrors the Agent runtime's
// ParseConfig/Config.Validate contract. The Control manifest schema is
// intentionally generic, so this package-specific gate prevents a topology
// from reaching the Agent only to fail during configure.
func validateNftablesForwardTopologyConfig(raw string, basePath string) []TopologyValidationIssue {
	issues := make([]TopologyValidationIssue, 0)
	add := func(code, path, message string) {
		issues = append(issues, TopologyValidationIssue{code, path, message})
	}
	type rawRule struct {
		ID            string `json:"id"`
		Protocol      string `json:"protocol"`
		ListenAddress string `json:"listen_address"`
		ListenPort    int    `json:"listen_port"`
		TargetAddress string `json:"target_address"`
		TargetPort    int    `json:"target_port"`
		Comment       string `json:"comment"`
	}
	type rawConfig struct {
		Apply          *bool           `json:"apply"`
		NftBinary      string          `json:"nft_binary"`
		PlanPath       string          `json:"plan_path"`
		Family         string          `json:"family"`
		Table          string          `json:"table"`
		Chain          string          `json:"chain"`
		Priority       *int            `json:"priority"`
		RollbackOnExit *bool           `json:"rollback_on_exit"`
		Rules          json.RawMessage `json:"rules"`
	}
	decoder := json.NewDecoder(bytes.NewReader([]byte(strings.TrimSpace(raw))))
	decoder.DisallowUnknownFields()
	var config *rawConfig
	if err := decoder.Decode(&config); err != nil {
		add("nftables_config_invalid", basePath, "nftables-forward config must be one JSON object: "+err.Error())
		return issues
	}
	if config == nil {
		add("nftables_config_invalid", basePath, "nftables-forward config must be a JSON object")
		return issues
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		add("nftables_config_invalid", basePath, "nftables-forward config must contain exactly one JSON object")
	}

	apply := config.Apply != nil && *config.Apply
	rollbackOnExit := config.RollbackOnExit == nil || *config.RollbackOnExit
	if !rollbackOnExit {
		add("nftables_rollback_required", basePath+".rollback_on_exit", "rollback_on_exit must remain enabled for the crash-safe lifecycle")
	}
	family := strings.TrimSpace(config.Family)
	if family == "" {
		family = "inet"
	}
	if family != "inet" && family != "ip" && family != "ip6" {
		add("nftables_family_invalid", basePath+".family", "family must be inet, ip, or ip6")
	}
	table := strings.TrimSpace(config.Table)
	if table == "" {
		table = "anixops_forward"
	}
	if !safeNftablesForwardIdentifier(table) {
		add("nftables_table_invalid", basePath+".table", "table must be a safe nftables identifier")
	}
	chain := strings.TrimSpace(config.Chain)
	if chain == "" {
		chain = "prerouting"
	}
	if !safeNftablesForwardIdentifier(chain) {
		add("nftables_chain_invalid", basePath+".chain", "chain must be a safe nftables identifier")
	}
	priority := -100
	if config.Priority != nil {
		priority = *config.Priority
	}
	if priority < -500 || priority > 500 {
		add("nftables_priority_invalid", basePath+".priority", "priority must be between -500 and 500")
	}
	if strings.ContainsAny(config.NftBinary, "\x00\r\n") {
		add("nftables_binary_invalid", basePath+".nft_binary", "nft_binary must not contain control characters")
	}
	planPath := strings.TrimSpace(config.PlanPath)
	if planPath != "" && (!filepath.IsAbs(planPath) || filepath.Clean(planPath) != planPath) {
		add("nftables_plan_path_invalid", basePath+".plan_path", "plan_path must be absolute and canonical")
	}
	if len(config.Rules) == 0 || string(bytes.TrimSpace(config.Rules)) == "null" {
		add("nftables_rules_required", basePath+".rules", "rules must be declared")
		if apply {
			add("nftables_apply_requires_rules", basePath+".rules", "apply=true requires at least one forwarding rule")
		}
		return issues
	}
	var rules []rawRule
	ruleDecoder := json.NewDecoder(bytes.NewReader(config.Rules))
	ruleDecoder.DisallowUnknownFields()
	if err := ruleDecoder.Decode(&rules); err != nil {
		add("nftables_rules_invalid", basePath+".rules", "unable to decode forwarding rules: "+err.Error())
		return issues
	}
	if len(rules) > 1024 {
		add("nftables_rules_limit", basePath+".rules", "forwarding rules exceed 1024 entries")
	}
	if apply && len(rules) == 0 {
		add("nftables_apply_requires_rules", basePath+".rules", "apply=true requires at least one forwarding rule")
	}
	seenIDs := make(map[string]struct{}, len(rules))
	for index, rule := range rules {
		path := fmt.Sprintf("%s.rules[%d]", basePath, index)
		id := strings.TrimSpace(rule.ID)
		if !safeNftablesForwardRuleID(id) {
			add("nftables_rule_id_invalid", path+".id", "rule id is invalid")
		} else if _, exists := seenIDs[id]; exists {
			add("nftables_rule_id_duplicate", path+".id", fmt.Sprintf("rule %q is duplicated", id))
		} else {
			seenIDs[id] = struct{}{}
		}
		protocol := strings.ToLower(strings.TrimSpace(rule.Protocol))
		if protocol != "tcp" && protocol != "udp" {
			add("nftables_rule_protocol_invalid", path+".protocol", "protocol must be tcp or udp")
		}
		if rule.ListenPort < 1 || rule.ListenPort > 65535 {
			add("nftables_rule_port_invalid", path+".listen_port", "listen_port must be between 1 and 65535")
		}
		if rule.TargetPort < 1 || rule.TargetPort > 65535 {
			add("nftables_rule_port_invalid", path+".target_port", "target_port must be between 1 and 65535")
		}
		listen, listenErr := netip.ParseAddr(strings.TrimSpace(rule.ListenAddress))
		target, targetErr := netip.ParseAddr(strings.TrimSpace(rule.TargetAddress))
		if listenErr != nil || listen.IsUnspecified() || listen.IsMulticast() {
			add("nftables_rule_address_invalid", path+".listen_address", "listen_address must be a unicast IP address")
		}
		if targetErr != nil || target.IsUnspecified() || target.IsMulticast() {
			add("nftables_rule_address_invalid", path+".target_address", "target_address must be a unicast IP address")
		}
		if listenErr == nil && targetErr == nil && listen.Is4() != target.Is4() {
			add("nftables_rule_address_family_mismatch", path, "listen_address and target_address must use the same IP family")
		}
		if (family == "ip" && listenErr == nil && !listen.Is4()) || (family == "ip6" && listenErr == nil && listen.Is4()) {
			add("nftables_family_rule_mismatch", path+".listen_address", "rule address family does not match nftables family")
		}
		if rule.Comment != "" && (len(rule.Comment) > 96 || strings.ContainsAny(rule.Comment, "\x00\r\n\"\\")) {
			add("nftables_rule_comment_invalid", path+".comment", "comment is invalid")
		}
	}
	return issues
}

func safeNftablesForwardIdentifier(value string) bool {
	if value == "" || len(value) > 63 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || char == '_' || (index > 0 && char >= '0' && char <= '9') {
			continue
		}
		return false
	}
	return true
}

func safeNftablesForwardRuleID(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_' || (index > 0 && char == '.') {
			continue
		}
		return false
	}
	return true
}
