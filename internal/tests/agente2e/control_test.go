package agente2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/AnixOps/anix-control/v5/internal/service"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	adminEmail    = "agent-e2e@anixops.test"
	adminPassword = "AgentE2E!2026"
)

// control is the real Control process of the suite with its config, its
// database (shared with the test, which reads what Control recorded and
// seeds rows the way the admin routes would) and an admin API client.
type control struct {
	proc       *process
	configPath string
	apiPort    int
	grpcPort   int
	db         *gorm.DB
	dbDriver   string
	sqlitePath string
	token      string
}

func (c *control) URL() string      { return "http://127.0.0.1:" + strconv.Itoa(c.apiPort) }
func (c *control) GRPCAddr() string { return "127.0.0.1:" + strconv.Itoa(c.grpcPort) }

func quoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func writeControlConfig(t *testing.T, dir string, keys testPKI, identityPackageDir string, apiPort, grpcPort int, postgresDSN string) (string, string) {
	t.Helper()
	database := ""
	sqlitePath := ""
	if postgresDSN != "" {
		database = "  driver: postgres\n  dsn: " + quoted(postgresDSN) + "\n  log_level: silent\n"
	} else {
		sqlitePath = filepath.Join(dir, "control.db")
		database = "  driver: sqlite\n  database: " + quoted(sqlitePath) + "\n  log_level: silent\n"
	}
	// The operator keys, and the deprecated keys this release still reads:
	// the database, the test's package key and identity package, the gRPC
	// listener without a domain (its own test-CA certificate), the CA key
	// (adopted into data_dir/root.key at the start), the first
	// administrator, closed registration and the inert flux runtime.
	config := fmt.Sprintf(`listen: "127.0.0.1:%d"
data_dir: %s
frontend:
  enable: false
database:
%sauth:
  registration:
    enabled: false
plugins:
  official_public_key: %s
  identity_bootstrap_package_dir: %s
grpc:
  enabled: true
  host: "127.0.0.1"
  port: %d
  tls_cert_file: %s
  tls_key_file: %s
module_runtime:
  ca_kek: %s
admin:
  email: %s
  password: %s
forward_runtime:
  backend: clean_agent
  clean_agent:
    legacy_bridge_enabled: false
`, apiPort, quoted(filepath.Join(dir, "data")), database, quoted(keys.SigningPublicKey), quoted(identityPackageDir), grpcPort,
		quoted(keys.ServerCertFile), quoted(keys.ServerKeyFile), quoted(keys.CAKEK), quoted(adminEmail), quoted(adminPassword))
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
	return path, sqlitePath
}

