//go:build linux

// Package builtinpackages runs the real Control binary with the official
// packages built in: the slim signed set build_package.py --builtin writes,
// embedded with a go build overlay over internal/builtinpkg/dist, and the
// hosts started by executing the binary again (package-host). It checks
// the start-up bootstrap of docs/architecture/plugin-kernel-contract.md
// ("Built-in Packages"): a cold start installs every package, a restart
// with the same binary changes nothing, a new version upgrades and runs the
// package migrations again, and a package that does not verify leaves the
// others and Control available.
package builtinpackages

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	adminEmail    = "builtin@anixops.test"
	adminPassword = "BuiltinPackages!2026"
)

// packageCount is every official package of this tree (packages/builtin):
// 12 in a community checkout, 15 with the commercial packages overlaid.
var packageCount = countPackages()

func countPackages() int {
	manifests, err := filepath.Glob(filepath.Join("..", "..", "..", "packages", "*", "manifest.template.json"))
	if err != nil {
		panic(err)
	}
	return len(manifests)
}

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	require.NoError(t, err)
	return root
}

func requireTools(t *testing.T) {
	t.Helper()
	for _, tool := range []string{"python3", "openssl", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
}

func run(t *testing.T, dir string, env []string, name string, args ...string) []byte {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = dir
	command.Env = append(os.Environ(), env...)
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s %s\n%s", name, strings.Join(args, " "), output)
	return output
}

// signingKey is the test's package signing key and its raw public key, the
// value of plugins.official_public_key.
type signingKey struct {
	path    string
	encoded string
}

func newSigningKey(t *testing.T, dir string) signingKey {
	t.Helper()
	path := filepath.Join(dir, "signing-key.pem")
	run(t, dir, nil, "openssl", "genpkey", "-algorithm", "ED25519", "-out", path)
	require.NoError(t, os.Chmod(path, 0o600))
	der := run(t, dir, nil, "openssl", "pkey", "-in", path, "-pubout", "-outform", "DER")
	require.Len(t, der, 44)
	return signingKey{path: path, encoded: base64.StdEncoding.EncodeToString(der[12:])}
}

// buildPackageSet writes the built-in set of version, as the release does.
func buildPackageSet(t *testing.T, dir string, key signingKey, version string) string {
	t.Helper()
	out := filepath.Join(dir, "set-"+version)
	run(t, repoRoot(t), nil, "python3", "packages/shared/build_package.py",
		"--all", "--builtin", "--version", version, "--out", out, "--signing-key", key.path)
	return out
}

// buildControl compiles cmd/server with setDir in place of
// internal/builtinpkg/dist, without touching the checkout.
func buildControl(t *testing.T, dir, setDir, name string) string {
	t.Helper()
	root := repoRoot(t)
	entries, err := os.ReadDir(setDir)
	require.NoError(t, err)
	replace := map[string]string{}
	for _, entry := range entries {
		replace[filepath.Join(root, "internal", "builtinpkg", "dist", entry.Name())] = filepath.Join(setDir, entry.Name())
	}
	overlay, err := json.Marshal(map[string]any{"Replace": replace})
	require.NoError(t, err)
	overlayPath := filepath.Join(dir, name+".overlay.json")
	require.NoError(t, os.WriteFile(overlayPath, overlay, 0o600))
	binary := filepath.Join(dir, name)
	run(t, root, []string{"GOWORK=off", "CGO_ENABLED=0"}, "go", "build", "-trimpath", "-overlay", overlayPath, "-o", binary, "./cmd/server")
	return binary
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	return listener.Addr().(*net.TCPAddr).Port
}

type control struct {
	t      *testing.T
	dir    string
	config string
	port   int
	db     *gorm.DB
	cmd    *exec.Cmd
	exited chan struct{}
	output *syncBuffer
	// binary is the Control the latest start ran; claimed is set once the
	// first administrator claimed this database.
	binary  string
	claimed bool
}

type syncBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *syncBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(data)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func newControl(t *testing.T, key signingKey) *control {
	t.Helper()
	dir := t.TempDir()
	port := freePort(t)
	sqlitePath := filepath.Join(dir, "control.db")
	// The operator keys, and the deprecated keys this release still reads
	// (database, the package key of a development build, the web app
	// switch). The root key is generated in data_dir at the first start; the
	// first administrator claims the panel (claim).
	config := fmt.Sprintf(`listen: "127.0.0.1:%d"
data_dir: %q
frontend:
  enable: false
database:
  driver: sqlite
  database: %q
  log_level: silent
plugins:
  official_public_key: %q
`, port, filepath.Join(dir, "data"), sqlitePath, key.encoded)
	configPath := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte(config), 0o600))
	db, err := gorm.Open(sqlite.Open(sqlitePath+"?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return &control{t: t, dir: dir, config: configPath, port: port, db: db}
}

func (c *control) url(path string) string { return fmt.Sprintf("http://127.0.0.1:%d%s", c.port, path) }

// start runs binary and waits until /readyz answers.
func (c *control) start(binary string) {
	c.t.Helper()
	c.output = &syncBuffer{}
	c.binary = binary
	c.cmd = exec.Command(binary, "-config", c.config)
	c.cmd.Dir = c.dir
	c.cmd.Env = c.environment()
	c.cmd.Stdout, c.cmd.Stderr = c.output, c.output
	c.cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(c.t, c.cmd.Start())
	exited := make(chan struct{})
	go func() {
		_ = c.cmd.Wait()
		close(exited)
	}()
	c.t.Cleanup(func() { c.stopWith(exited) })
	c.eventually(60*time.Second, "Control ready", func() bool {
		select {
		case <-exited:
			c.t.Fatalf("Control exited during startup\n%s", c.output.String())
		default:
		}
		response, err := http.Get(c.url("/readyz"))
		if err != nil {
			return false
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusOK
	})
	c.exited = exited
}

func (c *control) stop() {
	c.t.Helper()
	c.stopWith(c.exited)
}

func (c *control) stopWith(exited chan struct{}) {
	if c.cmd == nil || c.cmd.Process == nil || exited == nil {
		return
	}
	select {
	case <-exited:
		return
	default:
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-exited:
	case <-time.After(60 * time.Second):
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		<-exited
	}
}

func (c *control) eventually(timeout time.Duration, what string, condition func() bool) {
	c.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	c.t.Fatalf("timed out waiting for %s\n%s", what, c.output.String())
}

func (c *control) installations() map[string]model.PluginInstallation {
	c.t.Helper()
	var installations []model.PluginInstallation
	require.NoError(c.t, c.db.Where("target = ?", "control").Find(&installations).Error)
	out := map[string]model.PluginInstallation{}
	for _, installation := range installations {
		out[installation.PluginID] = installation
	}
	return out
}

// waitHealthy waits until every package but skip is healthy at version.
func (c *control) waitHealthy(version string, skip ...string) map[string]model.PluginInstallation {
	c.t.Helper()
	want := packageCount - len(skip)
	var installations map[string]model.PluginInstallation
	c.eventually(180*time.Second, fmt.Sprintf("%d packages healthy at %s", want, version), func() bool {
		installations = c.installations()
		healthy := 0
		for id, installation := range installations {
			for _, skipped := range skip {
				if id == skipped {
					c.t.Fatalf("%s has an installation: %+v", id, installation)
				}
			}
			if installation.State == "failed" {
				c.t.Fatalf("%s failed: %s\n%s", id, installation.LastError, c.output.String())
			}
			if installation.Enabled && installation.State == "healthy" && installation.ObservedVersion == version && installation.DesiredVersion == version {
				healthy++
			}
		}
		return healthy == want
	})
	return installations
}

func (c *control) environment() []string {
	return []string{"PATH=" + os.Getenv("PATH"), "HOME=" + c.dir, "TMPDIR=" + c.dir, "TZ=UTC"}
}

// claimToken finds the token in what `admin claim-link` printed.
var claimToken = regexp.MustCompile(`anixclaim_[A-Za-z0-9_-]{43}`)

// claim creates the first administrator as a fresh panel gets it: the
// running binary's `admin claim-link`, then POST /api/v5/claim.
func (c *control) claim() {
	c.t.Helper()
	command := exec.Command(c.binary, "admin", "claim-link")
	command.Dir = c.dir
	command.Env = append(c.environment(), "ANIX_CONTROL_CONFIG="+c.config)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	require.NoError(c.t, command.Run(), "admin claim-link\n%s", stderr.String())
	token := claimToken.FindString(stdout.String())
	require.NotEmpty(c.t, token, "admin claim-link printed no link:\n%s", stdout.String())
	body, _ := json.Marshal(map[string]string{"token": token, "email": adminEmail, "password": adminPassword})
	response, err := http.Post(c.url("/api/v5/claim"), "application/json", bytes.NewReader(body))
	require.NoError(c.t, err)
	defer func() { _ = response.Body.Close() }()
	answer, _ := io.ReadAll(response.Body)
	require.Equal(c.t, http.StatusCreated, response.StatusCode, string(answer))
	c.claimed = true
}

func (c *control) login() {
	c.t.Helper()
	if !c.claimed {
		c.claim()
	}
	c.eventually(60*time.Second, "an administrator login", func() bool {
		body, _ := json.Marshal(map[string]string{"email": adminEmail, "password": adminPassword})
		response, err := http.Post(c.url("/api/v2/login"), "application/json", bytes.NewReader(body))
		if err != nil {
			return false
		}
		defer func() { _ = response.Body.Close() }()
		answer, _ := io.ReadAll(response.Body)
		return response.StatusCode == http.StatusOK && strings.Contains(string(answer), `"token"`)
	})
}

// hostProcesses maps the package id of every child process that runs
// `anix-control package-host <id>` to the executable it runs.
func (c *control) hostProcesses() map[string]string {
	c.t.Helper()
	entries, err := os.ReadDir("/proc")
	require.NoError(c.t, err)
	hosts := map[string]string{}
	parent := fmt.Sprint(c.cmd.Process.Pid)
	for _, entry := range entries {
		stat, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			continue
		}
		// pid (comm) state ppid ...
		fields := strings.Fields(string(stat[bytes.LastIndexByte(stat, ')')+1:]))
		if len(fields) < 2 || fields[1] != parent {
			continue
		}
		cmdline, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "cmdline"))
		if err != nil {
			continue
		}
		args := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
		if len(args) != 3 || args[0] != "anix-control" || args[1] != "package-host" {
			continue
		}
		exe, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		require.NoError(c.t, err)
		hosts[args[2]] = exe
	}
	return hosts
}

