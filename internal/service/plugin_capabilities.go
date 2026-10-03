package service

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/nodesecrets"
	"github.com/AnixOps/anix-control/v4/internal/packagestore"
	"gorm.io/gorm"
)

// Capabilities in the kernel. namespace grant kernel authority to a signed
// package, so only the forms below are accepted. Other namespaces are package
// vocabulary and only need a well-formed, unique name.
const (
	// CapabilityObservedState lets an Agent-target package report observed
	// runtime state.
	CapabilityObservedState = "kernel.observed-state"
	// CapabilityStorage leases a per-package database role and schema.
	CapabilityStorage = "kernel.storage.v1"
	// CapabilityIdentity lets the official identity package call the
	// KernelIdentity contract (subscribers, account projection, revocation).
	CapabilityIdentity = "kernel.identity.v1"
	// CapabilityOrderComplete lets an official package call
	// KernelOrder.CompleteOrderPayment (order-service.md): a paid payment
	// record marks its order paid and completes it.
	CapabilityOrderComplete = "kernel.order.complete.v1"
	// The KernelSubscriber method families (subscriber-service.md): each
	// lets an official package call one part of the subscriber contract.
	CapabilitySubscriberEntitlements = "kernel.subscriber.entitlements.v1"
	CapabilitySubscriberTraffic      = "kernel.subscriber.traffic.v1"
	CapabilitySubscriberCredentials  = "kernel.subscriber.credentials.v1"
	CapabilitySubscriberBalance      = "kernel.subscriber.balance.v1"
	CapabilitySubscriberDirectory    = "kernel.subscriber.directory.v1"
	// CapabilitySubscriberGroups edits one subscription group membership
	// (v2_user_subscription_group) at a time.
	CapabilitySubscriberGroups = "kernel.subscriber.groups.v1"
	// CapabilitySubscriberSummary reads one subscriber's cached
	// subscription summary (kernel-caches.md).
	CapabilitySubscriberSummary = "kernel.subscriber.summary.v1"
	// CapabilityTelemetryDashboard lets an official package call
	// KernelTelemetry.GetDashboard (kernel-caches.md): the administrator
	// dashboard's snapshot from the kernel's cache, with the online users
	// as a count.
	CapabilityTelemetryDashboard = "kernel.telemetry.dashboard.v1"
	// capabilityStorageAdoptPrefix grants the storage role access to an
	// existing kernel table, adopted in place: kernel.storage.adopt:<table>.
	capabilityStorageAdoptPrefix = "kernel.storage.adopt:"
	// capabilityViewPrefix grants read access to a kernel API view:
	// kernel.view:kapi_<name>_v<N>.
	capabilityViewPrefix = "kernel.view:"
	// capabilitySettingsPrefix starts the KernelSettings capabilities,
	// kernel.settings.<namespace>.<read|write|secrets>.v1
	// (settings-service.md); see SettingsCapability.
	capabilitySettingsPrefix = "kernel.settings."
)

