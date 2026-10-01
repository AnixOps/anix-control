package service

import (
	"strings"
	"sync/atomic"
)

// Settings namespaces (docs/architecture/settings-service.md). Every
// setting key belongs to at most one namespace, which the KernelSettings
// contract authorizes per capability; a key in no namespace is reachable
// through no contract call.
const (
	// SettingsNamespaceMail is the SMTP delivery configuration
	// (notification.email.*).
	SettingsNamespaceMail = "mail"
	// SettingsNamespaceInvite is the affiliate's frontend settings
	// (invite.*).
	SettingsNamespaceInvite = "invite"
	// SettingsNamespaceNodeX is the NodeX control plane address, shared
	// token and timeout (forward.runtime.nodex.*).
	SettingsNamespaceNodeX = "nodex"
	// SettingsNamespaceForwardRuntime is the forward runtime backend and
	// its local Ansible settings.
	SettingsNamespaceForwardRuntime = "forward-runtime"
	// SettingsNamespaceBackup is the backup configuration row of
	// v2_backup_config, one key per field (backup.<field>).
	SettingsNamespaceBackup = "backup"
)

// Settings capability accesses: kernel.settings.<namespace>.<access>.v1.
const (
	SettingsAccessRead    = "read"
	SettingsAccessWrite   = "write"
	SettingsAccessSecrets = "secrets"
)

// SettingsStorage is where a namespace's keys live.
type SettingsStorage int

const (
	// SettingsInSystemConfig keys are rows of v2_system_config.
	SettingsInSystemConfig SettingsStorage = iota
	// SettingsInBackupConfig keys are the fields of the first
	// v2_backup_config row.
	SettingsInBackupConfig
)

// SettingsAudit is the audit entry a write of the namespace records: the
// one the kernel's own handler records for the same change.
type SettingsAudit int

const (
	// SettingsAuditNone records nothing.
	SettingsAuditNone SettingsAudit = iota
	// SettingsAuditSystemConfig records a system_config entry per key, as
	// the system configuration handlers do.
	SettingsAuditSystemConfig
	// SettingsAuditBackupConfig records a backup_config entry, as the
	// backup configuration handler does.
	SettingsAuditBackupConfig
)

// SettingsNamespace is one entry of the kernel's namespace table.
type SettingsNamespace struct {
	Name string
	// Keys are exact keys; Prefixes match the longer keys that start with
	// them.
	Keys     []string
	Prefixes []string
	Storage  SettingsStorage
	Audit    SettingsAudit
	// SecretKeys are secrets the kernel's key-name rule does not catch.
	SecretKeys []string
}

// BackupSettingsPrefix starts every key of the backup namespace.
const BackupSettingsPrefix = "backup."

// settingsNamespaces is the kernel's namespace table. No key may match two
// entries (TestSettingsNamespacesDoNotOverlap).
var settingsNamespaces = []SettingsNamespace{
	{
		Name: SettingsNamespaceMail, Prefixes: []string{"notification.email."}, Audit: SettingsAuditSystemConfig,
		// The SMTP configuration is one JSON value that holds the password.
		SecretKeys: []string{"notification.email.config"},
	},
	{Name: SettingsNamespaceInvite, Prefixes: []string{"invite."}, Audit: SettingsAuditSystemConfig},
	{Name: SettingsNamespaceNodeX, Prefixes: []string{"forward.runtime.nodex."}, Audit: SettingsAuditSystemConfig},
	{
		Name: SettingsNamespaceForwardRuntime, Audit: SettingsAuditSystemConfig,
		Keys:     []string{forwardRuntimeBackendConfigKey, forwardRuntimeNodeXModeConfigKey},
		Prefixes: []string{"forward.runtime.ansible.", "forward.runtime.iptables_ansible.", "forward.ansible."},
	},
	{Name: SettingsNamespaceBackup, Prefixes: []string{BackupSettingsPrefix}, Storage: SettingsInBackupConfig, Audit: SettingsAuditBackupConfig},
}

// SettingsNamespaces returns the namespace table.
func SettingsNamespaces() []SettingsNamespace {
	return append([]SettingsNamespace(nil), settingsNamespaces...)
}

// LookupSettingsNamespace returns the namespace called name.
func LookupSettingsNamespace(name string) (SettingsNamespace, bool) {
	for _, namespace := range settingsNamespaces {
		if namespace.Name == name {
			return namespace, true
		}
	}
	return SettingsNamespace{}, false
}

// Contains reports whether key belongs to the namespace.
func (n SettingsNamespace) Contains(key string) bool {
	for _, exact := range n.Keys {
		if key == exact {
			return true
		}
	}
	for _, prefix := range n.Prefixes {
		if strings.HasPrefix(key, prefix) && len(key) > len(prefix) {
			return true
		}
	}
	return false
}

// Secret reports whether key's value is a secret: a name the kernel masks
// for its administrators, or a key the namespace declares secret.
func (n SettingsNamespace) Secret(key string) bool {
	if IsSensitiveSystemConfigKey(key) {
		return true
	}
	for _, secret := range n.SecretKeys {
		if key == secret {
			return true
		}
	}
	return false
}

// SettingsNamespaceOf returns the namespace key belongs to.
func SettingsNamespaceOf(key string) (SettingsNamespace, bool) {
	for _, namespace := range settingsNamespaces {
		if namespace.Contains(key) {
			return namespace, true
		}
	}
	return SettingsNamespace{}, false
}

// SettingsCapability is the capability for access to namespace.
func SettingsCapability(namespace, access string) string {
	return capabilitySettingsPrefix + namespace + "." + access + ".v1"
}

// settingsGenerations count the changes of each namespace in this process.
// A copy of a namespace's settings the kernel keeps in memory (the backup
// service's configuration, the invite service's) remembers the generation
// it loaded at and reloads when it moved: a writer bumps it after its
// change commits, and a reader reads it before it reads the database, so a
// copy is never older than its generation.
var settingsGenerations = func() map[string]*atomic.Uint64 {
	generations := make(map[string]*atomic.Uint64, len(settingsNamespaces))
	for _, namespace := range settingsNamespaces {
		generations[namespace.Name] = new(atomic.Uint64)
	}
	return generations
}()

// SettingsGeneration returns the namespace's current generation.
func SettingsGeneration(namespace string) uint64 {
	if generation, ok := settingsGenerations[namespace]; ok {
		return generation.Load()
	}
	return 0
}

// BumpSettingsGeneration marks the namespace changed, so every in-memory
// copy of it reloads, and returns the new generation. Call it after the
// change committed.
func BumpSettingsGeneration(namespace string) uint64 {
	if generation, ok := settingsGenerations[namespace]; ok {
		return generation.Add(1)
	}
	return 0
}

// BumpSettingsGenerationAfter bumps namespace after a write that read its
// generation before writing, and reports whether the writer may keep the
// value it wrote as its in-memory copy at the new generation: only when no
// other write bumped the namespace in between, else the copy must reload.
func BumpSettingsGenerationAfter(namespace string, before uint64) (uint64, bool) {
	generation := BumpSettingsGeneration(namespace)
	return generation, generation == before+1
}

// TouchSettingsKey bumps the generation of key's namespace, if any.
func TouchSettingsKey(key string) {
	if namespace, ok := SettingsNamespaceOf(key); ok {
		BumpSettingsGeneration(namespace.Name)
	}
}
