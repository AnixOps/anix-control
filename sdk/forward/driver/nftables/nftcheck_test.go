package nftables_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// nftCheckRequired makes a missing or unusable nft fail the check instead
// of skipping it (CI sets it where it installs nftables and runs as root).
const nftCheckRequired = "ANIXOPS_NFT_CHECK"

// nftChecker answers a function that runs `nft -c -f` on a script. nft -c
// talks to the kernel over netlink and needs CAP_NET_ADMIN; as root it runs
// in a fresh network namespace (unshare -n) when it can, so not even the
// check sees the host's ruleset. It never applies anything.
func nftChecker(t *testing.T) func(t *testing.T, script []byte) {
	t.Helper()
	required := os.Getenv(nftCheckRequired) == "1"
	unavailable := func(format string, args ...any) {
		t.Helper()
		if required {
			t.Fatalf(format, args...)
		}
		t.Skipf(format, args...)
	}
	nft, err := exec.LookPath("nft")
	if err != nil {
		unavailable("nft not installed: %v", err)
	}
	argv := []string{nft, "-c", "-f"}
	if os.Geteuid() == 0 {
		if unshare, err := exec.LookPath("unshare"); err == nil {
			if exec.Command(unshare, "-n", "true").Run() == nil { // #nosec G204 -- fixed arguments
				argv = append([]string{unshare, "-n"}, argv...)
			}
		}
	}
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe.nft")
	if err := os.WriteFile(probe, []byte("table inet anixops_fwd_probe {\n}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(argv[0], append(argv[1:], probe)...).CombinedOutput(); err != nil { // #nosec G204 -- nft and unshare from PATH with fixed flags
		unavailable("nft -c cannot run here (it needs CAP_NET_ADMIN): %v: %s", err, bytes.TrimSpace(out))
	}
	return func(t *testing.T, script []byte) {
		t.Helper()
		f := filepath.Join(t.TempDir(), "check.nft")
		if err := os.WriteFile(f, script, 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command(argv[0], append(argv[1:], f)...).CombinedOutput(); err != nil { // #nosec G204 -- as above
			t.Fatalf("%s: %v\n%s", strings.Join(argv, " "), err, out)
		}
	}
}

// TestNFTCheckGoldens checks every golden script with nft -c.
func TestNFTCheckGoldens(t *testing.T) {
	check := nftChecker(t)
	files, err := filepath.Glob(filepath.Join(goldenDir, "*.nft"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no goldens: %v", err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			b, err := os.ReadFile(f) // #nosec G304 -- a golden of this package
			if err != nil {
				t.Fatal(err)
			}
			check(t, b)
		})
	}
}

// TestNFTCheckConformanceStates checks the render of every conformance
// state case with nft -c.
func TestNFTCheckConformanceStates(t *testing.T) {
	check := nftChecker(t)
	b := builder(t)
	d := newDriver(t, nil)
	for _, c := range conformance.StateCases() {
		t.Run(c.Name, func(t *testing.T) {
			a, err := d.Render(conformance.State(b.Top.NodeRef, 1, c.Hops(b)...))
			if err != nil {
				t.Fatal(err)
			}
			check(t, a.Content)
		})
	}
}
