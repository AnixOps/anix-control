package gost_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver"
	"github.com/AnixOps/anix-control/sdk/forward/driver/conformance"
	"github.com/AnixOps/anix-control/sdk/forward/driver/gost"
)

// The real-gost harness: every test gets a fresh network namespace (`ip
// netns add`) and runs the pinned gost inside it through a
// ProcessSupervisor ("ip netns exec <ns> gost -C ..."), so nothing touches
// the host's own network or its systemd units. It needs root
// (CAP_NET_ADMIN and CAP_SYS_ADMIN for the namespace) and runs only with
// ANIXOPS_GOST_E2E=1, which also makes a missing prerequisite fail instead
// of skip. The gost binary is ANIXOPS_GOST_BIN, or gost from PATH. CI runs
// it under sudo in Backend Tests shard 1, with the release ci.yml
// downloads and checks (GOST_VERSION).
const gostE2E = "ANIXOPS_GOST_E2E"

// gostBinary answers the gost binary to test with, or "" when there is
// none.
func gostBinary() string {
	if p := os.Getenv("ANIXOPS_GOST_BIN"); p != "" {
		return p
	}
	p, _ := exec.LookPath("gost")
	return p
}

// requireNetns skips unless ANIXOPS_GOST_E2E=1 and answers the ip and gost
// binaries.
func requireNetns(t testing.TB) (ip, bin string) {
	t.Helper()
	if os.Getenv(gostE2E) != "1" {
		t.Skipf("set %s=1 (as root) to run gost in network namespaces", gostE2E)
	}
	if os.Geteuid() != 0 {
		t.Fatalf("%s=1 needs root", gostE2E)
	}
	ip, err := exec.LookPath("ip")
	if err != nil {
		t.Fatalf("%s=1 needs iproute2: %v", gostE2E, err)
	}
	if _, err := exec.LookPath("ss"); err != nil {
		t.Fatalf("%s=1 needs ss: %v", gostE2E, err)
	}
	if bin = gostBinary(); bin == "" {
		t.Fatalf("%s=1 needs gost: set ANIXOPS_GOST_BIN or put gost on PATH", gostE2E)
	}
	return ip, bin
}

// netns is one throwaway network namespace.
type netns struct {
	name, ip string
}

func newNetns(t testing.TB) *netns {
	t.Helper()
	ip, _ := requireNetns(t)
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	n := &netns{name: "anixops-f4a-" + hex.EncodeToString(b), ip: ip}
	if out, err := exec.Command(ip, "netns", "add", n.name).CombinedOutput(); err != nil { // #nosec G204 -- fixed binary, generated name
		t.Fatalf("ip netns add: %v: %s", err, out)
	}
	t.Cleanup(func() {
		if out, err := exec.Command(ip, "netns", "del", n.name).CombinedOutput(); err != nil { // #nosec G204 -- as above
			t.Errorf("ip netns del %s: %v: %s", n.name, err, out)
		}
	})
	n.must(t, "ip", "link", "set", "lo", "up")
	n.must(t, "ip", "link", "add", "dummy0", "type", "dummy")
	n.must(t, "ip", "link", "set", "dummy0", "up")
	return n
}

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

// addAddress puts addr on the namespace's dummy interface, so gost can
// bind it.
func (n *netns) addAddress(t testing.TB, addr string) {
	t.Helper()
	bits := "/32"
	if strings.Contains(addr, ":") {
		bits = "/128"
	}
	args := []string{"addr", "add", addr + bits, "dev", "dummy0"}
	if bits == "/128" {
		args = append(args, "nodad")
	}
	n.must(t, "ip", args...)
}

