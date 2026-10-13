//go:build unix

package pluginhost

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	pluginhostv1 "github.com/AnixOps/anix-control/sdk/api/pluginhost/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// TestMain lets the test binary act as a built-in package host: the
// launcher executes the running binary (here the test binary) as
// `package-host <id>`, as Control runs `anix-control package-host <id>`.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == BuiltinHostCommand {
		os.Exit(runBuiltinTestHost(os.Args[2:]))
	}
	os.Exit(m.Run())
}

// builtinTestHost answers every dispatch with what the process received:
// its package id argument, the version the kernel passed and its argv[0].
type builtinTestHost struct {
	testHostServer
	packageID string
	version   string
}

func (h builtinTestHost) Dispatch(context.Context, *pluginhostv1.DispatchRequest) (*pluginhostv1.DispatchResponse, error) {
	exe, _ := os.Readlink("/proc/self/exe")
	body, err := json.Marshal(map[string]string{
		"package": h.packageID, "version": h.version, "argv0": os.Args[0], "exe": exe,
		"pid": fmt.Sprint(os.Getpid()),
	})
	if err != nil {
		return nil, err
	}
	return &pluginhostv1.DispatchResponse{StatusCode: 200, ResponseBody: body}, nil
}

func runBuiltinTestHost(args []string) int {
	if len(args) != 1 {
		_, _ = fmt.Fprintln(os.Stderr, "usage: package-host <id>")
		return 2
	}
	version := os.Getenv(BuiltinHostVersionEnvironment)
	socketPath := os.Getenv(hostSocketEnvironment)
	if version == "" || socketPath == "" {
		_, _ = fmt.Fprintln(os.Stderr, "the kernel host environment is missing")
		return 1
	}
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		return 1
	}
	server := grpc.NewServer()
	pluginhostv1.RegisterControlPackageHostServer(server, builtinTestHost{packageID: args[0], version: version})
	terminate := make(chan os.Signal, 1)
	signal.Notify(terminate, syscall.SIGTERM)
	go func() {
		<-terminate
		server.Stop()
	}()
	if err := server.Serve(listener); err != nil {
		return 1
	}
	return 0
}

func writeBuiltinArtifactRef(t *testing.T, packageID, version string) ArtifactRef {
	t.Helper()
	ref := writeVerifiedArtifactRef(t, packageID, version)
	ref.Builtin, ref.EntrypointPath, ref.EntrypointSHA256 = true, "", ""
	return ref
}

func TestBuiltinHostStartsThroughTheRunningBinary(t *testing.T) {
	manager, err := NewManager(ManagerConfig{
		RuntimeDir: filepath.Join(shortHostTempDir(t), "runtime"), StartupTimeout: 10 * time.Second,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Shutdown(context.Background())) })
	ref := writeBuiltinArtifactRef(t, "knowledge", "4.3.0")

	require.NoError(t, manager.Start(context.Background(), ref, 3))
	host := manager.hosts[hostKey(ref.PackageID, ref.Version)]
	require.NotNil(t, host)
	self, err := Self()
	require.NoError(t, err)
	require.Equal(t, self.Path, host.command.Path)
	require.Equal(t, []string{"anix-control", BuiltinHostCommand, "knowledge"}, host.command.Args)
	if runtime.GOOS == "linux" {
		require.Equal(t, "/proc/self/exe", host.command.Path)
		require.True(t, self.Pinned)
	}

	response, err := manager.Dispatch(context.Background(), DispatchInput{
		PackageID: ref.PackageID, Version: ref.Version, Generation: 3,
		RequestID: "builtin-1", RouteID: "knowledge.article.list", Method: "GET",
		PrincipalJSON: []byte(`{"actor_id":7}`), Deadline: time.Now().Add(5 * time.Second),
	})
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	var answer map[string]string
	require.NoError(t, json.Unmarshal(response.Body, &answer))
	require.Equal(t, "knowledge", answer["package"])
	require.Equal(t, "4.3.0", answer["version"])
	require.Equal(t, "anix-control", answer["argv0"])
	require.Equal(t, fmt.Sprint(host.command.Process.Pid), answer["pid"])
	if runtime.GOOS == "linux" {
		// The host runs the test binary's own image.
		parent, err := os.Readlink("/proc/self/exe")
		require.NoError(t, err)
		require.Equal(t, parent, answer["exe"])
	}

	require.NoError(t, manager.Stop(context.Background(), ref.PackageID, ref.Version, 4))
	require.Empty(t, manager.hosts)
}

func TestBuiltinArtifactRefMustNotNameAnEntrypoint(t *testing.T) {
	manager := newTestManager(t)
	ref := writeHostArtifactRef(t, "knowledge", "4.3.0")
	ref.Builtin = true

	err := manager.Start(context.Background(), ref, 3)

	require.ErrorIs(t, err, ErrHostIncompatible)
	require.Empty(t, manager.hosts)
}

func TestBuiltinArtifactRefStillVerifiesTheSignedFiles(t *testing.T) {
	manager := newTestManager(t)
	ref := writeBuiltinArtifactRef(t, "knowledge", "4.3.0")
	require.NoError(t, os.WriteFile(ref.ManifestPath, []byte(`{"id":"knowledge","version":"9.9.9","targets":["control"]}`), 0o600))

	err := manager.Start(context.Background(), ref, 3)

	require.ErrorIs(t, err, ErrHostIncompatible)
	require.Empty(t, manager.hosts)
}

func TestSelfExecutableRefusesAChangedBinary(t *testing.T) {
	self, err := Self()
	require.NoError(t, err)
	require.NoError(t, self.Check())
	require.Len(t, self.SHA256, 64)

	changed := self
	changed.modTime++
	require.ErrorIs(t, changed.Check(), ErrHostIncompatible)
	changed = self
	changed.inode++
	require.ErrorIs(t, changed.Check(), ErrHostIncompatible)
	require.ErrorIs(t, SelfExecutable{}.Check(), ErrHostIncompatible)

	// An unpinned path (a development machine) is hashed again.
	directory := t.TempDir()
	path := filepath.Join(directory, "anix-control")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	identity, digest, err := statAndHashExecutable(path)
	require.NoError(t, err)
	identity.Path, identity.SHA256 = path, digest
	require.NoError(t, identity.Check())
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o755))
	require.ErrorIs(t, identity.Check(), ErrHostIncompatible)
}
