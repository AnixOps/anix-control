package service

import (
	"context"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/packagebridge"
	"github.com/stretchr/testify/require"
)

// No key may belong to two namespaces: no exact key or prefix of one
// namespace starts with a prefix of another or equals another's key.
func TestSettingsNamespacesDoNotOverlap(t *testing.T) {
	type entry struct{ namespace, value string }
	var exact, prefixes []entry
	names := map[string]bool{}
	for _, namespace := range SettingsNamespaces() {
		require.False(t, names[namespace.Name], "namespace %s is listed twice", namespace.Name)
		names[namespace.Name] = true
		require.Regexp(t, `^[a-z][a-z0-9-]*$`, namespace.Name, "a namespace name must fit the capability grammar")
		for _, key := range namespace.Keys {
			exact = append(exact, entry{namespace.Name, key})
		}
		for _, prefix := range namespace.Prefixes {
			require.True(t, strings.HasSuffix(prefix, "."), "prefix %q ends a key segment", prefix)
			prefixes = append(prefixes, entry{namespace.Name, prefix})
		}
	}
	for _, prefix := range prefixes {
		for _, other := range prefixes {
			if prefix != other {
				require.False(t, strings.HasPrefix(other.value, prefix.value), "%s %q overlaps %s %q", prefix.namespace, prefix.value, other.namespace, other.value)
			}
		}
		for _, key := range exact {
			require.False(t, strings.HasPrefix(key.value, prefix.value), "%s key %q is under %s prefix %q", key.namespace, key.value, prefix.namespace, prefix.value)
		}
	}
	seen := map[string]bool{}
	for _, key := range exact {
		require.False(t, seen[key.value], "key %q is listed twice", key.value)
		seen[key.value] = true
	}
}

func TestSettingsKeysMapToOneNamespace(t *testing.T) {
	for key, want := range map[string]string{
		"notification.email.config":             SettingsNamespaceMail,
		"invite.frontend.config":                SettingsNamespaceInvite,
		"forward.runtime.nodex.token":           SettingsNamespaceNodeX,
		"forward.runtime.nodex.base_url":        SettingsNamespaceNodeX,
		"forward.runtime.nodex_mode":            SettingsNamespaceForwardRuntime,
		"forward.runtime_backend":               SettingsNamespaceForwardRuntime,
		"forward.runtime.ansible.inventory":     SettingsNamespaceForwardRuntime,
		"forward.ansible.become":                SettingsNamespaceForwardRuntime,
		"backup.s3_secret_key":                  SettingsNamespaceBackup,
		"security.mfa.config":                   "",
		"scheduler.forward_flow_reset.last_day": "",
		"site.name":                             "",
		"invite.":                               "",
		"forward.runtime.nodex":                 "",
	} {
		namespace, ok := SettingsNamespaceOf(key)
		require.Equal(t, want != "", ok, key)
		require.Equal(t, want, namespace.Name, key)
	}
}

func TestSettingsSecretsFollowTheAdministratorMasking(t *testing.T) {
	mail, _ := LookupSettingsNamespace(SettingsNamespaceMail)
	nodex, _ := LookupSettingsNamespace(SettingsNamespaceNodeX)
	backup, _ := LookupSettingsNamespace(SettingsNamespaceBackup)
	require.True(t, mail.Secret("notification.email.config"), "declared: the JSON value holds the SMTP password")
	require.True(t, nodex.Secret("forward.runtime.nodex.token"))
	require.False(t, nodex.Secret("forward.runtime.nodex.base_url"))
	require.True(t, backup.Secret("backup.s3_access_key"))
	require.True(t, backup.Secret("backup.s3_secret_key"))
	require.False(t, backup.Secret("backup.s3_bucket"))
}

func TestSettingsCapabilityGrammar(t *testing.T) {
	require.NoError(t, validateManifestCapabilities([]string{
		"kernel.settings.mail.read.v1", "kernel.settings.mail.write.v1", "kernel.settings.mail.secrets.v1",
		"kernel.settings.forward-runtime.write.v1", "kernel.settings.backup.write.v1",
	}))
	for name, capabilities := range map[string][]string{
		"unknown namespace":         {"kernel.settings.all.read.v1"},
		"unknown access":            {"kernel.settings.mail.admin.v1"},
		"no version":                {"kernel.settings.mail.read"},
		"secrets without read":      {"kernel.settings.nodex.secrets.v1", "kernel.settings.nodex.write.v1"},
		"another namespace's read":  {"kernel.settings.nodex.secrets.v1", "kernel.settings.mail.read.v1"},
		"no namespace":              {"kernel.settings.read.v1"},
		"underscore namespace name": {"kernel.settings.forward_runtime.read.v1"},
	} {
		require.Error(t, validateManifestCapabilities(capabilities), name)
	}
}

// Settings capabilities are authorized one at a time, on the same terms as
// the other kernel contracts: official, signed, current generation.
func TestAuthorizeCapabilityGrantsOnlyTheSignedSettingsCapabilities(t *testing.T) {
	db := newKernelTestDB(t)
	read := SettingsCapability(SettingsNamespaceNodeX, SettingsAccessRead)
	secrets := SettingsCapability(SettingsNamespaceNodeX, SettingsAccessSecrets)
	publicKey, _ := seedKnowledgeRelease(t, db, "", []string{read, secrets})
	operations := PackageHostOperations{DB: db, FallbackPublicKey: publicKey}
	host := packagebridge.HostIdentity{PackageID: "knowledge", Version: "4.0.1", Generation: 7}
	ctx := context.Background()

	require.NoError(t, operations.AuthorizeCapability(ctx, host, read))
	require.NoError(t, operations.AuthorizeCapability(ctx, host, secrets))
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, host, SettingsCapability(SettingsNamespaceNodeX, SettingsAccessWrite)), ErrCapabilityNotAuthorized)
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, host, SettingsCapability(SettingsNamespaceMail, SettingsAccessRead)), ErrCapabilityNotAuthorized)
	require.NoError(t, db.Model(&model.Plugin{}).Where("id = ?", "knowledge").Update("official", false).Error)
	require.ErrorIs(t, operations.AuthorizeCapability(ctx, host, read), ErrCapabilityNotAuthorized, "only an official package")
}

// A copy of a namespace kept in memory reloads after another writer's
// change, and keeps its own write only when no other write came between.
func TestSettingsGenerations(t *testing.T) {
	before := SettingsGeneration(SettingsNamespaceInvite)
	generation, keep := BumpSettingsGenerationAfter(SettingsNamespaceInvite, before)
	require.True(t, keep)
	require.Equal(t, before+1, generation)
	BumpSettingsGeneration(SettingsNamespaceInvite)
	_, keep = BumpSettingsGenerationAfter(SettingsNamespaceInvite, generation)
	require.False(t, keep, "another write came between")

	mail := SettingsGeneration(SettingsNamespaceMail)
	TouchSettingsKey("notification.email.config")
	require.Equal(t, mail+1, SettingsGeneration(SettingsNamespaceMail))
	TouchSettingsKey("site.name")
	require.Zero(t, SettingsGeneration("everything"))
}
