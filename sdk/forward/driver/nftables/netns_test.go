package nftables

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
)

// The real-kernel harness: every test gets a fresh network namespace
// (`ip netns add`), and the driver runs nft and tc inside it through
// `ip netns exec`, so nothing touches the host's own ruleset or qdiscs.
// It needs root (CAP_NET_ADMIN and CAP_SYS_ADMIN for the namespace) and
// runs only with ANIXOPS_NFT_E2E=1, which also makes a missing
// prerequisite fail instead of skip. CI runs it under sudo in Backend
// Tests shard 1.
const nftE2E = "ANIXOPS_NFT_E2E"

// limitIface is the dummy interface of every namespace that the driver
// shapes; foreignIface carries a foreign qdisc the driver must not touch.
const (
	limitIface   = "lim0"
	foreignIface = "frn0"
)

// netns is one throwaway network namespace.
type netns struct {
	name string
	ip   string
}

// Traffic addresses: a client namespace joined to the driver's by a veth
// pair, and the conformance topology's upstreams routed out of the dummy
// limit interface, which drops what it sends: forwarded packets pass the
// prerouting, forward and postrouting hooks and the tc qdisc, and never
// get an answer.
const (
	nodeAddr   = "10.200.0.1"
	clientAddr = "10.200.0.2"
)

func requireNetns(t testing.TB) string {
	t.Helper()
	if os.Getenv(nftE2E) != "1" {
		t.Skipf("set %s=1 (as root) to run the real-kernel nftables tests", nftE2E)
	}
	if os.Geteuid() != 0 {
		t.Fatalf("%s=1 needs root", nftE2E)
	}
	ip, err := exec.LookPath("ip")
	if err != nil {
		t.Fatalf("%s=1 needs iproute2: %v", nftE2E, err)
	}
	for _, bin := range []string{"nft", "tc"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Fatalf("%s=1 needs %s: %v", nftE2E, bin, err)
		}
	}
	return ip
}

// newNetns creates a namespace with lo up and the two dummy interfaces,
// and deletes it when the test ends.
func newNetns(t testing.TB) *netns {
	t.Helper()
	ip := requireNetns(t)
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := &netns{name: "anixops-f2c-" + hex.EncodeToString(b), ip: ip}
	if out, err := exec.Command(ip, "netns", "add", n.name).CombinedOutput(); err != nil { // #nosec G204 -- fixed binary, generated name
		t.Fatalf("ip netns add: %v: %s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command(ip, "netns", "del", n.name).CombinedOutput(); err != nil { // #nosec G204 -- as above
			t.Errorf("ip netns del %s: %v: %s", n.name, err, out)
		}
	})
	n.must(t, "ip", "link", "set", "lo", "up")
	for _, dev := range []string{limitIface, foreignIface} {
		n.must(t, "ip", "link", "add", dev, "type", "dummy")
		n.must(t, "ip", "link", "set", dev, "up")
	}
	return n
}

// exec runs a command inside the namespace.
func (n *netns) exec(ctx context.Context, name string, args []string, stdin []byte) ([]byte, []byte, error) {
	argv := append([]string{"netns", "exec", n.name, name}, args...)
	cmd := exec.CommandContext(ctx, n.ip, argv...) // #nosec G204 -- test harness, fixed binaries
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func (n *netns) must(t testing.TB, name string, args ...string) []byte {
	t.Helper()
	out, errOut, err := n.exec(context.Background(), name, args, nil)
	if err != nil {
		t.Fatalf("%s %s: %v: %s", name, strings.Join(args, " "), err, errOut)
	}
	return out
}

func (n *netns) nft(t testing.TB, script string) {
	t.Helper()
	if _, errOut, err := n.exec(context.Background(), "nft", []string{"-f", "-"}, []byte(script)); err != nil {
		t.Fatalf("nft -f: %v: %s\n%s", err, errOut, script)
	}
}

// nsRunner runs the driver's commands in the namespace. It counts the
// full applies that succeeded and can make the next one fail inside the
// kernel.
type nsRunner struct {
	ns *netns

	mu       sync.Mutex
	applies  int
	failNext bool
	log      []string
	// textTC drops -j from tc class and filter listings, as iproute2
	// before 6.3 effectively does for classes.
	textTC bool
}

// failLine makes the kernel refuse the whole transaction.
const failLine = "delete chain inet anixops_fwd anixops_no_such_chain\n"

func (r *nsRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	full := name == "nft" && slices.Equal(args, []string{"-f", "-"}) && bytes.Contains(stdin, []byte("# Rendered by sdk/forward/driver/nftables"))
	r.mu.Lock()
	if r.textTC && name == "tc" && len(args) > 1 && args[0] == "-j" && (args[1] == "class" || args[1] == "filter") {
		args = args[1:]
	}
	if full && r.failNext {
		r.failNext = false
		stdin = append(slices.Clone(stdin), failLine...)
	}
	r.log = append(r.log, name+" "+strings.Join(args, " "))
	r.mu.Unlock()
	out, errOut, err := r.ns.exec(ctx, name, args, stdin)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		// ip netns exec answers 255 when the command is missing.
		if strings.Contains(string(errOut), "No such file or directory") && strings.Contains(string(errOut), "exec of") {
			err = fmt.Errorf("%w: %s", exec.ErrNotFound, errOut)
		}
		return out, &CommandError{Name: name, Args: args, Stderr: string(errOut), Err: err}
	}
	if full {
		r.mu.Lock()
		r.applies++
		r.mu.Unlock()
	}
	return out, nil
}

