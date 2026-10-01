package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func migrationTestManifest(index []byte) PluginManifest {
	return PluginManifest{
		ID: "knowledge", Version: "4.0.1",
		Migrations: &PluginMigrations{Index: "migrations/index.json", SHA256: sha256Bytes(index)},
	}
}

func TestParsePluginMigrationIndexReturnsVerifiedStepsInOrder(t *testing.T) {
	first, second := []byte("CREATE TABLE a (id int);\n"), []byte("CREATE TABLE b (id int);\n")
	index := []byte(`{"format":"anixops.migrations/v1","migrations":[` +
		`{"id":"001_a","path":"migrations/001_a.sql","sha256":"` + sha256Bytes(first) + `"},` +
		`{"id":"002_b","path":"migrations/002_b.sql","sha256":"` + sha256Bytes(second) + `"}` +
		`],"package_id":"knowledge","version":"4.0.1"}`)
	artifact := kernelTestV2Package(t, map[string][]byte{
		"migrations/index.json": index, "migrations/001_a.sql": first, "migrations/002_b.sql": second,
	})

	steps, err := ParsePluginMigrationIndex(migrationTestManifest(index), artifact)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	assert.Equal(t, "001_a", steps[0].ID)
	assert.Equal(t, sha256Bytes(first), steps[0].SHA256)
	assert.Equal(t, "migrations/002_b.sql", steps[1].Path)
}

func TestParsePluginMigrationIndexAcceptsV400IndexesWithoutStepDigests(t *testing.T) {
	script := []byte("CREATE TABLE IF NOT EXISTS identity_platform_projection (id text);\n")
	index := []byte(`{"format":"anixops.migrations/v1","migrations":[{"id":"001_identity_platform","path":"migrations/001_identity_platform.sql"}],"package_id":"knowledge","version":"4.0.1"}`)
	artifact := kernelTestV2Package(t, map[string][]byte{"migrations/index.json": index, "migrations/001_identity_platform.sql": script})

	steps, err := ParsePluginMigrationIndex(migrationTestManifest(index), artifact)
	require.NoError(t, err)
	require.Len(t, steps, 1)
	assert.Equal(t, sha256Bytes(script), steps[0].SHA256, "the digest is computed from the packaged script")
}

func TestParsePluginMigrationIndexRejectsTamperedOrForeignIndexes(t *testing.T) {
	script := []byte("SELECT 1;\n")
	build := func(body string) ([]byte, []byte) {
		index := []byte(body)
		return index, kernelTestV2Package(t, map[string][]byte{"migrations/index.json": index, "migrations/001_a.sql": script})
	}
	cases := map[string]string{
		"step digest mismatch": `{"format":"anixops.migrations/v1","migrations":[{"id":"001_a","path":"migrations/001_a.sql","sha256":"` + sha256Bytes([]byte("other")) + `"}],"package_id":"knowledge","version":"4.0.1"}`,
		"foreign package":      `{"format":"anixops.migrations/v1","migrations":[],"package_id":"ticket","version":"4.0.1"}`,
		"foreign version":      `{"format":"anixops.migrations/v1","migrations":[],"package_id":"knowledge","version":"4.0.0"}`,
		"version token":        `{"format":"anixops.migrations/v1","migrations":[],"package_id":"knowledge","version":"__ANIXOPS_PACKAGE_VERSION__"}`,
		"unknown format":       `{"format":"anixops.migrations/v2","migrations":[],"package_id":"knowledge","version":"4.0.1"}`,
		"unknown field":        `{"format":"anixops.migrations/v1","migrations":[],"package_id":"knowledge","version":"4.0.1","extra":1}`,
		"path outside dir":     `{"format":"anixops.migrations/v1","migrations":[{"id":"001_a","path":"bin/control-host"}],"package_id":"knowledge","version":"4.0.1"}`,
		"duplicate id":         `{"format":"anixops.migrations/v1","migrations":[{"id":"001_a","path":"migrations/001_a.sql"},{"id":"001_a","path":"migrations/001_a.sql"}],"package_id":"knowledge","version":"4.0.1"}`,
		"missing script":       `{"format":"anixops.migrations/v1","migrations":[{"id":"002_b","path":"migrations/002_b.sql"}],"package_id":"knowledge","version":"4.0.1"}`,
		"invalid id":           `{"format":"anixops.migrations/v1","migrations":[{"id":"../x","path":"migrations/001_a.sql"}],"package_id":"knowledge","version":"4.0.1"}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			index, artifact := build(body)
			_, err := ParsePluginMigrationIndex(migrationTestManifest(index), artifact)
			require.Error(t, err)
		})
	}

	t.Run("index digest mismatch", func(t *testing.T) {
		index, artifact := build(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"knowledge","version":"4.0.1"}`)
		manifest := migrationTestManifest(index)
		manifest.Migrations.SHA256 = sha256Bytes([]byte("tampered"))
		_, err := ParsePluginMigrationIndex(manifest, artifact)
		require.ErrorContains(t, err, "migrations index digest")
	})
}

