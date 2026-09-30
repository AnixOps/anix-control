package service

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
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
	// capabilityStorageAdoptPrefix grants the storage role access to an
	// existing kernel table, adopted in place: kernel.storage.adopt:<table>.
	capabilityStorageAdoptPrefix = "kernel.storage.adopt:"
	// capabilityViewPrefix grants read access to a kernel API view:
	// kernel.view:kapi_<name>_v<N>.
	capabilityViewPrefix = "kernel.view:"
)

var (
	capabilityNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9._:-]{0,127}$`)
	adoptableTablePattern  = regexp.MustCompile(`^[a-z][a-z0-9_]{0,62}$`)
	kernelAPIViewPattern   = regexp.MustCompile(`^kapi_[a-z][a-z0-9_]*_v[1-9][0-9]*$`)
	protectedTablePrefixes = []string{"v2_user", "v3_kernel_", "v4_kernel_", "identity_", "kapi_", "pg_"}
	protectedTables        = map[string]bool{"v2_system_config": true, "v2_audit_log": true, "v2_operation_log": true}
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
		case capability == CapabilityObservedState:
		case capability == CapabilityStorage:
			storage = true
		case strings.HasPrefix(capability, capabilityStorageAdoptPrefix):
			table := strings.TrimPrefix(capability, capabilityStorageAdoptPrefix)
			if !adoptableTablePattern.MatchString(table) {
				return fmt.Errorf("capability %q names an invalid table", capability)
			}
			if protectedKernelTable(table) {
				return fmt.Errorf("capability %q may not adopt a kernel or identity table", capability)
			}
			needsStorage = capability
		case strings.HasPrefix(capability, capabilityViewPrefix):
			view := strings.TrimPrefix(capability, capabilityViewPrefix)
			if !kernelAPIViewPattern.MatchString(view) {
				return fmt.Errorf("capability %q must name a kapi_<name>_v<N> view", capability)
			}
			needsStorage = capability
		default:
			return fmt.Errorf("unknown kernel capability %q", capability)
		}
	}
	if needsStorage != "" && !storage {
		return fmt.Errorf("capability %q requires %s", needsStorage, CapabilityStorage)
	}
	return nil
}

func protectedKernelTable(table string) bool {
	if protectedTables[table] {
		return true
	}
	for _, prefix := range protectedTablePrefixes {
		if strings.HasPrefix(table, prefix) {
			return true
		}
	}
	return false
}
