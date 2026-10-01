package kernelnodeops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	kernelnodeopsv1 "github.com/AnixOps/anix-control/sdk/api/kernelnodeops/v1"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"gorm.io/gorm"
)

// The operation kinds (node-ops-service.md section 3.3): each is recorded
// with its operation and answered in Operation.kind.
const (
	KindForwardApply       = "forward.apply"
	KindForwardTunnel      = "forward.tunnel"
	KindForwardSyncBackend = "forward.sync_backend"
	KindForwardLegacyRule  = "forward.legacy_rule"
	KindNodeSync           = "node.sync"
	KindNodeRetire         = "node.retire"
	KindProtocolRetire     = "protocol.retire"
	KindSecretsPut         = "secrets.put"
	KindDiagnoseEndpoints  = "diagnose.endpoints"
	KindDiagnoseNodeStats  = "diagnose.node_stats"
	KindDiagnoseForward    = "diagnose.forward"
	KindDiagnoseTunnel     = "diagnose.tunnel"
	KindAgentDiagnostic    = "agent.diagnostic"
	KindAgentOperation     = "agent.operation"
	KindCredentialIssue    = "credential.issue"  // #nosec G101 -- an operation kind name, not a credential.
	KindCredentialRevoke   = "credential.revoke" // #nosec G101 -- an operation kind name, not a credential.
	KindRegKeyIssue        = "regkey.issue"
	KindRegKeyRevoke       = "regkey.revoke"
	KindCleanAgentIssue    = "cleanagent.issue"
)

// SealedPrefix starts a sealed secret handle (section 3.7).
const SealedPrefix = "anix-sealed:v1:"

// Bounds of an operation's fields.
const (
	maxID            = math.MaxUint32
	maxReasons       = 16
	maxReasonBytes   = 64
	maxNameBytes     = 100
	maxDocumentBytes = 256 << 10
	maxPayloadBytes  = 64 << 10
	maxActionBytes   = 64
	maxHandleBytes   = 256
	maxCheckNodes    = 256
	maxTimeout       = 3600
	maxAgentOpID     = 128
)

// families maps each operation family to its name in the ledger and its
// capability.
var families = map[kernelnodeopsv1.OperationFamily]struct{ name, capability string }{
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_FORWARD:     {"forward", service.CapabilityNodeOpsForward},
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG: {"nodeconfig", service.CapabilityNodeOpsNodeConfig},
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE:    {"diagnose", service.CapabilityNodeOpsDiagnose},
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_AGENTS:      {"agents", service.CapabilityNodeOpsAgents},
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS: {"credentials", service.CapabilityNodeOpsCredentials},
}

// familyOrder lists the families as the contract's enum orders them.
var familyOrder = []kernelnodeopsv1.OperationFamily{
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_FORWARD, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG,
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE, kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_AGENTS,
	kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS,
}

func familyByName(name string) kernelnodeopsv1.OperationFamily {
	for family, entry := range families {
		if entry.name == name {
			return family
		}
	}
	return kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_UNSPECIFIED
}

// kind is one operation kind: its family, how its fields are checked, how
// its targets are found, and how it relates to other operations on the same
// resource.
type kind struct {
	name   string
	family kernelnodeopsv1.OperationFamily
	// check validates the fields: INVALID_ARGUMENT, or PERMISSION_DENIED for
	// a target of a kind the operation does not take.
	check func(*kernelnodeopsv1.OperationSpec) error
	// resolve finds the targets and names the resource; NOT_FOUND when a
	// target does not exist.
	resolve func(*gorm.DB, *kernelnodeopsv1.OperationSpec) (resolution, error)
	// supersedes reports whether a newer operation of this kind replaces an
	// older one still pending on the same resource. Nil for kinds that are
	// not level-triggered: they never replace each other.
	supersedes func(newer, older *kernelnodeopsv1.OperationSpec) bool
	// fanOut kinds end by creating child operations and end when they have.
	fanOut bool
}

// levelTriggered kinds converge on the resource's current state, so running
// one again after an interruption is safe.
func (k *kind) levelTriggered() bool { return k.supersedes != nil }

