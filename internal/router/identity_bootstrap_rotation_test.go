package router

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
	"github.com/AnixOps/anix-control/v4/internal/database"
	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/AnixOps/anix-control/v4/internal/plugincontrol"
	"github.com/AnixOps/anix-control/v4/internal/pluginhost"
	"github.com/AnixOps/anix-control/v4/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// rotationLifecycleDispatcher stands in for the host dispatcher on the
// lifecycle worker. Like the real one it cannot start a release whose trust
// root is retired ("verified artifact reference is unavailable"); it starts
// identity-platform at the version and generation it is asked for.
type rotationLifecycleDispatcher struct {
	calls []rotationLifecycleCall
}

type rotationLifecycleCall struct {
	PluginID, Version, Kind string
	Generation              uint64
}

func (d *rotationLifecycleDispatcher) ExecuteLifecycle(_ context.Context, pluginID, version string, request plugincontrol.LifecycleRequest) (json.RawMessage, error) {
	d.calls = append(d.calls, rotationLifecycleCall{PluginID: pluginID, Version: version, Kind: request.Kind, Generation: request.Generation})
	var release model.PluginRelease
	if err := database.GetDB().First(&release, "plugin_id = ? AND version = ?", pluginID, version).Error; err != nil {
		return nil, err
	}
	if _, err := service.VerifyStoredPluginRelease(database.GetDB(), release, nil); err != nil {
		return nil, pluginhost.ErrHostUnavailable
	}
	return json.RawMessage(`{}`), nil
}

// writeRotationIdentityBootstrap writes the identity-platform bootstrap trio
// of version, declaring the login route, signed by privateKey (a test key).
func writeRotationIdentityBootstrap(t *testing.T, privateKey ed25519.PrivateKey, version string) string {
	t.Helper()
	entrypoint := []byte("#!/bin/sh\nexit 0\n")
	migrations := []byte(`{"format":"anixops.migrations/v1","migrations":[],"package_id":"identity-platform","version":"` + version + `"}`)
	routes := []byte(`{"api_version":"v2","package_id":"identity-platform","routes":[{"method":"POST","legacy_path":"/api/v2/login","package_route":"identity.auth.login","envelope":"panel"}]}`)
	artifact := v2TestPackage(t, map[string][]byte{
		"bin/control-host": entrypoint, "compat/v2-routes.json": routes, "migrations/index.json": migrations,
	})
	artifactDigest := sha256.Sum256(artifact)
	entrypointDigest := sha256.Sum256(entrypoint)
	migrationsDigest := sha256.Sum256(migrations)
	routesDigest := sha256.Sum256(routes)
	manifest := service.PluginManifest{
		ID: "identity-platform", Name: "Identity Platform", Version: version, APIVersion: "v2", Publisher: "AnixOps", Targets: []string{"control"},
		ArtifactSHA256:    hex.EncodeToString(artifactDigest[:]),
		ControlEntrypoint: &service.PluginEntrypoint{Path: "bin/control-host", SHA256: hex.EncodeToString(entrypointDigest[:])},
		Migrations:        &service.PluginMigrations{Index: "migrations/index.json", SHA256: hex.EncodeToString(migrationsDigest[:])},
		CompatibilityRoutes: &service.PluginCompatibilityRoutes{
			Path: "compat/v2-routes.json", SHA256: hex.EncodeToString(routesDigest[:]),
		},
		RouteContractDigest: hex.EncodeToString(routesDigest[:]),
	}
	canonical, err := service.CanonicalPluginManifest(manifest)
	require.NoError(t, err)
	directory := t.TempDir()
	stem := "identity-platform-" + version
	require.NoError(t, os.WriteFile(filepath.Join(directory, stem+".manifest.json"), canonical, 0o644))
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(privateKey, canonical)) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, stem+".manifest.sig"), []byte(signature), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(directory, stem+".anxp"), artifact, 0o644))
	return directory
}

func rotationLogin(router *gin.Engine) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v2/login", strings.NewReader(`{"email":"u@example.test","password":"secret"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