func TestManifestCapabilityGrammar(t *testing.T) {
	valid := [][]string{
		nil,
		{"telemetry.read"},
		{"forward.gost.mesh", "plugin.runtime-state", "kernel.observed-state"},
		{CapabilityStorage},
		{CapabilityStorage, "kernel.storage.adopt:v2_agent_diagnostic_task"},
		{CapabilityStorage, "kernel.storage.adopt:v2_knowledge", "kernel.view:kapi_user_directory_v1"},
		{CapabilityStorage, "kernel.storage.adopt:v2_node_log", "kernel.view:kapi_node_status_v1"},
		{CapabilityStorage, "kernel.storage.adopt:v2_forward_tunnel", "kernel.view:kapi_forward_node_v1", CapabilitySubscriberTraffic},
		{CapabilityIdentity, CapabilityStorage},
	}
	for _, capabilities := range valid {
		assert.NoError(t, validateManifestCapabilities(capabilities), "%v", capabilities)
	}
	invalid := map[string][]string{
		"unknown kernel capability":  {"kernel.admin"},
		"unknown identity version":   {"kernel.identity.v2"},
		"adopt without storage":      {"kernel.storage.adopt:v2_knowledge"},
		"view without storage":       {"kernel.view:kapi_user_directory_v1"},
		"adopt users table":          {CapabilityStorage, "kernel.storage.adopt:v2_user"},
		"adopt kernel table":         {CapabilityStorage, "kernel.storage.adopt:v4_kernel_lease"},
		"adopt identity table":       {CapabilityStorage, "kernel.storage.adopt:identity_platform_projection"},
		"adopt system config":        {CapabilityStorage, "kernel.storage.adopt:v2_system_config"},
		"adopt forward node tokens":  {CapabilityStorage, "kernel.storage.adopt:v2_forward_node"},
		"adopt clean agent tokens":   {CapabilityStorage, "kernel.storage.adopt:v2_forward_clean_agent"},
		"adopt forward runtime jobs": {CapabilityStorage, "kernel.storage.adopt:v2_forward_runtime_job"},
		"adopt node credentials":     {CapabilityStorage, "kernel.storage.adopt:v2_node"},
		"adopt registration keys":    {CapabilityStorage, "kernel.storage.adopt:v2_authorized_key"},
		"adopt a view":               {CapabilityStorage, "kernel.storage.adopt:kapi_user_directory_v1"},
		"bad table name":             {CapabilityStorage, "kernel.storage.adopt:V2-Knowledge"},
		"bad view name":              {CapabilityStorage, "kernel.view:v2_user"},
		"duplicate":                  {"telemetry.read", "telemetry.read"},
		"malformed":                  {"Telemetry Read"},
		"adopt node protocols":       {CapabilityStorage, "kernel.storage.adopt:v2_node_protocol"},
		"adopt wireguard peers":      {CapabilityStorage, "kernel.storage.adopt:v2_wireguard_peer"},
		"adopt split credentials":    {CapabilityStorage, "kernel.storage.adopt:v4_kernel_node_credential"},
		"adopt split secrets":        {CapabilityStorage, "kernel.storage.adopt:v4_kernel_protocol_secret"},
		"adopt split state":          {CapabilityStorage, "kernel.storage.adopt:v4_kernel_node_secret_split"},
	}
	for name, capabilities := range invalid {
		assert.Error(t, validateManifestCapabilities(capabilities), name)
	}
}

func TestStorageGrantsExtractsSortedGrants(t *testing.T) {
	grants := StorageGrants(PluginManifest{Capabilities: []string{
		"kernel.view:kapi_user_directory_v1", "kernel.storage.adopt:v2_ticket_message", CapabilityStorage,
		"kernel.storage.adopt:v2_ticket", "plugin.cleanup",
	}})
	assert.True(t, grants.Storage)
	assert.Equal(t, []string{"v2_ticket", "v2_ticket_message"}, grants.AdoptTables)
	assert.Equal(t, []string{"kapi_user_directory_v1"}, grants.Views)
	assert.False(t, StorageGrants(PluginManifest{Capabilities: []string{"telemetry.read"}}).Storage)
}