type resolution struct {
	targets  []*kernelnodeopsv1.NodeRef
	resource string
}

var kinds = map[string]*kind{}

func register(k *kind) {
	if _, exists := kinds[k.name]; exists {
		panic("kernelnodeops: kind registered twice: " + k.name)
	}
	kinds[k.name] = k
}

// KnownKinds returns every operation kind of the contract, sorted.
func KnownKinds() []string {
	names := make([]string, 0, len(kinds))
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// FamilyOfKind returns a kind's family, or UNSPECIFIED for an unknown kind.
func FamilyOfKind(name string) kernelnodeopsv1.OperationFamily {
	if k, ok := kinds[name]; ok {
		return k.family
	}
	return kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_UNSPECIFIED
}

// errUnknownKind is an operation whose case this kernel does not know: a
// kind added to the contract after this kernel was built.
var errUnknownKind = errors.New("operation kind is not known to this kernel")

// kindOf returns the kind of spec. errUnknownKind when the operation is a
// case this kernel does not know; INVALID_ARGUMENT when there is none.
func kindOf(spec *kernelnodeopsv1.OperationSpec) (*kind, error) {
	var name string
	switch spec.GetOperation().(type) {
	case *kernelnodeopsv1.OperationSpec_ApplyForward:
		name = KindForwardApply
	case *kernelnodeopsv1.OperationSpec_ApplyTunnel:
		name = KindForwardTunnel
	case *kernelnodeopsv1.OperationSpec_SyncForwardBackend:
		name = KindForwardSyncBackend
	case *kernelnodeopsv1.OperationSpec_ApplyLegacyRule:
		name = KindForwardLegacyRule
	case *kernelnodeopsv1.OperationSpec_SyncNode:
		name = KindNodeSync
	case *kernelnodeopsv1.OperationSpec_RetireNode:
		name = KindNodeRetire
	case *kernelnodeopsv1.OperationSpec_RetireProtocol:
		name = KindProtocolRetire
	case *kernelnodeopsv1.OperationSpec_PutSecretDocument:
		name = KindSecretsPut
	case *kernelnodeopsv1.OperationSpec_CheckEndpoints:
		name = KindDiagnoseEndpoints
	case *kernelnodeopsv1.OperationSpec_CollectNodeStats:
		name = KindDiagnoseNodeStats
	case *kernelnodeopsv1.OperationSpec_DiagnoseForward:
		name = KindDiagnoseForward
	case *kernelnodeopsv1.OperationSpec_DiagnoseTunnel:
		name = KindDiagnoseTunnel
	case *kernelnodeopsv1.OperationSpec_RunAgentDiagnostic:
		name = KindAgentDiagnostic
	case *kernelnodeopsv1.OperationSpec_AgentControlOperation:
		name = KindAgentOperation
	case *kernelnodeopsv1.OperationSpec_IssueCredential:
		name = KindCredentialIssue
	case *kernelnodeopsv1.OperationSpec_RevokeCredential:
		name = KindCredentialRevoke
	case *kernelnodeopsv1.OperationSpec_IssueRegistrationKey:
		name = KindRegKeyIssue
	case *kernelnodeopsv1.OperationSpec_RevokeRegistrationKey:
		name = KindRegKeyRevoke
	case *kernelnodeopsv1.OperationSpec_IssueCleanAgent:
		name = KindCleanAgentIssue
	case nil:
		if spec != nil && len(spec.ProtoReflect().GetUnknown()) > 0 {
			return nil, errUnknownKind
		}
		return nil, status.Error(codes.InvalidArgument, "operation is required")
	}
	k := kinds[name]
	if k == nil {
		return nil, errUnknownKind
	}
	return k, nil
}

// Node kinds as the ledger names them.
const (
	nodeKindProxy      = "proxy"
	nodeKindForward    = "forward"
	nodeKindCleanAgent = "clean_agent"
)

var nodeKindNames = map[kernelnodeopsv1.NodeKind]string{
	kernelnodeopsv1.NodeKind_NODE_KIND_PROXY:       nodeKindProxy,
	kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD:     nodeKindForward,
	kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT: nodeKindCleanAgent,
}

// nodeTables are the tables each node kind lives in.
var nodeTables = map[kernelnodeopsv1.NodeKind]string{
	kernelnodeopsv1.NodeKind_NODE_KIND_PROXY:       "v2_node",
	kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD:     "v2_forward_node",
	kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT: "v2_forward_clean_agent",
}

func nodeKindByName(name string) kernelnodeopsv1.NodeKind {
	for kind, known := range nodeKindNames {
		if known == name {
			return kind
		}
	}
	return kernelnodeopsv1.NodeKind_NODE_KIND_UNSPECIFIED
}

func nodeRef(kind kernelnodeopsv1.NodeKind, id uint64) *kernelnodeopsv1.NodeRef {
	return &kernelnodeopsv1.NodeRef{Kind: kind, Id: id}
}

// nodeKey is a node's name in resource keys: "proxy-3", "forward-7".
func nodeKey(ref *kernelnodeopsv1.NodeRef) string {
	return nodeKindNames[ref.GetKind()] + "-" + strconv.FormatUint(ref.GetId(), 10)
}

func invalid(format string, args ...any) error {
	return status.Errorf(codes.InvalidArgument, format, args...)
}

// checkID bounds a row id: v2 ids are 32-bit.
func checkID(value uint64, field string) error {
	if value == 0 || value > maxID {
		return invalid("%s is required and at most %d", field, uint64(maxID))
	}
	return nil
}

// checkNode validates a node reference and refuses node kinds the
// operation does not take: a family cannot reach another domain's nodes.
func checkNode(ref *kernelnodeopsv1.NodeRef, field, kindName string, allowed ...kernelnodeopsv1.NodeKind) error {
	if ref == nil || ref.GetKind() == kernelnodeopsv1.NodeKind_NODE_KIND_UNSPECIFIED {
		return invalid("%s with a node kind is required", field)
	}
	if _, known := nodeKindNames[ref.GetKind()]; !known {
		return invalid("%s has an unknown node kind", field)
	}
	if err := checkID(ref.GetId(), field+".id"); err != nil {
		return err
	}
	for _, kind := range allowed {
		if ref.GetKind() == kind {
			return nil
		}
	}
	return status.Errorf(codes.PermissionDenied, "%s does not take %s nodes", kindName, nodeKindNames[ref.GetKind()])
}

// checkText bounds a text field: valid UTF-8 without control characters.
func checkText(value, field string, maxBytes int, required bool) error {
	if required && strings.TrimSpace(value) == "" {
		return invalid("%s is required", field)
	}
	if len(value) > maxBytes || !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		return invalid("%s must be at most %d bytes of text", field, maxBytes)
	}
	return nil
}

