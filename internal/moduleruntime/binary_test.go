package moduleruntime

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/modulepki"
	"github.com/stretchr/testify/require"
)

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

func buildGenericHost(t *testing.T) string {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	binary := filepath.Join(t.TempDir(), "control-host")
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=false",
		"-ldflags", "-X main.packageID=knowledge -X main.packageVersion=4.1.0", "-o", binary, "./packages/shared/controlhost")
	command.Dir = repoRoot
	command.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))
	return binary
}

type moduleProcess struct {
	command *exec.Cmd
	exited  chan error
	health  string
}

func startModuleProcess(t *testing.T, binary string, environment []string, health string) *moduleProcess {
	t.Helper()
	command := exec.Command(binary)
	command.Env = environment
	command.Stdout, command.Stderr = os.Stderr, os.Stderr
	require.NoError(t, command.Start())
	process := &moduleProcess{command: command, exited: make(chan error, 1), health: health}
	go func() { process.exited <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-process.exited:
		case <-time.After(5 * time.Second):
		}
	})
	return process
}

func (p *moduleProcess) ready() bool {
	response, err := http.Get("http://" + p.health + "/readyz") // #nosec G107 -- local test address.
	if err != nil {
		return false
	}
	_ = response.Body.Close()
	return response.StatusCode == http.StatusOK
}

func TestModuleBinaryRunsAsANetworkModule(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a module binary")
	}
	fixture := newRemoteFixture(t)
	binary := buildGenericHost(t)
	ctx := context.Background()

	directory := t.TempDir()
	bundle, err := fixture.authority.TrustBundlePEM(ctx)
	require.NoError(t, err)
	bundleFile := filepath.Join(directory, "bundle.pem")
	require.NoError(t, os.WriteFile(bundleFile, bundle, 0o600))
	credential, _, err := fixture.authority.CreateEnrollment(ctx, modulepki.EnrollmentRequest{PackageID: "knowledge", TTL: time.Hour})
	require.NoError(t, err)
	credentialFile := filepath.Join(directory, "credential")
	require.NoError(t, os.WriteFile(credentialFile, []byte(credential+"\n"), 0o600))
	certDir := filepath.Join(directory, "certs")
	listen, health := freeAddress(t), freeAddress(t)
	environment := []string{
		"ANIX_MODULE_MODE=remote", "ANIX_MODULE_KERNEL_ADDR=" + fixture.address, "ANIX_MODULE_CLUSTER=prod",
		"ANIX_MODULE_LISTEN_ADDR=" + listen, "ANIX_MODULE_ADVERTISE_ADDR=" + listen, "ANIX_MODULE_INSTANCE_ID=pod-bin",
		"ANIX_MODULE_CERT_DIR=" + certDir, "ANIX_MODULE_TRUST_BUNDLE_FILE=" + bundleFile,
		"ANIX_MODULE_ENROLL_CREDENTIAL_FILE=" + credentialFile, "ANIX_MODULE_HEALTH_ADDR=" + health,
		"ANIX_MODULE_SHUTDOWN_GRACE=200ms", "PATH=" + os.Getenv("PATH"),
	}
	first := startModuleProcess(t, binary, environment, health)

	require.NoError(t, fixture.supervisor.Start(ctx, writeArtifactRef(t), 5), "the module enrolls, binds and reports healthy")
	response, err := dispatchList(ctx, fixture.supervisor, 5)
	require.NoError(t, err)
	require.EqualValues(t, 200, response.StatusCode)
	<-fixture.calls
	require.Eventually(t, first.ready, 5*time.Second, 50*time.Millisecond, "/readyz reports the bound module")
	for _, name := range []string{"key.pem", "cert.pem", "bundle.pem"} {
		info, err := os.Stat(filepath.Join(certDir, name))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm(), name)
	}

	// SIGTERM: the module leaves rotation and exits cleanly.
	require.NoError(t, first.command.Process.Signal(syscall.SIGTERM))
	select {
	case err := <-first.exited:
		require.NoError(t, err)
	case <-time.After(15 * time.Second):
		t.Fatal("the module did not exit after SIGTERM")
	}

	// A restarted module reuses its stored certificate: the one-time
	// credential is already consumed.
	second := startModuleProcess(t, binary, environment, health)
	require.Eventually(t, second.ready, 10*time.Second, 50*time.Millisecond)
	require.Eventually(t, func() bool {
		response, err := dispatchList(ctx, fixture.supervisor, 5)
		if err != nil {
			return false
		}
		<-fixture.calls
		return response.StatusCode == 200
	}, 10*time.Second, 100*time.Millisecond, fmt.Sprintf("dispatch reaches the restarted module at %s", listen))
}