func (c *control) count(value any, query string, args ...any) int64 {
	c.t.Helper()
	var count int64
	require.NoError(c.t, c.db.Model(value).Where(query, args...).Count(&count).Error)
	return count
}

func TestBuiltinPackagesInstallUpgradeAndSurviveAFailure(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the Control binary")
	}
	requireTools(t)
	work := t.TempDir()
	key := newSigningKey(t, work)
	setV1 := buildPackageSet(t, work, key, "9.1.0")
	setV2 := buildPackageSet(t, work, key, "9.2.0")
	controlV1 := buildControl(t, work, setV1, "anix-control-v1")
	controlV2 := buildControl(t, work, setV2, "anix-control-v2")

	panel := newControl(t, key)

	// Cold start: every package is registered, installed and started.
	panel.start(controlV1)
	cold := panel.waitHealthy("9.1.0")
	panel.login()
	for id, installation := range cold {
		require.EqualValues(t, 1, installation.LifecycleGeneration, id)
	}
	require.EqualValues(t, packageCount, panel.count(&model.PluginRelease{}, "1 = 1"))
	require.EqualValues(t, packageCount, panel.count(&model.OperationLog{}, "action = ?", "builtin_package_installation"))
	require.EqualValues(t, 1, panel.count(&model.PackageMigrationRun{}, "package_id = ? AND generation = ? AND state = ?",
		"identity-platform", 1, model.PackageMigrationStateCompleted), "identity-platform migrates on its first start")
	output := panel.output.String()
	require.Contains(t, output, fmt.Sprintf("Built-in packages: %d embedded, 0 failed", packageCount))
	// Every host is the Control binary itself, started as package-host.
	hosts := panel.hostProcesses()
	require.Len(t, hosts, packageCount)
	for id, exe := range hosts {
		require.Equal(t, controlV1, exe, id)
	}
	panel.stop()

	// A restart with the same binary changes nothing.
	panel.start(controlV1)
	restarted := panel.waitHealthy("9.1.0")
	for id, installation := range restarted {
		require.Equal(t, cold[id].LifecycleGeneration, installation.LifecycleGeneration, id)
	}
	require.EqualValues(t, packageCount, panel.count(&model.PluginRelease{}, "1 = 1"))
	require.EqualValues(t, packageCount, panel.count(&model.OperationLog{}, "action = ?", "builtin_package_installation"))
	require.EqualValues(t, 1, panel.count(&model.PackageMigrationRun{}, "package_id = ?", "identity-platform"))
	require.NotContains(t, panel.output.String(), "Built-in package identity-platform 9.1.0 installed")
	panel.stop()

	// A new binary upgrades every package and runs the migrations of the
	// new lifecycle generation.
	panel.start(controlV2)
	upgraded := panel.waitHealthy("9.2.0")
	panel.login()
	for id, installation := range upgraded {
		require.EqualValues(t, 2, installation.LifecycleGeneration, id)
		require.Equal(t, "9.1.0", installation.PreviousVersion, id)
	}
	require.EqualValues(t, 2*packageCount, panel.count(&model.PluginRelease{}, "1 = 1"))
	require.EqualValues(t, 1, panel.count(&model.PackageMigrationRun{}, "package_id = ? AND generation = ? AND state = ?",
		"identity-platform", 2, model.PackageMigrationStateCompleted))
	require.Contains(t, panel.output.String(), "Built-in package identity-platform 9.2.0 upgraded")
	panel.stop()
}