func checkReasons(reasons []string) error {
	if len(reasons) > maxReasons {
		return invalid("at most %d reasons", maxReasons)
	}
	for _, reason := range reasons {
		if err := checkText(reason, "reasons", maxReasonBytes, true); err != nil {
			return err
		}
	}
	return nil
}

// checkJSON accepts an empty value or one JSON document of at most maxBytes.
func checkJSON(raw []byte, field string, maxBytes int) error {
	if len(raw) > maxBytes {
		return invalid("%s is larger than %d bytes", field, maxBytes)
	}
	if len(raw) > 0 && !json.Valid(raw) {
		return invalid("%s is not JSON", field)
	}
	return nil
}

func checkTimeout(seconds uint32) error {
	if seconds > maxTimeout {
		return invalid("timeout_seconds is at most %d", maxTimeout)
	}
	return nil
}

func checkForwardAction(action kernelnodeopsv1.ForwardAction) error {
	if action == kernelnodeopsv1.ForwardAction_FORWARD_ACTION_UNSPECIFIED {
		return invalid("action is required")
	}
	if _, known := kernelnodeopsv1.ForwardAction_name[int32(action)]; !known {
		return invalid("action is unknown")
	}
	return nil
}

func checkVantage(vantage kernelnodeopsv1.Vantage) error {
	if _, known := kernelnodeopsv1.Vantage_name[int32(vantage)]; !known {
		return invalid("vantage is unknown")
	}
	return nil
}

