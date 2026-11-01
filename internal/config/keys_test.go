package config

import (
	"bufio"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The operator keys are exactly the table of the design's section 5, plus
// grpc.edge_proxies (docs/design/edge-grpc), which is read only in the edge
// mode.
func TestOperatorKeysAreTheDesignTable(t *testing.T) {
	var paths []string
	for _, key := range OperatorKeys() {
		paths = append(paths, key.Path)
		assert.NotEmpty(t, key.Description, key.Path)
		assert.False(t, key.Secret, "the configuration file holds no secret: %s", key.Path)
	}
	assert.Equal(t, []string{"domain", "tls.mode", "tls.cert_file", "tls.key_file", "listen", "grpc.port", "grpc.public_name", "grpc.edge_proxies", "subscribe_path", "data_dir"}, paths)
}

// Every key an earlier release or template knew is an operator key or in
// the deprecated list, so that no earlier configuration is refused as
// unknown.
func TestEveryEarlierKeyIsKnown(t *testing.T) {
	file, err := os.Open(filepath.Join("testdata", "earlier-keys.txt"))
	require.NoError(t, err)
	defer func() { _ = file.Close() }()
	scanner := bufio.NewScanner(file)
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		count++
		_, ok := LookupKey(line)
		assert.True(t, ok, "%s is neither an operator key nor deprecated", line)
	}
	require.NoError(t, scanner.Err())
	assert.Equal(t, 165, count, "the fixture lists every earlier key")
}

// Every field of Config is set by a key: an operator key, or a deprecated
// key (whose built-in value stands when it is ignored).
func TestEveryConfigFieldHasAKey(t *testing.T) {
	fields := map[string]bool{}
	for _, key := range Keys() {
		fields[key.fieldPath()] = true
	}
	var missing []string
	for path := range configFieldTypes {
		if !fields[path] {
			missing = append(missing, path)
		}
	}
	sort.Strings(missing)
	assert.Empty(t, missing, "fields no key sets")
}

func TestDeprecatedListIsComplete(t *testing.T) {
	read, ignored := 0, 0
	for _, key := range Keys() {
		if !key.Deprecated() {
			continue
		}
		assert.NotEmpty(t, key.Reason, key.Path)
		assert.Regexp(t, `^M[2-5]( \((P[0-9]+|N[0-9]+|S[0-9]+)\))?$`, key.RemovedIn, key.Path)
		if key.Replacement != "" {
			replacement, ok := LookupKey(key.Replacement)
			require.True(t, ok, "%s names an unknown replacement %s", key.Path, key.Replacement)
			assert.False(t, replacement.Deprecated(), "%s is replaced by a deprecated key", key.Path)
		}
		switch key.Status {
		case KeyRead:
			read++
		case KeyIgnored:
			ignored++
		}
	}
	assert.Equal(t, 54, read, "keys still read")
	assert.Equal(t, 95, ignored, "keys ignored")
	// The P7 switches are ignored whatever order the pull requests land in.
	for _, path := range []string{"plugins.control_execution_enabled", "plugins.dispatch_enabled", "plugins.topology_execution_enabled", "package_routes.default_mode"} {
		key, ok := LookupKey(path)
		require.True(t, ok, path)
		assert.Equal(t, KeyIgnored, key.Status, path)
	}
	// Secrets are marked, so -print-env and the docs never show a value.
	for _, path := range []string{"jwt.secret", "identity.kek", "module_runtime.ca_kek", "admin.password", "app.api_token", "grpc.api_token", "database.password", "database.dsn", "forward_runtime.nodex.token", "cache.redis_password"} {
		key, ok := LookupKey(path)
		require.True(t, ok, path)
		assert.True(t, key.Secret, path)
	}
}