func openTestDB(t *testing.T, postgresDSN, sqlitePath string) *gorm.DB {
	t.Helper()
	var dialector gorm.Dialector
	if postgresDSN != "" {
		dialector = postgres.Open(postgresDSN)
	} else {
		dialector = sqlite.Open(sqlitePath + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)")
	}
	db, err := gorm.Open(dialector, &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// Start runs Control and waits for /health.
func (c *control) Start(t *testing.T) {
	t.Helper()
	c.proc.Start(t)
	eventually(t, 60*time.Second, func() (bool, string) {
		if !c.proc.Running() {
			t.Fatalf("Control exited during startup\n%s", c.proc.Tail())
		}
		response, err := http.Get(c.URL() + "/health")
		if err != nil {
			return false, err.Error()
		}
		_ = response.Body.Close()
		return response.StatusCode == http.StatusOK, response.Status
	}, c.proc)
}

// Login signs in as the bootstrap administrator (the identity-platform
// package answers once its host runs).
func (c *control) Login(t *testing.T) {
	t.Helper()
	eventually(t, 60*time.Second, func() (bool, string) {
		status, body := c.do(t, http.MethodPost, "/api/v2/login", map[string]string{"email": adminEmail, "password": adminPassword}, false)
		if status != http.StatusOK {
			return false, fmt.Sprintf("%d %s", status, body)
		}
		var answer struct {
			Data struct {
				Token string `json:"token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &answer); err != nil || answer.Data.Token == "" {
			return false, string(body)
		}
		c.token = answer.Data.Token
		return true, ""
	}, c.proc)
}

func (c *control) do(t *testing.T, method, path string, body any, auth bool) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequest(method, c.URL()+path, reader)
	require.NoError(t, err)
	request.Header.Set("Content-Type", "application/json")
	if auth {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return 0, []byte(err.Error())
	}
	defer func() { _ = response.Body.Close() }()
	answer, _ := io.ReadAll(response.Body)
	return response.StatusCode, answer
}

// API calls an admin route and decodes the answer into out (when not
// nil); a status other than 2xx fails the test.
func (c *control) API(t *testing.T, method, path string, body any, out any) {
	t.Helper()
	status, answer := c.do(t, method, path, body, true)
	require.Truef(t, status >= 200 && status < 300, "%s %s answered %d: %s", method, path, status, answer)
	if out != nil {
		require.NoError(t, json.Unmarshal(answer, out), "%s %s: %s", method, path, answer)
	}
}

// TryAPI is API without failing: the status and the raw answer.
func (c *control) TryAPI(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	return c.do(t, method, path, body, true)
}

// InstallPackages puts the signed packages where the start-up bootstrap
// puts the built-in ones: a registered release, its artifact and an enabled
// Control installation, written while Control is stopped; the restart
// starts their hosts, and InstallPackages signs in again and waits until
// they are healthy.
// Control has no route that imports or installs a package any more (the
// built-in packages install at every start), and this suite's packages are
// full packages signed with the suite's key.
func (c *control) InstallPackages(t *testing.T, publicKey string, pkgs ...signedPackage) {
	t.Helper()
	key, err := service.ParseOfficialPluginPublicKey(publicKey)
	require.NoError(t, err)
	c.proc.Stop(t, 10*time.Second)
	for _, pkg := range pkgs {
		release, err := service.RegisterPluginRelease(c.db, string(pkg.Manifest), pkg.Signature, key)
		require.NoError(t, err, pkg.ID)
		_, err = service.StorePluginArtifact(c.db, release.ID, pkg.Artifact)
		require.NoError(t, err, pkg.ID)
		require.NoError(t, c.db.Transaction(func(tx *gorm.DB) error {
			if err := service.LockPluginInstallationTarget(tx, "control"); err != nil {
				return err
			}
			if err := service.ValidatePluginInstallationPlan(tx, *release, "control", true, 0); err != nil {
				return err
			}
			return tx.Create(&model.PluginInstallation{
				PluginID: pkg.ID, Target: "control", DesiredVersion: pkg.Version, State: "pending", Enabled: true, LifecycleGeneration: 1,
			}).Error
		}), pkg.ID)
	}
	c.Start(t)
	c.Login(t)
	for _, pkg := range pkgs {
		c.WaitInstallation(t, pkg.ID, "control", pkg.Version)
	}
}

// InstallAgentPackage makes the package's Agent installation, enabled at
// the release InstallPackages registered, the way the bootstrap makes a
// Control one; the node assignments then deploy it (once Agent plugin
// dispatch is on, in M4).
func (c *control) InstallAgentPackage(t *testing.T, pkg signedPackage) {
	t.Helper()
	require.NoError(t, c.db.Transaction(func(tx *gorm.DB) error {
		if err := service.LockPluginInstallationTarget(tx, "agent"); err != nil {
			return err
		}
		var release model.PluginRelease
		if err := tx.Take(&release, "plugin_id = ? AND version = ?", pkg.ID, pkg.Version).Error; err != nil {
			return err
		}
		if err := service.ValidatePluginInstallationPlan(tx, release, "agent", true, 0); err != nil {
			return err
		}
		return tx.Create(&model.PluginInstallation{
			PluginID: pkg.ID, Target: "agent", DesiredVersion: pkg.Version, State: "pending", Enabled: true, LifecycleGeneration: 1,
		}).Error
	}), pkg.ID)
}

type installation struct {
	ID              uint   `json:"id"`
	PluginID        string `json:"plugin_id"`
	Target          string `json:"target"`
	State           string `json:"state"`
	ObservedVersion string `json:"observed_version"`
	Enabled         bool   `json:"enabled"`
	LastError       string `json:"last_error"`
}

func (c *control) Installations(t *testing.T) []installation {
	t.Helper()
	var answer struct {
		Data []installation `json:"data"`
	}
	c.API(t, http.MethodGet, "/api/v3/plugin-installations", nil, &answer)
	return answer.Data
}

func (c *control) WaitInstallation(t *testing.T, pluginID, target, version string) installation {
	t.Helper()
	var found installation
	eventually(t, 60*time.Second, func() (bool, string) {
		for _, row := range c.Installations(t) {
			if row.PluginID == pluginID && row.Target == target {
				found = row
				return row.State == "healthy" && row.ObservedVersion == version && row.Enabled, fmt.Sprintf("%+v", row)
			}
		}
		return false, pluginID + " not installed"
	}, c.proc)
	return found
}

// Metric reads one sample of Control's /metrics: the sum of every series
// of name whose labels contain all of match.
func (c *control) Metric(t *testing.T, name string, match ...string) float64 {
	t.Helper()
	response, err := http.Get(c.URL() + "/metrics")
	if err != nil {
		return -1
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	total := 0.0
	for _, line := range strings.Split(string(body), "\n") {
		if !strings.HasPrefix(line, name) || strings.HasPrefix(line, "#") {
			continue
		}
		rest := strings.TrimPrefix(line, name)
		if rest == "" || (rest[0] != '{' && rest[0] != ' ') {
			continue
		}
		matched := true
		for _, m := range match {
			if !strings.Contains(rest, m) {
				matched = false
			}
		}
		if !matched {
			continue
		}
		fields := strings.Fields(rest)
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err == nil {
			total += value
		}
	}
	return total
}

// freshPostgresDatabase creates a throwaway database on the server of the
// given DSN (its role needs CREATEDB) and answers the DSN of it; the
// database is dropped after the run.
func freshPostgresDatabase(t *testing.T, base string) string {
	t.Helper()
	admin, err := gorm.Open(postgres.Open(base), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	name := fmt.Sprintf("agente2e_%d", time.Now().UnixNano())
	require.NoError(t, admin.Exec("CREATE DATABASE "+name).Error)
	t.Cleanup(func() {
		_ = admin.Exec("DROP DATABASE IF EXISTS " + name + " WITH (FORCE)").Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return base + " dbname=" + name
}