func deletes(action kernelnodeopsv1.ForwardAction) bool {
	return action == kernelnodeopsv1.ForwardAction_FORWARD_ACTION_DELETE ||
		action == kernelnodeopsv1.ForwardAction_FORWARD_ACTION_FORCE_DELETE
}

// supersedesForwardAction: a newer change replaces a pending one, except
// that only a deletion replaces a pending deletion, whose captured state
// must still be removed.
func supersedesForwardAction(newer, older kernelnodeopsv1.ForwardAction) bool {
	return !deletes(older) || deletes(newer)
}

// exists requires the row id of table, by id only: credential columns are
// never read.
func exists(db *gorm.DB, table string, id uint64, what string) error {
	var count int64
	if err := db.Table(table).Where("id = ?", id).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return status.Errorf(codes.NotFound, "%s %d not found", what, id)
	}
	return nil
}

func nodeExists(db *gorm.DB, ref *kernelnodeopsv1.NodeRef) error {
	return exists(db, nodeTables[ref.GetKind()], ref.GetId(), strings.ReplaceAll(nodeKindNames[ref.GetKind()], "_", " ")+" node")
}

// tunnelTargets finds a tunnel and its entry and exit forward nodes.
func tunnelTargets(db *gorm.DB, tunnelID uint64) ([]*kernelnodeopsv1.NodeRef, error) {
	var tunnels []struct {
		InNodeID  uint64
		OutNodeID *uint64
	}
	if err := db.Table("v2_forward_tunnel").Select("in_node_id, out_node_id").Where("id = ?", tunnelID).Limit(1).Find(&tunnels).Error; err != nil {
		return nil, err
	}
	if len(tunnels) == 0 {
		return nil, status.Errorf(codes.NotFound, "tunnel %d not found", tunnelID)
	}
	return forwardNodes(tunnels[0].InNodeID, tunnels[0].OutNodeID), nil
}

func forwardNodes(entry uint64, exit *uint64) []*kernelnodeopsv1.NodeRef {
	var targets []*kernelnodeopsv1.NodeRef
	if entry != 0 {
		targets = append(targets, nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, entry))
	}
	if exit != nil && *exit != 0 && *exit != entry {
		targets = append(targets, nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, *exit))
	}
	return targets
}

// forwardTargets finds a panel forward and its tunnel's nodes.
func forwardTargets(db *gorm.DB, forwardID uint64) ([]*kernelnodeopsv1.NodeRef, error) {
	var forwards []struct{ TunnelID uint64 }
	if err := db.Table("v2_forward").Select("tunnel_id").Where("id = ?", forwardID).Limit(1).Find(&forwards).Error; err != nil {
		return nil, err
	}
	if len(forwards) == 0 {
		return nil, status.Errorf(codes.NotFound, "forward %d not found", forwardID)
	}
	targets, err := tunnelTargets(db, forwards[0].TunnelID)
	if status.Code(err) == codes.NotFound {
		// A forward whose tunnel is gone still names itself.
		return nil, nil
	}
	return targets, err
}

func protocolNode(db *gorm.DB, protocolID uint64) (*kernelnodeopsv1.NodeRef, error) {
	var protocols []struct{ NodeID uint64 }
	if err := db.Table("v2_node_protocol").Select("node_id").Where("id = ?", protocolID).Limit(1).Find(&protocols).Error; err != nil {
		return nil, err
	}
	if len(protocols) == 0 {
		return nil, status.Errorf(codes.NotFound, "protocol %d not found", protocolID)
	}
	return nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, protocols[0].NodeID), nil
}

func singleNode(db *gorm.DB, ref *kernelnodeopsv1.NodeRef, resource string) (resolution, error) {
	if err := nodeExists(db, ref); err != nil {
		return resolution{}, err
	}
	return resolution{targets: []*kernelnodeopsv1.NodeRef{nodeRef(ref.GetKind(), ref.GetId())}, resource: resource}, nil
}