// e2eConfig is the configuration the harness probes: every feature, with
// the namespace's dummy interface for bandwidth limits.
func e2eConfig() Config {
	c := DefaultConfig()
	c.LimitInterfaces = []string{limitIface}
	c.MSSClampInterfaces = []string{limitIface}
	return c
}

// nsEnv is the conformance Env of one namespace.
type nsEnv struct {
	ns     *netns
	client *netns // created on the first Traffic
	run    *nsRunner
	cfg    Config
}

var (
	_ conformance.Env             = (*nsEnv)(nil)
	_ conformance.Damager         = (*nsEnv)(nil)
	_ conformance.ApplyFaulter    = (*nsEnv)(nil)
	_ conformance.ConflictPlanter = (*nsEnv)(nil)
	_ conformance.ImpostorPlanter = (*nsEnv)(nil)
	_ conformance.ApplyCounter    = (*nsEnv)(nil)
	_ conformance.TrafficSource   = (*nsEnv)(nil)
)

func newNsEnv(t testing.TB) *nsEnv {
	t.Helper()
	ns := newNetns(t)
	r := &nsRunner{ns: ns}
	cfg, rep, err := Probe(context.Background(), r, e2eConfig())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version == "" {
		t.Fatalf("probe: unavailable: %s (missing %v)", cfg.Unavailable, rep.Missing)
	}
	if len(rep.Missing) > 0 {
		t.Logf("probe: missing %v", rep.Missing)
	}
	return &nsEnv{ns: ns, run: r, cfg: cfg}
}