func TestBuiltinPackageThatDoesNotVerifyLeavesTheOthersAndControlAvailable(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and runs the Control binary")
	}
	requireTools(t)
	work := t.TempDir()
	key := newSigningKey(t, work)
	set := buildPackageSet(t, work, key, "9.1.0")
	// One package's artifact no longer matches its signed manifest.
	artifact := filepath.Join(set, "plan-9.1.0.anxp")
	data, err := os.ReadFile(artifact)
	require.NoError(t, err)
	data[len(data)-1] ^= 0xff
	require.NoError(t, os.WriteFile(artifact, data, 0o644))
	binary := buildControl(t, work, set, "anix-control-tampered")

	panel := newControl(t, key)
	panel.start(binary)
	panel.waitHealthy("9.1.0", "plan")
	panel.login()
	require.EqualValues(t, 0, panel.count(&model.PluginRelease{}, "plugin_id = ?", "plan"))
	output := panel.output.String()
	require.Contains(t, output, "built-in package plan 9.1.0 invalid")
	require.Contains(t, output, fmt.Sprintf("Built-in packages: %d embedded, 1 failed", packageCount))
	panel.stop()
}

// The package-host command runs only hosts this binary links, at the
// version it embeds.
func TestPackageHostCommandChecksTheEmbeddedVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the Control binary")
	}
	requireTools(t)
	work := t.TempDir()
	key := newSigningKey(t, work)
	binary := buildControl(t, work, buildPackageSet(t, work, key, "9.1.0"), "anix-control")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	for _, test := range []struct {
		args    []string
		version string
		code    int
		message string
	}{
		{[]string{"package-host"}, "", 2, "usage"},
		{[]string{"package-host", "no-such-package"}, "", 2, "no built-in host"},
		{[]string{"package-host", "plan"}, "9.0.0", 1, "embeds plan 9.1.0, not 9.0.0"},
		{[]string{"package-host", "plan"}, "9.1.0", 1, "control host socket is required"},
	} {
		command := exec.CommandContext(ctx, binary, test.args...)
		command.Env = []string{"PATH=" + os.Getenv("PATH")}
		if test.version != "" {
			command.Env = append(command.Env, "ANIX_CONTROL_HOST_PACKAGE_VERSION="+test.version)
		}
		output, err := command.CombinedOutput()
		exitErr, ok := err.(*exec.ExitError)
		require.True(t, ok, "%v: %v %s", test.args, err, output)
		require.Equal(t, test.code, exitErr.ExitCode(), "%v: %s", test.args, output)
		require.Contains(t, string(output), test.message, test.args)
		require.NotContains(t, string(output), "Loading config", "a package host reads no configuration")
	}
}