// spawn starts a long-running command in the namespace; the caller kills
// it.
func (n *netns) spawn(t testing.TB, log *syncBuffer, name string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(n.ip, append([]string{"netns", "exec", n.name, name}, args...)...) // #nosec G204 -- test harness
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func kill(cmd *exec.Cmd) {
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
}

// nsRunner runs the driver's ss (and Probe's gost) in the namespace.
type nsRunner struct {
	ns  *netns
	bin string
}

func (r nsRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	bin := name
	switch name {
	case "ss":
	case "gost":
		bin = r.bin
	default:
		return nil, fmt.Errorf("nsRunner: refusing %s", name)
	}
	out, errOut, err := r.ns.exec(ctx, bin, args, stdin)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		if strings.Contains(string(errOut), "No such file or directory") && strings.Contains(string(errOut), "exec of") {
			err = fmt.Errorf("%w: %s", exec.ErrNotFound, errOut)
		}
		return out, &gost.CommandError{Name: name, Args: args, Stderr: string(errOut), Err: err}
	}
	return out, nil
}

// syncBuffer collects gost's output for the test log.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

// linkPKI writes a CA (ca.crt, and its key ca.key for renewLink) and,
// per node name, a link certificate with the name as DNS SAN and SPIFFE
// URI and both server and client auth, as the gost driver needs them
// (doc.go, "Link certificates").
func linkPKI(t testing.TB, dir string, nodes ...string) map[string][3]string {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test link CA"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(dir, "ca.crt")
	writePEM(t, caPath, "CERTIFICATE", caDER)
	caKDER, err := x509.MarshalECPrivateKey(caKey)
	if err != nil {
		t.Fatal(err)
	}
	writePEM(t, filepath.Join(dir, "ca.key"), "EC PRIVATE KEY", caKDER)
	out := map[string][3]string{}
	for i, n := range nodes {
		crt, kf := filepath.Join(dir, n+".crt"), filepath.Join(dir, n+".key")
		out[n] = [3]string{crt, kf, caPath}
		issueLink(t, out[n], n, int64(i)+2)
	}
	return out
}

// issueLink writes a link certificate for name with serial, signed by the
// CA next to files[2] (linkPKI), to files[0] and its key to files[1], each
// replaced by a rename, as an Agent renewing them would.
func issueLink(t testing.TB, files [3]string, name string, serial int64) {
	t.Helper()
	read := func(p string) []byte {
		b, err := os.ReadFile(p) // #nosec G304 -- the test's own PKI
		if err != nil {
			t.Fatal(err)
		}
		blk, _ := pem.Decode(b)
		if blk == nil {
			t.Fatalf("%s: no PEM", p)
		}
		return blk.Bytes
	}
	ca, err := x509.ParseCertificate(read(files[2]))
	if err != nil {
		t.Fatal(err)
	}
	caKey, err := x509.ParseECPrivateKey(read(filepath.Join(filepath.Dir(files[2]), "ca.key")))
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	u, _ := url.Parse("spiffe://anixops/example/agent/" + name)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		DNSNames: []string{name}, URIs: []*url.URL{u},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	kder, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct {
		path, typ string
		der       []byte
	}{{files[1], "EC PRIVATE KEY", kder}, {files[0], "CERTIFICATE", der}} {
		writePEM(t, f.path+".new", f.typ, f.der)
		if err := os.Rename(f.path+".new", f.path); err != nil {
			t.Fatal(err)
		}
	}
}

func writePEM(t testing.TB, path, typ string, der []byte) {
	t.Helper()
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: typ, Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
}

