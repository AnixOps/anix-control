package pluginhost_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeneratorUsesToolPinsAndRejectsMismatchedProtocBeforeGeneration(t *testing.T) {
	fixtureRoot := writeGeneratorFixture(t, "29.1")
	fixtureScript := filepath.Join(fixtureRoot, "api", "pluginhost", "gen.sh")

	marker := filepath.Join(fixtureRoot, "generation-ran")
	fakeBin := filepath.Join(fixtureRoot, "bin")
	require.NoError(t, os.MkdirAll(fakeBin, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(fakeBin, "protoc"), []byte(`#!/usr/bin/env bash
if [[ "${1:-}" == "--version" ]]; then
  printf 'libprotoc 29.2\n'
  exit 0
fi
touch "${GEN_MARKER}"
`), 0o755))

	command := exec.Command(fixtureScript)
	command.Dir = fixtureRoot
	command.Env = append(os.Environ(), "PATH="+fakeBin+":"+os.Getenv("PATH"), "GEN_MARKER="+marker)
	output, err := command.CombinedOutput()

	require.Error(t, err)
	require.Contains(t, string(output), "expected libprotoc 29.1")
	require.NoFileExists(t, marker)
	require.NoFileExists(t, filepath.Join(fixtureRoot, "api", "pluginhost", "v1", "control_host.pb.go"))
	require.NoFileExists(t, filepath.Join(fixtureRoot, "api", "pluginhost", "v1", "control_host_grpc.pb.go"))
}

func TestGeneratorUsesPinnedPluginsInsteadOfPATH(t *testing.T) {
	protocPath := requirePinnedProtoc(t)
	fixtureRoot := writeGeneratorFixture(t, "29.2")
	fixtureScript := filepath.Join(fixtureRoot, "api", "pluginhost", "gen.sh")

	fakeBin := filepath.Join(fixtureRoot, "bin")
	require.NoError(t, os.MkdirAll(fakeBin, 0o755))
	goMarker := filepath.Join(fixtureRoot, "path-protoc-gen-go-used")
	grpcMarker := filepath.Join(fixtureRoot, "path-protoc-gen-go-grpc-used")
	writeFakePlugin(t, filepath.Join(fakeBin, "protoc-gen-go"), goMarker)
	writeFakePlugin(t, filepath.Join(fakeBin, "protoc-gen-go-grpc"), grpcMarker)

	command := exec.Command(fixtureScript)
	command.Dir = fixtureRoot
	command.Env = append(
		os.Environ(),
		"PATH="+fakeBin+":"+filepath.Dir(protocPath)+":"+os.Getenv("PATH"),
	)
	output, err := command.CombinedOutput()

	require.NoError(t, err, string(output))
	require.NoFileExists(t, goMarker)
	require.NoFileExists(t, grpcMarker)

	generated, err := os.ReadFile(filepath.Join(fixtureRoot, "api", "pluginhost", "v1", "control_host.pb.go"))
	require.NoError(t, err)
	require.Contains(t, string(generated), "protoc-gen-go v1.36.11")

	grpcGenerated, err := os.ReadFile(filepath.Join(fixtureRoot, "api", "pluginhost", "v1", "control_host_grpc.pb.go"))
	require.NoError(t, err)
	require.Contains(t, string(grpcGenerated), "protoc-gen-go-grpc v1.6.1")
}

func writeGeneratorFixture(t *testing.T, protocVersion string) string {
	t.Helper()

	repoRoot := repositoryRoot(t)
	fixtureRoot := t.TempDir()
	fixtureScript := filepath.Join(fixtureRoot, "api", "pluginhost", "gen.sh")
	fixtureTools := filepath.Join(fixtureRoot, "tools.go")
	fixtureProto := filepath.Join(fixtureRoot, "api", "pluginhost", "v1", "control_host.proto")
	require.NoError(t, os.MkdirAll(filepath.Dir(fixtureProto), 0o755))

	script, err := os.ReadFile(filepath.Join(repoRoot, "api", "pluginhost", "gen.sh"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fixtureScript, script, 0o755))

	tools, err := os.ReadFile(filepath.Join(repoRoot, "tools.go"))
	require.NoError(t, err)
	tools = []byte(strings.Replace(string(tools), `ProtocVersion          = "29.2"`, `ProtocVersion          = "`+protocVersion+`"`, 1))
	require.Contains(t, string(tools), `ProtocVersion          = "`+protocVersion+`"`)
	require.NoError(t, os.WriteFile(fixtureTools, tools, 0o644))

	proto, err := os.ReadFile(filepath.Join(repoRoot, "api", "pluginhost", "v1", "control_host.proto"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(fixtureProto, proto, 0o644))
	return fixtureRoot
}

func writeFakePlugin(t *testing.T, path, marker string) {
	t.Helper()

	plugin := "#!/usr/bin/env bash\ntouch \"" + marker + "\"\nexit 1\n"
	require.NoError(t, os.WriteFile(path, []byte(plugin), 0o755))
}

func requirePinnedProtoc(t *testing.T) string {
	t.Helper()

	protocPath, err := exec.LookPath("protoc")
	if err != nil {
		t.Skip("libprotoc 29.2 is required for generator integration coverage")
	}
	output, err := exec.Command(protocPath, "--version").CombinedOutput()
	require.NoError(t, err, string(output))
	if strings.TrimSpace(string(output)) != "libprotoc 29.2" {
		t.Skipf("libprotoc 29.2 is required for generator integration coverage, got %q", strings.TrimSpace(string(output)))
	}
	return protocPath
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	currentDirectory, err := os.Getwd()
	require.NoError(t, err)
	return filepath.Clean(filepath.Join(currentDirectory, "..", ".."))
}