func TestKeyVariablesAreUniqueAndNeverReserved(t *testing.T) {
	seen := map[string]string{}
	for _, key := range Keys() {
		if key.Subtree || !envCapable(key) {
			continue
		}
		name := key.EnvName()
		require.False(t, reservedEnvNames[name], name)
		require.False(t, reservedEnvNames[name+EnvFileSuffix], name)
		if previous, ok := seen[name]; ok {
			t.Fatalf("%s is the variable of %s and %s", name, previous, key.Path)
		}
		seen[name] = key.Path
	}
	assert.Equal(t, "ANIX_CONTROL_SUBSCRIBE_PATH", mustKey(t, "subscribe_path").EnvName())
	assert.Equal(t, "ANIX_CONTROL_DATA_DIR", mustKey(t, "data_dir").EnvName())
	assert.Equal(t, "ANIX_CONTROL_GRPC_PUBLIC_NAME", mustKey(t, "grpc.public_name").EnvName())
}

func mustKey(t *testing.T, path string) Key {
	t.Helper()
	key, ok := LookupKey(path)
	require.True(t, ok, path)
	return key
}

func TestLookupKeyCoversSubtrees(t *testing.T) {
	key, ok := LookupKey("forward_runtime.iptables_ansible.inventory")
	require.True(t, ok)
	assert.Equal(t, "forward_runtime.iptables_ansible", key.Path)
	assert.True(t, key.Subtree)
	_, ok = LookupKey("forward_runtime.nftables_ansible.unknown")
	assert.False(t, ok)
}

func TestClosestKeyAndVariable(t *testing.T) {
	assert.Equal(t, "domain", closestKey("domian"))
	assert.Equal(t, "grpc.port", closestKey("grpc.prot"))
	assert.Equal(t, "data_dir", closestKey("datadir"))
	assert.Equal(t, "tls.mode", closestKey("tls.mod"))
	assert.Equal(t, "database.password", closestKey("databse.password"))
	assert.Equal(t, "ANIX_CONTROL_DATABASE_PASSWORD", closestEnvName("ANIX_CONTROL_DATABSE_PASSWORD"))
	assert.Equal(t, "ANIX_CONTROL_DOMAIN", closestEnvName("ANIX_CONTROL_DOMIAN"))
	assert.Equal(t, 3, editDistance("kitten", "sitting"))
	assert.Equal(t, 0, editDistance("", ""))
	assert.Equal(t, 4, editDistance("", "abcd"))
}

func TestBuildRegistryRefusesABadList(t *testing.T) {
	for name, text := range map[string]string{
		"unknown field":      "keys:\n  - key: env\n    status: ignored\n    removed_in: M2\n    reason: x\n    colour: red\n",
		"no reason":          "keys:\n  - key: env\n    status: ignored\n    removed_in: M2\n",
		"unknown status":     "keys:\n  - key: env\n    status: maybe\n    removed_in: M2\n    reason: x\n",
		"operator in list":   "keys:\n  - key: env\n    status: operator\n",
		"twice":              "keys:\n  - key: env\n    status: ignored\n    removed_in: M2\n    reason: x\n  - key: env\n    status: ignored\n    removed_in: M2\n    reason: x\n",
		"read without field": "keys:\n  - key: nothing.here\n    status: read\n    removed_in: M2\n    reason: x\n",
		"operator collision": "keys:\n  - key: domain\n    status: ignored\n    removed_in: M2\n    reason: x\n",
		"key and section":    "keys:\n  - key: server\n    status: ignored\n    removed_in: M2\n    reason: x\n  - key: server.port\n    status: read\n    removed_in: M2\n    reason: x\n",
		"variable collision": "keys:\n  - key: a.b_c\n    status: ignored\n    removed_in: M2\n    reason: x\n  - key: a_b.c\n    status: ignored\n    removed_in: M2\n    reason: x\n",
	} {
		_, err := buildRegistry([]byte(text))
		assert.Error(t, err, name)
	}
}

func TestConfigFieldResolvesYAMLPaths(t *testing.T) {
	cfg := Defaults()
	field, ok := configField(cfg, "app.subscribe_path")
	require.True(t, ok)
	assert.Equal(t, reflect.String, field.Kind())
	_, ok = configField(cfg, "app.missing")
	assert.False(t, ok)
	_, ok = configField(cfg, "domain.too.deep")
	assert.False(t, ok)
}