// shortDir answers a fresh directory with a short path (a unix socket path
// holds at most 107 bytes), removed when the test ends.
func shortDir(t testing.TB) string {
	t.Helper()
	d, err := os.MkdirTemp("", "agost")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// node is one gost node in a namespace: its directories, supervisor and
// probed configuration.
type node struct {
	ns   *netns
	bin  string
	run  nsRunner
	sup  *countingSupervisor
	cfg  gost.Config
	log  *syncBuffer
	name string
}

// newNode makes a node of the namespace named name (its link certificate's
// name), with its own directories, so several nodes share a namespace.
func newNode(t testing.TB, ns *netns, name string, pki map[string][3]string) *node {
	t.Helper()
	_, bin := requireNetns(t)
	root := shortDir(t)
	n := &node{ns: ns, bin: bin, run: nsRunner{ns: ns, bin: bin}, log: &syncBuffer{}, name: name}
	cfg := gost.DefaultConfig()
	cfg.Dir, cfg.RuntimeDir = filepath.Join(root, "d"), filepath.Join(root, "r")
	cfg.LinkCert, cfg.LinkKey, cfg.LinkCA = pki[name][0], pki[name][1], pki[name][2]
	cfg.ReadyTimeout = 5 * time.Second
	for _, d := range []string{cfg.Dir, cfg.RuntimeDir} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	n.sup = &countingSupervisor{ProcessSupervisor: &gost.ProcessSupervisor{
		Command: []string{ns.ip, "netns", "exec", ns.name, bin},
		Config:  filepath.Join(cfg.Dir, gost.ConfigFile),
		Log:     n.log,
	}, config: filepath.Join(cfg.Dir, gost.ConfigFile)}
	probed, rep, err := gost.Probe(context.Background(), n.run, n.sup, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if probed.Version == "" {
		t.Fatalf("probe: unavailable: %s (missing %v)", probed.Unavailable, rep.Missing)
	}
	for _, w := range rep.Warnings {
		t.Log("probe:", w)
	}
	n.cfg = probed
	t.Cleanup(func() {
		_ = n.sup.Stop(context.Background())
		if t.Failed() {
			t.Logf("gost %s output:\n%s", name, n.log.String())
		}
	})
	return n
}

func (n *node) driver(t testing.TB) *gost.Driver {
	t.Helper()
	d, err := gost.New(n.cfg, gost.WithRunner(n.run), gost.WithSupervisor(n.sup))
	if err != nil {
		t.Fatal(err)
	}
	// FailNextApply also fails an apply that changes gost through its
	// web API: gost refuses the first service it is asked to create.
	gost.SetAPIFault(d, func(method, path string) error {
		if method != http.MethodPost || path != "/config/services" {
			return nil
		}
		n.sup.mu.Lock()
		defer n.sup.mu.Unlock()
		if n.sup.failNext {
			n.sup.failNext = false
			return errors.New("injected failure: service refused")
		}
		return nil
	})
	return d
}

// countingSupervisor counts the starts and reloads (full applies) and can
// make the next one fail inside gost: it overwrites the configuration
// with one gost cannot parse just before, so gost refuses it as it would
// a bad configuration. An apply through the web API fails instead at the
// first service it creates (node.driver).
type countingSupervisor struct {
	*gost.ProcessSupervisor
	config string

	mu       sync.Mutex
	applies  int
	failNext bool
}

func (c *countingSupervisor) fault() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applies++
	if c.failNext {
		c.failNext = false
		_ = os.WriteFile(c.config, []byte("{ not a gost configuration"), 0o600) // #nosec G306 -- test fault
	}
}

func (c *countingSupervisor) Start(ctx context.Context) error {
	c.fault()
	return c.ProcessSupervisor.Start(ctx)
}

func (c *countingSupervisor) Reload(ctx context.Context) error {
	c.fault()
	return c.ProcessSupervisor.Reload(ctx)
}

// pid answers the running gost's pid, 0 when it is stopped.
func (c *countingSupervisor) pid(t testing.TB) string {
	t.Helper()
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Running {
		return ""
	}
	_, pid, _ := strings.Cut(st.Instance, "-")
	return pid
}

// sockets answers the namespace's listening sockets as "tcp *:30001",
// split into the ones the node's gost holds and the others.
func (n *node) sockets(t testing.TB) (mine, others []string) {
	t.Helper()
	pid := n.sup.pid(t)
	out := n.ns.must(t, "ss", "-H", "-l", "-n", "-t", "-u", "-p")
	for line := range strings.Lines(string(out)) {
		f := strings.Fields(line)
		if len(f) < 5 {
			continue
		}
		s := f[0] + " " + f[4]
		if pid != "" && strings.Contains(line, "pid="+pid+",") {
			mine = append(mine, s)
		} else {
			others = append(others, s)
		}
	}
	slices.Sort(mine)
	slices.Sort(others)
	return mine, others
}

// nsEnv is the conformance Env of one namespace with one gost node.
type nsEnv struct {
	*node
	foreign *syncBuffer

	mu      sync.Mutex
	planted []*exec.Cmd // foreign processes, killed after the suite checked them
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
	top := conformance.DefaultTopology()
	pki := linkPKI(t, shortDir(t), top.NodeRef)
	// The topology's IPv4 upstreams answer: an echo server on each, so
	// Traffic moves bytes through the hops.
	for _, a := range top.UpstreamsV4 {
		ns.addAddress(t, a)
		startEcho(t, ns, fmt.Sprintf("%s:%d", a, top.UpstreamPort))
	}
	e := &nsEnv{node: newNode(t, ns, top.NodeRef, pki), foreign: &syncBuffer{}}
	// Registered before the suite's own cleanup, so it runs after the
	// suite checked the foreign objects.
	t.Cleanup(func() {
		e.mu.Lock()
		defer e.mu.Unlock()
		for _, c := range e.planted {
			kill(c)
		}
	})
	return e
}

func (e *nsEnv) NewDriver(t testing.TB) driver.Driver { return e.driver(t) }

func fileDigest(path string) string {
	b, err := os.ReadFile(path) // #nosec G304 -- a file of the test's own directory
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

// owned answers whether the driver's state file is there and carries the
// driver's mark.
func (e *nsEnv) owned() bool {
	b, err := os.ReadFile(filepath.Join(e.cfg.Dir, gost.StateFile))
	return err == nil && bytes.Contains(b, []byte(gost.OwnerMark))
}

// Owned lists the configuration file, the running gost and the sockets it
// holds, when the driver owns them.
func (e *nsEnv) Owned(t testing.TB) []string {
	t.Helper()
	var out []string
	if e.owned() {
		if d := fileDigest(filepath.Join(e.cfg.Dir, gost.ConfigFile)); d != "" {
			out = append(out, "config "+d)
		}
	}
	if e.sup.pid(t) != "" {
		out = append(out, "gost running")
	}
	mine, _ := e.sockets(t)
	return append(out, mine...)
}

// PlantForeign starts a foreign gost on another port of the namespace.
func (e *nsEnv) PlantForeign(t testing.TB) {
	t.Helper()
	e.plantListener(t, 39999)
}

func (e *nsEnv) plantListener(t testing.TB, port uint32) {
	t.Helper()
	cmd := e.ns.spawn(t, e.foreign, e.bin, "-L", fmt.Sprintf("tcp://:%d/192.0.2.99:80", port))
	e.mu.Lock()
	e.planted = append(e.planted, cmd)
	e.mu.Unlock()
	want := fmt.Sprintf(":%d", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, others := e.sockets(t); slices.ContainsFunc(others, func(s string) bool { return strings.HasSuffix(s, want) }) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("foreign gost did not listen on %d: %s", port, e.foreign.String())
}

// Foreign lists the sockets the node's gost does not hold and a
// configuration in the driver's directory without its state file.
func (e *nsEnv) Foreign(t testing.TB) []string {
	t.Helper()
	_, out := e.sockets(t)
	if !e.owned() {
		if d := fileDigest(filepath.Join(e.cfg.Dir, gost.ConfigFile)); d != "" {
			out = append(out, "impostor config "+d)
		}
	}
	return out
}

// Damage kills gost, as a crash would.
func (e *nsEnv) Damage(testing.TB) { e.sup.Kill() }

// Traffic sends a line through the hop's TCP listener to the echo server
// behind it and reads the answer.
func (e *nsEnv) Traffic(t testing.TB, hop driver.HopKey) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(e.cfg.Dir, gost.StateFile))
	if err != nil {
		t.Fatalf("Traffic: %v", err)
	}
	var st struct {
		Hops []struct {
			Route     string
			Hop       uint32
			Listeners []struct {
				Network string
				Port    uint32
			}
		}
	}
	if err := json.Unmarshal(b, &st); err != nil {
		t.Fatal(err)
	}
	for _, h := range st.Hops {
		if h.Route != hop.RouteID || h.Hop != hop.HopIndex {
			continue
		}
		for _, l := range h.Listeners {
			if l.Network != "tcp" {
				continue
			}
			c, err := e.ns.dial("tcp", fmt.Sprintf("127.0.0.1:%d", l.Port))
			if err != nil {
				t.Fatalf("Traffic: %v", err)
			}
			got, err := exchange(c, strings.Repeat("t", 200))
			_ = c.Close()
			if err != nil || !strings.HasPrefix(got, "echo:") {
				t.Fatalf("Traffic through %s: %q %v", hop, got, err)
			}
			return
		}
	}
	t.Fatalf("Traffic: hop %s has no TCP listener", hop)
}

func (e *nsEnv) FailNextApply(testing.TB) {
	e.sup.mu.Lock()
	e.sup.failNext = true
	e.sup.mu.Unlock()
}

func (e *nsEnv) Applies(testing.TB) int {
	e.sup.mu.Lock()
	defer e.sup.mu.Unlock()
	return e.sup.applies
}

// PlantConflict starts a foreign gost on the port.
func (e *nsEnv) PlantConflict(t testing.TB, port uint32) { e.plantListener(t, port) }

// PlantImpostor writes a gost configuration into the driver's directory
// without the driver's state file.
func (e *nsEnv) PlantImpostor(t testing.TB) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(e.cfg.Dir, gost.ConfigFile), []byte(`{"services": []}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestNetnsConformance runs the whole conformance suite against the real
// driver and the real gost, one namespace per scenario, with traffic
// through the hops (TrafficSource).
func TestNetnsConformance(t *testing.T) {
	requireNetns(t)
	conformance.Run(t, func(t *testing.T) conformance.Env { return newNsEnv(t) }, conformance.WithTimeout(60*time.Second))
}

// TestNetnsGoldens loads every golden case's configuration in a real gost:
// each case renders with link certificates of a test CA, in a namespace
// whose dummy interface holds the hops' listen addresses, and must apply
// (gost serves the configuration and holds every listener) and remove.
func TestNetnsGoldens(t *testing.T) {
	requireNetns(t)
	for _, c := range goldenCases(t) {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ns := newNetns(t)
			ref := c.state.GetNodeRef()
			n := newNode(t, ns, ref, linkPKI(t, shortDir(t), ref))
			if c.cfg != nil {
				// Keep the case's switches, with the node's paths.
				cfg := n.cfg
				c.cfg(&cfg)
				if cfg.LinkCert == "" {
					n.cfg.LinkCert, n.cfg.LinkKey, n.cfg.LinkCA = "", "", ""
				}
				n.cfg.Strategies = cfg.Strategies
			}
			seen := map[string]bool{}
			for _, h := range c.state.GetHops() {
				if a := h.GetListen().GetAddress(); a != "" && h.GetEngine() == gostE && !seen[a] {
					seen[a] = true
					ns.addAddress(t, a)
				}
			}
			d := n.driver(t)
			a, err := d.Render(c.state)
			if err != nil {
				t.Logf("render: %v", err)
			}
			r, err := d.Apply(t.Context(), a)
			if err != nil {
				t.Fatalf("Apply: %v", err)
			}
			if r.Changed == a.Empty() {
				t.Fatalf("Apply changed %v for an artifact of %d hops", r.Changed, len(a.Hops))
			}
			if !a.Empty() {
				if mine, _ := n.sockets(t); len(mine) == 0 {
					t.Fatal("gost holds no socket")
				}
			}
			if err := d.Remove(t.Context()); err != nil {
				t.Fatal(err)
			}
			if n.sup.pid(t) != "" {
				t.Fatal("gost still runs after Remove")
			}
		})
	}
}