func (e *nsEnv) driver(t testing.TB) *Driver {
	t.Helper()
	d, err := New(e.cfg, WithRunner(e.run))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func (e *nsEnv) NewDriver(t testing.TB) driver.Driver { return e.driver(t) }

// Owned lists the owned table (normalized, epochs left out) and the
// driver's tc objects.
func (e *nsEnv) Owned(t testing.TB) []string {
	t.Helper()
	d := e.driver(t)
	h, err := d.readHost(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	if h.owned {
		out = h.table.normalized(normalizeOptions{noComments: true})
	}
	tcs, err := d.tcListing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return append(out, tcs...)
}

// PlantForeign creates a table with the driver's chain and counter names
// in another table, a table named anixops_fwd in another family, and a
// foreign HTB qdisc on another interface.
func (e *nsEnv) PlantForeign(t testing.TB) {
	t.Helper()
	hop := hopName(driver.HopKey{RouteID: conformance.RouteA})
	e.ns.nft(t, fmt.Sprintf(`table inet foreign_t {
	counter %[1]s_up {
	}
	chain %[1]s_dnat {
		tcp dport 9 accept
	}
	chain forward {
		type filter hook forward priority 10; policy accept;
		ct mark 0x5 accept
	}
}
table ip anixops_fwd {
	chain prerouting {
		type nat hook prerouting priority dstnat; policy accept;
	}
}
`, hop))
	e.ns.must(t, "tc", "qdisc", "add", "dev", foreignIface, "root", "handle", "1:", "htb")
}

// Foreign lists every table but the driver's own (an impostor included)
// and every qdisc but the driver's.
func (e *nsEnv) Foreign(t testing.TB) []string {
	t.Helper()
	l, err := parseListing(e.ns.must(t, "nft", "-j", "list", "ruleset"))
	if err != nil {
		t.Fatal(err)
	}
	own := l.in(Family, Table)
	if tb, ok := own.table(Family, Table); ok && tb.str("comment") == OwnerComment {
		rest := &listing{}
		for _, it := range l.items {
			if it.family() != Family || it.tableName() != Table {
				rest.items = append(rest.items, it)
			}
		}
		l = rest
	}
	out := l.normalized(normalizeOptions{})
	for _, dev := range []string{limitIface, foreignIface} {
		var qdiscs []map[string]any
		if err := decodeJSON(e.ns.must(t, "tc", "-j", "qdisc", "show", "dev", dev), &qdiscs); err != nil {
			t.Fatal(err)
		}
		for _, q := range qdiscs {
			if h := q["handle"]; h != "af00:" && h != "0:" {
				out = append(out, fmt.Sprintf("tc %s qdisc %v %v root=%v", dev, q["kind"], h, q["root"]))
			}
		}
	}
	return out
}

// Damage flushes the forward chain, as an operator or a half-done apply
// might.
func (e *nsEnv) Damage(t testing.TB) {
	t.Helper()
	e.ns.must(t, "nft", "flush", "chain", Family, Table, "forward")
}

func (e *nsEnv) FailNextApply(testing.TB) {
	e.run.mu.Lock()
	e.run.failNext = true
	e.run.mu.Unlock()
}

func (e *nsEnv) Applies(testing.TB) int {
	e.run.mu.Lock()
	defer e.run.mu.Unlock()
	return e.run.applies
}

// PlantConflict makes another table DNAT the port.
func (e *nsEnv) PlantConflict(t testing.TB, port uint32) {
	t.Helper()
	e.ns.nft(t, fmt.Sprintf(`table ip foreign_conflict {
	chain pre {
		type nat hook prerouting priority dstnat; policy accept;
		tcp dport %d dnat to 192.0.2.99:80
	}
}
`, port))
}

// PlantImpostor creates "table inet anixops_fwd" without the driver's
// comment.
func (e *nsEnv) PlantImpostor(t testing.TB) {
	t.Helper()
	e.ns.nft(t, `table inet anixops_fwd {
	comment "someone else's table"
	chain keep {
		tcp dport 7 accept
	}
}
`)
}

// Traffic opens TCP connections from the client namespace to the hop's
// listener: their SYNs are DNATed and forwarded out of the dummy
// interface, so the hop's up counter grows (nothing answers, so down
// stays). Only TCP listeners are reached.
func (e *nsEnv) Traffic(t testing.TB, hop driver.HopKey) {
	t.Helper()
	if e.client == nil {
		e.client = e.connectClient(t)
	}
	h, err := e.driver(t).readHost(context.Background())
	if err != nil || h.doc == nil || h.doc.hop(hop) == nil {
		t.Fatalf("Traffic: hop %s not applied (%v)", hop, err)
	}
	protos, port, _ := strings.Cut(h.doc.hop(hop).Listen, "/")
	if !strings.Contains(protos, "tcp") {
		t.Fatalf("Traffic: hop %s does not listen on TCP", hop)
	}
	for range 3 {
		// A SYN that is never answered: bash gives up after the timeout.
		script := fmt.Sprintf("exec 3<>/dev/tcp/%s/%s", nodeAddr, port)
		_, _, _ = e.client.exec(context.Background(), "timeout", []string{"0.3", "bash", "-c", script}, nil)
	}
}

// connectClient creates the client namespace and the veth pair, enables
// forwarding in the driver's namespace and routes the upstreams out of
// the dummy interface.
func (e *nsEnv) connectClient(t testing.TB) *netns {
	t.Helper()
	c := &netns{name: e.ns.name + "-c", ip: e.ns.ip}
	if out, err := exec.Command(c.ip, "netns", "add", c.name).CombinedOutput(); err != nil { // #nosec G204 -- fixed binary, generated name
		t.Fatalf("ip netns add: %v: %s", err, out)
	}
	t.Cleanup(func() { _ = exec.Command(c.ip, "netns", "del", c.name).Run() }) // #nosec G204 -- as above
	c.must(t, "ip", "link", "set", "lo", "up")
	e.ns.must(t, "ip", "link", "add", "veth0", "type", "veth", "peer", "name", "veth1", "netns", c.name)
	e.ns.must(t, "ip", "addr", "add", nodeAddr+"/24", "dev", "veth0")
	e.ns.must(t, "ip", "link", "set", "veth0", "up")
	c.must(t, "ip", "addr", "add", clientAddr+"/24", "dev", "veth1")
	c.must(t, "ip", "link", "set", "veth1", "up")
	e.ns.must(t, "ip", "addr", "add", "192.0.2.1/24", "dev", limitIface)
	e.ns.must(t, "sysctl", "-qw", "net.ipv4.ip_forward=1")
	return c
}

// TestNetnsConformance runs the whole conformance suite against the real
// driver on a real kernel, one namespace per scenario.
func TestNetnsConformance(t *testing.T) {
	requireNetns(t)
	conformance.Run(t, func(t *testing.T) conformance.Env { return newNsEnv(t) }, conformance.WithTimeout(60*time.Second))
}