// TestV2LoginRecoversAfterTheOfficialRootChangedWithoutAnAdministrator is the
// 4.1.0 -> 4.2.0 lockout of the staging rehearsal at the gateway: identity
// platform runs from an old-root release, Control restarts with the new root
// and a bootstrap package signed with it, and login comes back by itself
// while the other packages (old-root releases) stay down until imported.
func TestV2LoginRecoversAfterTheOfficialRootChangedWithoutAnAdministrator(t *testing.T) {
	oldRouter, cfg, host := setupV2PackageRouter(t)
	defer teardownTestRouter(t)
	db := database.GetDB()
	// cmd/server's schema migration creates the operation log before the bootstrap runs.
	require.NoError(t, db.AutoMigrate(&model.OperationLog{}))
	require.Equal(t, http.StatusOK, rotationLogin(oldRouter).Code, "baseline: login works on the old root")

	// The restart with the new root: the configured root becomes the active
	// one (bootstrapDatabase), the old-root hosts cannot start any more.
	newPublic, newPrivate, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	cfg.Plugins.OfficialPublicKey = base64.StdEncoding.EncodeToString(newPublic)
	config.Set(cfg)
	_, err = service.EnsurePluginTrustRoot(db, newPublic)
	require.NoError(t, err)
	bootstrapDirectory := writeRotationIdentityBootstrap(t, newPrivate, "4.1.0")
	beforeBootstrap := gin.New()
	Setup(beforeBootstrap, cfg)
	// What the lifecycle worker recorded when the old-root host failed to start.
	require.NoError(t, db.Model(&model.PluginInstallation{}).Where("target = ?", "control").
		Updates(map[string]any{"state": "failed", "last_error": "plugin host unavailable: verified artifact reference is unavailable"}).Error)
	for _, response := range []*httptest.ResponseRecorder{
		rotationLogin(beforeBootstrap),
		requestV2(t, beforeBootstrap, cfg, http.MethodGet, "/api/v2/user/knowledge"),
	} {
		require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())
		require.Contains(t, response.Body.String(), "package_route_not_found")
	}

	// The bootstrap of the new start moves identity-platform; everything else
	// stays as it is.
	require.NoError(t, service.BootstrapIdentityPlatformPackage(db, cfg.Plugins.OfficialPublicKey, bootstrapDirectory))
	var moved model.PluginInstallation
	require.NoError(t, db.First(&moved, "plugin_id = ? AND target = ?", "identity-platform", "control").Error)
	require.Equal(t, "4.1.0", moved.DesiredVersion)
	require.EqualValues(t, 8, moved.LifecycleGeneration)
	require.Equal(t, "pending", moved.State)

	// The server then starts its lifecycle worker, whose restart
	// reconciliation queues plugin.enable at the installation's desired
	// version and generation.
	dispatcher := &rotationLifecycleDispatcher{}
	worker, err := plugincontrol.NewOperationWorker(db, dispatcher)
	require.NoError(t, err)
	_, err = worker.QueueReconciliation("boot-with-the-new-root")
	require.NoError(t, err)
	_, err = worker.RunOnce(context.Background())
	require.NoError(t, err)

	var started []rotationLifecycleCall
	for _, call := range dispatcher.calls {
		if call.PluginID == "identity-platform" {
			started = append(started, call)
		}
	}
	require.Equal(t, []rotationLifecycleCall{{PluginID: "identity-platform", Version: "4.1.0", Kind: "plugin.enable", Generation: 8}}, started)
	require.NoError(t, db.First(&moved, moved.ID).Error)
	require.Equal(t, "4.1.0", moved.DesiredVersion)
	require.Equal(t, "4.1.0", moved.ObservedVersion)
	require.Equal(t, "4.0.0", moved.PreviousVersion)
	require.Equal(t, "healthy", moved.State)
	require.Empty(t, moved.LastError)

	afterWorker := gin.New()
	Setup(afterWorker, cfg)
	host.lastRouteID, host.lastGeneration = "", 0
	login := rotationLogin(afterWorker)
	require.Equal(t, http.StatusOK, login.Code, login.Body.String())
	require.Equal(t, "identity.auth.login", host.lastRouteID)
	require.EqualValues(t, 8, host.lastGeneration)

	// The other packages were not touched: old-root releases, failed, their
	// routes answer 404 until the operator imports them with the active root.
	var knowledge model.PluginInstallation
	require.NoError(t, db.First(&knowledge, "plugin_id = ? AND target = ?", "knowledge", "control").Error)
	require.Equal(t, "4.0.0", knowledge.DesiredVersion)
	require.Equal(t, "failed", knowledge.State)
	require.EqualValues(t, 7, knowledge.LifecycleGeneration)
	response := requestV2(t, afterWorker, cfg, http.MethodGet, "/api/v2/user/knowledge")
	require.Equal(t, http.StatusNotFound, response.Code, response.Body.String())

	// A further start changes nothing: the installation is on the active root.
	require.NoError(t, service.BootstrapIdentityPlatformPackage(db, cfg.Plugins.OfficialPublicKey, bootstrapDirectory))
	var again model.PluginInstallation
	require.NoError(t, db.First(&again, moved.ID).Error)
	require.EqualValues(t, 8, again.LifecycleGeneration)
	require.Equal(t, "healthy", again.State)
	require.Equal(t, http.StatusOK, rotationLogin(afterWorker).Code)
}