func proxyNode(db *gorm.DB, id uint64, resource string) (resolution, error) {
	return singleNode(db, nodeRef(kernelnodeopsv1.NodeKind_NODE_KIND_PROXY, id), resource)
}

// agentOperationKinds are the Agent Control operations a package may send.
var agentOperationKinds = map[string]bool{"agent.ping": true, "node.reload": true, "users.reload": true}

// protocolSecretColumns are the v2_node_protocol columns that hold secrets.
var protocolSecretColumns = map[string]bool{
	"settings": true, "tls_settings": true, "transport_settings": true, "reality_settings": true, "custom_config": true,
}

// credentialKinds are the credentials each node kind holds; UNSPECIFIED
// names all of them.
var credentialKinds = map[kernelnodeopsv1.NodeKind][]kernelnodeopsv1.CredentialKind{
	kernelnodeopsv1.NodeKind_NODE_KIND_PROXY: {
		kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_API_KEY, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_NODE_SHARED_SECRET,
		kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT,
	},
	kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD: {
		kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_FORWARD_NODE_TOKEN, kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT,
	},
	kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT: {kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_CLEAN_AGENT_TOKEN},
}

func checkCredential(subject *kernelnodeopsv1.NodeRef, credential kernelnodeopsv1.CredentialKind, kindName string) error {
	if err := checkNode(subject, "subject", kindName, kernelnodeopsv1.NodeKind_NODE_KIND_PROXY,
		kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD, kernelnodeopsv1.NodeKind_NODE_KIND_CLEAN_AGENT); err != nil {
		return err
	}
	if credential == kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED {
		return nil
	}
	if _, known := kernelnodeopsv1.CredentialKind_name[int32(credential)]; !known {
		return invalid("kind is an unknown credential kind")
	}
	for _, held := range credentialKinds[subject.GetKind()] {
		if held == credential {
			return nil
		}
	}
	return invalid("a %s node holds no %s", nodeKindNames[subject.GetKind()], credential)
}

func checkHandle(handle, field string) error {
	if !strings.HasPrefix(handle, SealedPrefix) || len(handle) > maxHandleBytes || len(handle) == len(SealedPrefix) {
		return invalid("%s must be a sealed handle (%s...)", field, SealedPrefix)
	}
	return checkText(handle, field, maxHandleBytes, true)
}

// checkSecretDocument requires a secret document's secret values to be
// sealed handles, the placeholder (keep the stored value) or empty: a
// package never sends a secret in clear.
func checkSecretDocument(raw []byte) error {
	if len(raw) == 0 {
		return invalid("document_json is required")
	}
	if err := checkJSON(raw, "document_json", maxDocumentBytes); err != nil {
		return err
	}
	value, err := decodeJSON(raw)
	if err != nil {
		return invalid("document_json is not JSON")
	}
	return checkSecretValues(value, "")
}

func checkSecretValues(value any, key string) error {
	switch typed := value.(type) {
	case map[string]any:
		for child, item := range typed {
			if err := checkSecretValues(item, child); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := checkSecretValues(item, key); err != nil {
				return err
			}
		}
	case string:
		if key == "" || !service.IsNodeSecretKey(key) || typed == "" || typed == service.NodeSecretPlaceholder {
			return nil
		}
		if strings.HasPrefix(typed, SealedPrefix) {
			return checkHandle(typed, "document_json."+key)
		}
		return invalid("document_json carries a secret in clear at %q: send a sealed handle, the placeholder or an empty value", key)
	}
	return nil
}

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing data")
	}
	return value, nil
}