var (
	capabilityNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9._:-]{0,127}$`)
	adoptableTablePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	kernelAPIViewPattern   = regexp.MustCompile(`^kapi_[a-z][a-z0-9_]*_v[1-9][0-9]*$`)
	settingsCapability     = regexp.MustCompile(`^kernel\.settings\.([a-z][a-z0-9-]*)\.(read|write|secrets)\.v1$`)
	protectedTablePrefixes = []string{"v2_user", "v3_kernel_", "v4_kernel_", "identity_", "kapi_", "pg_"}
	protectedTables        = map[string]bool{
		"v2_system_config": true, "v2_audit_log": true, "v2_operation_log": true,
		// Node credentials: v2_node holds each node's API key, key hash and
		// secret, which the kernel's node authentication checks, and
		// v2_authorized_key the registration keys that mint them. A
		// package that could read or write them could act as any node.
		"v2_node": true, "v2_authorized_key": true,
		// Node runtime secrets: v2_node_protocol holds each protocol's
		// Reality private key, WireGuard server private key and custom
		// configuration, and the kernel builds every node's configuration
		// (UniProxy, the gRPC node service) and every subscription from its
		// rows without validating them again: its protocol validator runs on
		// its own writes only. v2_wireguard_peer holds every user's
		// WireGuard private and preshared keys. A package that could read
		// them could impersonate the nodes, and one that could write
		// v2_node_protocol could push unvalidated configuration to every
		// node.
		"v2_node_protocol": true, "v2_wireguard_peer": true,
		// The node credential split (node-ops-service.md, section 4): the
		// credentials and protocol secrets moved out of the tables above,
		// in clear, and the split's state. The v4_kernel_ prefix protects
		// them already; they are named so that no future exception to the
		// prefix (a finalized table becoming adoptable) can reach them.
		"v4_kernel_node_credential": true, "v4_kernel_protocol_secret": true,
		"v4_kernel_node_secret_split": true,
		// The KernelNodeOps ledger (node-ops-service.md section 3.4): who
		// asked for which node operation, and its outcome. The v4_kernel_
		// prefix protects them too; they are listed so that the rule holds
		// if the prefix list ever changes.
		"v4_kernel_node_operation": true, "v4_kernel_node_operation_event": true,
		"v4_kernel_node_operation_target": true,
		// Agent PKI: v4_kernel_agent_enrollment holds the hashes of
		// one-time agent enrollment credentials and v4_kernel_agent_certificate
		// the revocation state the agent listener checks. A package that
		// could write them could enroll as any node or un-revoke a
		// certificate. The v4_kernel_ prefix already protects them; they are
		// named here so the rule survives a prefix change.
		"v4_kernel_agent_enrollment": true, "v4_kernel_agent_certificate": true,
		// Agent reports (A2-5): v4_kernel_agent_report_batch records which
		// traffic and log batches the stream applied. A package that could
		// write it could make a batch count twice, or never. Protected by
		// the prefix; named so the rule survives a prefix change.
		"v4_kernel_agent_report_batch": true,
		// Desired node configurations (node-ops-service.md section 5.5):
		// each node's built configuration, with its protocol secrets, and
		// the revision and hash the agents' ConfigStatus names. A package
		// that could read it would read every node's keys; one that could
		// write it could push a configuration of its own.
		"v4_kernel_node_desired_config": true,
		// Applied node configurations (A2-3): what each node's agent last
		// reported in ConfigStatus, and the revision the kernel verified as
		// applied. A package that could write it could make a node look
		// converged on a configuration it never ran.
		"v4_kernel_node_config_status": true,
		// The agent transport inventory (A2-6): which channel each node's
		// agent was last seen on, which the operator reads before switching
		// agent_control.mtls to required. A package that could write it
		// could make a legacy node look enrolled.
		"v4_kernel_agent_transport": true,
		// Package reports (package-reports.v1): the latest report of each
		// node, plugin and kind, which the kernel authorized and sanitized.
		// A package reads its own rows through kapi_package_report_v1; one
		// that could write the table could show another package's data or a
		// field the sanitizer drops. Protected by the prefix; named so the
		// rule survives a prefix change.
		"v4_kernel_package_report_state": true,
	}
)

// PackageStorageGrants is what the storage capabilities of a verified manifest
// ask the kernel for.
type PackageStorageGrants struct {
	Storage bool
	// AdoptTables are existing tables the package role may read and write.
	AdoptTables []string
	// Views are kernel API views the package role may read.
	Views []string
}

// StorageGrants extracts the storage grants from a manifest whose
// capabilities already passed validation.
func StorageGrants(manifest PluginManifest) PackageStorageGrants {
	var grants PackageStorageGrants
	for _, capability := range manifest.Capabilities {
		switch {
		case capability == CapabilityStorage:
			grants.Storage = true
		case strings.HasPrefix(capability, capabilityStorageAdoptPrefix):
			grants.AdoptTables = append(grants.AdoptTables, strings.TrimPrefix(capability, capabilityStorageAdoptPrefix))
		case strings.HasPrefix(capability, capabilityViewPrefix):
			grants.Views = append(grants.Views, strings.TrimPrefix(capability, capabilityViewPrefix))
		}
	}
	sort.Strings(grants.AdoptTables)
	sort.Strings(grants.Views)
	return grants
}

func validateManifestCapabilities(capabilities []string) error {
	seen := make(map[string]struct{}, len(capabilities))
	storage := false
	needsStorage := ""
	settingsRead := map[string]bool{}
	var settingsSecrets []string
	for _, capability := range capabilities {
		if !capabilityNamePattern.MatchString(capability) {
			return fmt.Errorf("capability %q is not a valid capability name", capability)
		}
		if _, exists := seen[capability]; exists {
			return fmt.Errorf("duplicate capability %q", capability)
		}
		seen[capability] = struct{}{}
		if !strings.HasPrefix(capability, "kernel.") {
			continue
		}
		switch {
		case capability == CapabilityObservedState, capability == CapabilityIdentity, capability == CapabilityOrderComplete,
			capability == CapabilitySubscriberEntitlements, capability == CapabilitySubscriberTraffic,
			capability == CapabilitySubscriberCredentials, capability == CapabilitySubscriberBalance,
			capability == CapabilitySubscriberDirectory, capability == CapabilitySubscriberGroups,
			capability == CapabilitySubscriberSummary, capability == CapabilityTelemetryDashboard,
			isNodeOpsCapability(capability):
		case capability == CapabilityStorage:
			storage = true
		case strings.HasPrefix(capability, capabilityStorageAdoptPrefix):
			table := strings.TrimPrefix(capability, capabilityStorageAdoptPrefix)
			if !adoptableTablePattern.MatchString(table) {
				return fmt.Errorf("capability %q names an invalid table", capability)
			}
			if protectedKernelTable(table) && !finalizedAdoptableTables[table] {
				return fmt.Errorf("capability %q may not adopt a kernel or identity table", capability)
			}
			needsStorage = capability
		case strings.HasPrefix(capability, capabilityViewPrefix):
			view := strings.TrimPrefix(capability, capabilityViewPrefix)
			if !kernelAPIViewPattern.MatchString(view) {
				return fmt.Errorf("capability %q must name a kapi_<name>_v<N> view", capability)
			}
			needsStorage = capability
		case strings.HasPrefix(capability, capabilitySettingsPrefix):
			match := settingsCapability.FindStringSubmatch(capability)
			if match == nil {
				return fmt.Errorf("capability %q must be kernel.settings.<namespace>.<read|write|secrets>.v1", capability)
			}
			if _, known := LookupSettingsNamespace(match[1]); !known {
				return fmt.Errorf("capability %q names an unknown settings namespace", capability)
			}
			switch match[2] {
			case SettingsAccessRead:
				settingsRead[match[1]] = true
			case SettingsAccessSecrets:
				settingsSecrets = append(settingsSecrets, match[1])
			}
		default:
			return fmt.Errorf("unknown kernel capability %q", capability)
		}
	}
	if needsStorage != "" && !storage {
		return fmt.Errorf("capability %q requires %s", needsStorage, CapabilityStorage)
	}
	for _, namespace := range settingsSecrets {
		if !settingsRead[namespace] {
			return fmt.Errorf("capability %q requires %s", SettingsCapability(namespace, SettingsAccessSecrets),
				SettingsCapability(namespace, SettingsAccessRead))
		}
	}
	return nil
}

// forwardCredentialTables hold the credentials the kernel's forward agent
// authentication checks, or copies of them; a package that could read or
// write them could act as any forward node or clean agent:
//   - v2_forward_node holds each forward node's API token, which
//     authenticates the node's agent (WebSocket, gRPC and REST);
//   - v2_forward_clean_agent holds each clean agent's token;
//   - v2_forward_runtime_job holds clean agent jobs, whose payloads carry
//     the node's API token.
var forwardCredentialTables = map[string]bool{
	"v2_forward_node": true, "v2_forward_clean_agent": true, "v2_forward_runtime_job": true,
}

// finalizedAdoptableTables are the protected tables a package may declare
// it adopts (node-ops-service.md sections 4.3 and 4.6): their credentials
// move to the kernel's split tables, and once a table's split is finalized
// its legacy columns hold only tombstones and redacted documents. The
// manifest is accepted on every installation; the grant is honoured only
// where the table is finalized (EffectiveStorageGrants), so one package
// release works whatever the phase. Every other protected table, and these
// before finalize, stays the kernel's.
var finalizedAdoptableTables = map[string]bool{
	"v2_node": true, "v2_node_protocol": true, "v2_forward_node": true,
}

// EffectiveStorageGrants are the grants of a verified manifest the kernel
// honours now: an adoption of a finalized-adoptable table only once its
// split is finalized (nodesecrets.AdoptionAllowed), and a view of the
// split's remainder only once it exists (packagestore.FinalizedViewAvailable).
// What it leaves out is left out of the lease, as if not declared; the
// package's native routes for it stay legacy (mode_unsupported).
func EffectiveStorageGrants(db *gorm.DB, grants PackageStorageGrants) (PackageStorageGrants, error) {
	effective := PackageStorageGrants{Storage: grants.Storage}
	for _, table := range grants.AdoptTables {
		if finalizedAdoptableTables[table] {
			if err := nodesecrets.AdoptionAllowed(db, table); err != nil {
				continue
			}
		}
		effective.AdoptTables = append(effective.AdoptTables, table)
	}
	for _, view := range grants.Views {
		available, err := packagestore.FinalizedViewAvailable(db, view)
		if err != nil {
			return PackageStorageGrants{}, err
		}
		if available {
			effective.Views = append(effective.Views, view)
		}
	}
	return effective, nil
}

func protectedKernelTable(table string) bool {
	if protectedTables[table] || forwardCredentialTables[table] {
		return true
	}
	for _, prefix := range protectedTablePrefixes {
		if strings.HasPrefix(table, prefix) {
			return true
		}
	}
	return false
}
