//go:build linux

package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"testing"
)

// The suite runs only with ANIXOPS_FORWARD_E2E=1 as root; with the variable
// set, a missing prerequisite fails instead of skipping, so CI cannot pass
// by skipping everything.
const (
	envE2E = "ANIXOPS_FORWARD_E2E"
	// envLogDir is where a failed test writes the state of its namespaces
	// (rulesets, qdiscs, addresses, routes, server logs); without it the
	// state goes to the test log.
	envLogDir = "ANIXOPS_FORWARD_E2E_LOGDIR"
	// envServe turns the test binary into an echo server (serve), which the
	// tests start inside a target namespace with `ip netns exec`.
	envServe = "ANIXOPS_FORWARD_E2E_SERVE"
)

func TestMain(m *testing.M) {
	if spec := os.Getenv(envServe); spec != "" {
		os.Exit(serve(spec))
	}
	if os.Getenv(envE2E) == "1" && os.Geteuid() == 0 {
		removeStaleNamespaces(func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, format+"\n", args...)
		})
	}
	os.Exit(m.Run())
}

// requireE2E skips unless the suite is enabled and fails when it is
// enabled without what it needs.
func requireE2E(t testing.TB) {
	t.Helper()
	if os.Getenv(envE2E) != "1" {
		t.Skipf("set %s=1 (as root) to run the multi-namespace forwarding tests", envE2E)
	}
	if os.Geteuid() != 0 {
		t.Fatalf("%s=1 needs root (CAP_NET_ADMIN, and CAP_SYS_ADMIN for ip netns add)", envE2E)
	}
	for _, bin := range []string{"ip", "nft", "tc", "sysctl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Fatalf("%s=1 needs %s: %v", envE2E, bin, err)
		}
	}
}