func init() {
	forward := kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_FORWARD
	nodeConfig := kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_NODE_CONFIG
	diagnose := kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_DIAGNOSE
	agents := kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_AGENTS
	credentials := kernelnodeopsv1.OperationFamily_OPERATION_FAMILY_CREDENTIALS
	proxy := kernelnodeopsv1.NodeKind_NODE_KIND_PROXY
	forwardNode := kernelnodeopsv1.NodeKind_NODE_KIND_FORWARD

	register(&kind{
		name: KindForwardApply, family: forward,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetApplyForward()
			if err := checkID(op.GetForwardId(), "forward_id"); err != nil {
				return err
			}
			return checkForwardAction(op.GetAction())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			id := spec.GetApplyForward().GetForwardId()
			targets, err := forwardTargets(db, id)
			return resolution{targets: targets, resource: fmt.Sprintf("forward:%d", id)}, err
		},
		supersedes: func(newer, older *kernelnodeopsv1.OperationSpec) bool {
			return supersedesForwardAction(newer.GetApplyForward().GetAction(), older.GetApplyForward().GetAction())
		},
	})
	register(&kind{
		name: KindForwardTunnel, family: forward, fanOut: true,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetApplyTunnel()
			if err := checkID(op.GetTunnelId(), "tunnel_id"); err != nil {
				return err
			}
			return checkReasons(op.GetReasons())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			id := spec.GetApplyTunnel().GetTunnelId()
			targets, err := tunnelTargets(db, id)
			return resolution{targets: targets, resource: fmt.Sprintf("tunnel:%d", id)}, err
		},
		supersedes: func(_, _ *kernelnodeopsv1.OperationSpec) bool { return true },
	})
	register(&kind{
		name: KindForwardSyncBackend, family: forward, fanOut: true,
		check: func(*kernelnodeopsv1.OperationSpec) error { return nil },
		resolve: func(*gorm.DB, *kernelnodeopsv1.OperationSpec) (resolution, error) {
			return resolution{resource: "forward-backend"}, nil
		},
		supersedes: func(_, _ *kernelnodeopsv1.OperationSpec) bool { return true },
	})
	register(&kind{
		name: KindForwardLegacyRule, family: forward,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetApplyLegacyRule()
			if err := checkID(op.GetRuleId(), "rule_id"); err != nil {
				return err
			}
			return checkForwardAction(op.GetAction())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			id := spec.GetApplyLegacyRule().GetRuleId()
			var rules []struct {
				RelayNodeID uint64
				ExitNodeID  uint64
			}
			if err := db.Table("v2_forward_rule").Select("relay_node_id, exit_node_id").Where("id = ?", id).Limit(1).Find(&rules).Error; err != nil {
				return resolution{}, err
			}
			if len(rules) == 0 {
				return resolution{}, status.Errorf(codes.NotFound, "legacy rule %d not found", id)
			}
			exit := rules[0].ExitNodeID
			return resolution{targets: forwardNodes(rules[0].RelayNodeID, &exit), resource: fmt.Sprintf("legacy-rule:%d", id)}, nil
		},
		supersedes: func(newer, older *kernelnodeopsv1.OperationSpec) bool {
			return supersedesForwardAction(newer.GetApplyLegacyRule().GetAction(), older.GetApplyLegacyRule().GetAction())
		},
	})

	register(&kind{
		name: KindNodeSync, family: nodeConfig,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetSyncNode()
			if err := checkNode(op.GetNode(), "node", KindNodeSync, proxy, forwardNode); err != nil {
				return err
			}
			return checkReasons(op.GetReasons())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			node := spec.GetSyncNode().GetNode()
			return singleNode(db, node, "nodeconfig:"+nodeKey(node))
		},
		// A newer sync replaces a pending one unless the pending one forces
		// a push and the newer does not.
		supersedes: func(newer, older *kernelnodeopsv1.OperationSpec) bool {
			return !older.GetSyncNode().GetForce() || newer.GetSyncNode().GetForce()
		},
	})
	register(&kind{
		name: KindNodeRetire, family: nodeConfig,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			return checkNode(spec.GetRetireNode().GetNode(), "node", KindNodeRetire, proxy, forwardNode)
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			node := spec.GetRetireNode().GetNode()
			return singleNode(db, node, "nodeconfig:"+nodeKey(node))
		},
	})
	register(&kind{
		name: KindProtocolRetire, family: nodeConfig,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			return checkID(spec.GetRetireProtocol().GetProtocolId(), "protocol_id")
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			id := spec.GetRetireProtocol().GetProtocolId()
			node, err := protocolNode(db, id)
			if err != nil {
				return resolution{}, err
			}
			return resolution{targets: []*kernelnodeopsv1.NodeRef{node}, resource: fmt.Sprintf("protocol:%d", id)}, nil
		},
	})
	register(&kind{
		name: KindSecretsPut, family: nodeConfig,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetPutSecretDocument()
			if err := checkID(op.GetOwnerId(), "owner_id"); err != nil {
				return err
			}
			switch op.GetScope() {
			case kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_PROTOCOL:
				if !protocolSecretColumns[op.GetColumn()] {
					return invalid("column must be one of settings, tls_settings, transport_settings, reality_settings, custom_config")
				}
			case kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG:
				if op.GetColumn() != "raw_config" {
					return invalid("column must be raw_config")
				}
			default:
				return invalid("scope is required")
			}
			return checkSecretDocument(op.GetDocumentJson())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			op := spec.GetPutSecretDocument()
			resource := fmt.Sprintf("secret:%s:%d:%s", strings.ToLower(strings.TrimPrefix(op.GetScope().String(), "SECRET_SCOPE_")), op.GetOwnerId(), op.GetColumn())
			if op.GetScope() == kernelnodeopsv1.SecretScope_SECRET_SCOPE_NODE_RAW_CONFIG {
				return proxyNode(db, op.GetOwnerId(), resource)
			}
			node, err := protocolNode(db, op.GetOwnerId())
			if err != nil {
				return resolution{}, err
			}
			return resolution{targets: []*kernelnodeopsv1.NodeRef{node}, resource: resource}, nil
		},
	})

	register(&kind{
		name: KindDiagnoseEndpoints, family: diagnose,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetCheckEndpoints()
			if len(op.GetNodes()) == 0 || len(op.GetNodes()) > maxCheckNodes {
				return invalid("between 1 and %d nodes are required", maxCheckNodes)
			}
			seen := map[uint64]bool{}
			for _, node := range op.GetNodes() {
				if err := checkNode(node, "nodes", KindDiagnoseEndpoints, forwardNode); err != nil {
					return err
				}
				if seen[node.GetId()] {
					return invalid("node %d is repeated", node.GetId())
				}
				seen[node.GetId()] = true
			}
			return checkVantage(op.GetVantage())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			var result resolution
			for _, node := range spec.GetCheckEndpoints().GetNodes() {
				if err := nodeExists(db, node); err != nil {
					return resolution{}, err
				}
				result.targets = append(result.targets, nodeRef(node.GetKind(), node.GetId()))
			}
			return result, nil
		},
	})
	register(&kind{
		name: KindDiagnoseNodeStats, family: diagnose,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			return checkNode(spec.GetCollectNodeStats().GetNode(), "node", KindDiagnoseNodeStats, forwardNode)
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			return singleNode(db, spec.GetCollectNodeStats().GetNode(), "")
		},
	})
	register(&kind{
		name: KindDiagnoseForward, family: diagnose,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetDiagnoseForward()
			if err := checkID(op.GetForwardId(), "forward_id"); err != nil {
				return err
			}
			return checkVantage(op.GetVantage())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			targets, err := forwardTargets(db, spec.GetDiagnoseForward().GetForwardId())
			return resolution{targets: targets}, err
		},
	})
	register(&kind{
		name: KindDiagnoseTunnel, family: diagnose,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetDiagnoseTunnel()
			if err := checkID(op.GetTunnelId(), "tunnel_id"); err != nil {
				return err
			}
			return checkVantage(op.GetVantage())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			targets, err := tunnelTargets(db, spec.GetDiagnoseTunnel().GetTunnelId())
			return resolution{targets: targets}, err
		},
	})
	register(&kind{
		name: KindAgentDiagnostic, family: diagnose,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetRunAgentDiagnostic()
			if err := checkID(op.GetNodeId(), "node_id"); err != nil {
				return err
			}
			if err := checkText(op.GetAction(), "action", maxActionBytes, true); err != nil {
				return err
			}
			if err := checkJSON(op.GetParamsJson(), "params_json", maxPayloadBytes); err != nil {
				return err
			}
			return checkTimeout(op.GetTimeoutSeconds())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			return proxyNode(db, spec.GetRunAgentDiagnostic().GetNodeId(), "")
		},
	})

	register(&kind{
		name: KindAgentOperation, family: agents,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetAgentControlOperation()
			if err := checkID(op.GetNodeId(), "node_id"); err != nil {
				return err
			}
			if !agentOperationKinds[op.GetKind()] {
				return invalid("kind must be agent.ping, node.reload or users.reload")
			}
			if err := checkJSON(op.GetPayloadJson(), "payload_json", maxPayloadBytes); err != nil {
				return err
			}
			if err := checkText(op.GetAgentOperationId(), "agent_operation_id", maxAgentOpID, false); err != nil {
				return err
			}
			return checkTimeout(op.GetTimeoutSeconds())
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			return proxyNode(db, spec.GetAgentControlOperation().GetNodeId(), "")
		},
	})

	register(&kind{
		name: KindCredentialIssue, family: credentials,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetIssueCredential()
			if err := checkCredential(op.GetSubject(), op.GetKind(), KindCredentialIssue); err != nil {
				return err
			}
			if op.GetValue() == nil {
				return nil
			}
			if op.GetKind() == kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_UNSPECIFIED && op.GetSubject().GetKind() == proxy {
				return invalid("one value cannot be every credential of a proxy node; name the credential kind")
			}
			if op.GetKind() == kernelnodeopsv1.CredentialKind_CREDENTIAL_KIND_AGENT_ENROLLMENT {
				return invalid("agent enrollment credentials are generated, never sent")
			}
			return checkHandle(op.GetValue().GetHandle(), "value.handle")
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			subject := spec.GetIssueCredential().GetSubject()
			return singleNode(db, subject, "credential:"+nodeKey(subject))
		},
	})
	register(&kind{
		name: KindCredentialRevoke, family: credentials,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetRevokeCredential()
			return checkCredential(op.GetSubject(), op.GetKind(), KindCredentialRevoke)
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			subject := spec.GetRevokeCredential().GetSubject()
			return singleNode(db, subject, "credential:"+nodeKey(subject))
		},
	})
	register(&kind{
		name: KindRegKeyIssue, family: credentials,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetIssueRegistrationKey()
			if err := checkText(op.GetName(), "name", maxNameBytes, false); err != nil {
				return err
			}
			if op.GetExpiresAtUnix() < 0 {
				return invalid("expires_at_unix must not be negative")
			}
			return nil
		},
		resolve: func(*gorm.DB, *kernelnodeopsv1.OperationSpec) (resolution, error) { return resolution{}, nil },
	})
	register(&kind{
		name: KindRegKeyRevoke, family: credentials,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			return checkID(spec.GetRevokeRegistrationKey().GetKeyId(), "key_id")
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			id := spec.GetRevokeRegistrationKey().GetKeyId()
			if err := exists(db, "v2_authorized_key", id, "registration key"); err != nil {
				return resolution{}, err
			}
			return resolution{resource: fmt.Sprintf("regkey:%d", id)}, nil
		},
	})
	register(&kind{
		name: KindCleanAgentIssue, family: credentials,
		check: func(spec *kernelnodeopsv1.OperationSpec) error {
			op := spec.GetIssueCleanAgent()
			if err := checkText(op.GetName(), "name", maxNameBytes, true); err != nil {
				return err
			}
			return checkID(op.GetForwardNodeId(), "forward_node_id")
		},
		resolve: func(db *gorm.DB, spec *kernelnodeopsv1.OperationSpec) (resolution, error) {
			return singleNode(db, nodeRef(forwardNode, spec.GetIssueCleanAgent().GetForwardNodeId()), "")
		},
	})
}
