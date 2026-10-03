//go:build linux

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/sdk/forward/driver/nftables"
	"golang.org/x/sys/unix"
)

// Namespaces are named afe2e-<pid>-<lab>-<role>: the prefix marks them as
// this suite's, and the pid of the test process that made them lets a later
// run remove the leftovers of a run that died (removeStaleNamespaces)
// without touching the namespaces of a run that is still going.
const nsPrefix = "afe2e-"

// netnsDir is where `ip netns add` mounts named namespaces.
const netnsDir = "/run/netns"

// netns is one throwaway network namespace.
type netns struct {
	name string
	role string // short: cli, ent, rel, t1; interfaces are named after it
}

func nsName(lab int64, role string) string {
	return fmt.Sprintf("%s%d-%d-%s", nsPrefix, os.Getpid(), lab, role)
}

func ipCmd(args ...string) ([]byte, error) {
	out, err := exec.Command("ip", args...).CombinedOutput() // #nosec G204 -- test harness: iproute2 with generated namespace names
	if err != nil {
		return out, fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, bytes.TrimSpace(out))
	}
	return out, nil
}

// addNetns creates a namespace with its loopback up, IPv6 duplicate address
// detection off (addresses are usable at once) and forwarding off.
func addNetns(name, role string) (*netns, error) {
	if _, err := ipCmd("netns", "add", name); err != nil {
		return nil, err
	}
	n := &netns{name: name, role: role}
	for _, c := range [][]string{
		{"ip", "link", "set", "lo", "up"},
		{"sysctl", "-qw", "net.ipv6.conf.all.accept_dad=0", "net.ipv6.conf.default.accept_dad=0"},
	} {
		if _, errOut, err := n.run(context.Background(), c[0], c[1:], nil); err != nil {
			_ = delNetns(name)
			return nil, fmt.Errorf("%s in %s: %w: %s", strings.Join(c, " "), name, err, errOut)
		}
	}
	return n, nil
}

// delNetns kills every process inside the namespace (an echo server keeps
// a namespace alive after `ip netns del` unlinks its name) and deletes it.
func delNetns(name string) error {
	if out, err := ipCmd("netns", "pids", name); err == nil {
		for _, f := range strings.Fields(string(out)) {
			if pid, err := strconv.Atoi(f); err == nil && pid > 1 && pid != os.Getpid() {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	}
	_, err := ipCmd("netns", "del", name)
	return err
}

// listNetns answers the names of every named namespace.
func listNetns() []string {
	entries, err := os.ReadDir(netnsDir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// ownerPID answers the pid in a namespace name of this suite, or 0.
func ownerPID(name string) int {
	rest, ok := strings.CutPrefix(name, nsPrefix)
	if !ok {
		return 0
	}
	pid, _, _ := strings.Cut(rest, "-")
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

func processAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// removeStaleNamespaces deletes the namespaces this suite left behind in
// runs that died before their cleanup (a killed test binary, a CI timeout):
// those with the suite's prefix whose owning process is gone. Namespaces of
// a live run, and every namespace without the prefix, are left alone.
func removeStaleNamespaces(logf func(format string, args ...any)) {
	for _, name := range listNetns() {
		pid := ownerPID(name)
		if pid == 0 || pid == os.Getpid() || processAlive(pid) {
			continue
		}
		if err := delNetns(name); err != nil {
			logf("forward e2e: removing stale namespace %s: %v", name, err)
			continue
		}
		logf("forward e2e: removed stale namespace %s", name)
	}
}

// run runs a command inside the namespace with `ip netns exec`.
func (n *netns) run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, []byte, error) {
	argv := append([]string{"netns", "exec", n.name, name}, args...)
	cmd := exec.CommandContext(ctx, "ip", argv...) // #nosec G204 -- test harness: fixed binaries inside a generated namespace
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// must runs a command inside the namespace and fails the test when it
// fails.
func (n *netns) must(t testing.TB, name string, args ...string) string {
	t.Helper()
	out, errOut, err := n.run(context.Background(), name, args, nil)
	if err != nil {
		t.Fatalf("%s: %s %s: %v: %s", n.name, name, strings.Join(args, " "), err, bytes.TrimSpace(errOut))
	}
	return string(out)
}

// do runs f on an OS thread that has joined the namespace, so the sockets
// f creates belong to it; they keep working from any goroutine afterwards.
// The thread is never unlocked, so it ends with the goroutine and never
// runs other goroutines inside the namespace.
func (n *netns) do(f func() error) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		fd, err := unix.Open(filepath.Join(netnsDir, n.name), unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			errc <- fmt.Errorf("open namespace %s: %w", n.name, err)
			return
		}
		defer func() { _ = unix.Close(fd) }()
		if err := unix.Setns(fd, unix.CLONE_NEWNET); err != nil {
			errc <- fmt.Errorf("join namespace %s: %w", n.name, err)
			return
		}
		errc <- f()
	}()
	return <-errc
}

// dialTCP connects from the namespace, from local when it is valid.
func (n *netns) dialTCP(local netip.Addr, to netip.AddrPort, timeout time.Duration) (*net.TCPConn, error) {
	var c net.Conn
	err := n.do(func() error {
		d := net.Dialer{Timeout: timeout}
		if local.IsValid() {
			d.LocalAddr = &net.TCPAddr{IP: local.AsSlice()}
		}
		var err error
		c, err = d.Dial("tcp", to.String())
		return err
	})
	if err != nil {
		return nil, err
	}
	return c.(*net.TCPConn), nil
}

// dialUDP makes a connected UDP socket in the namespace.
func (n *netns) dialUDP(to netip.AddrPort) (*net.UDPConn, error) {
	var c *net.UDPConn
	err := n.do(func() error {
		var err error
		c, err = net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(to))
		return err
	})
	return c, err
}

// interfaces answers the namespace's interfaces but loopback.
func (n *netns) interfaces() []string {
	out, _, err := n.run(context.Background(), "ip", []string{"-o", "link", "show"}, nil)
	if err != nil {
		return nil
	}
	var names []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimSuffix(f[1], ":"), "@")
		if name != "lo" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

// state answers what a failed test needs to see of the namespace.
func (n *netns) state() string {
	var b strings.Builder
	cmds := [][]string{
		{"ip", "-br", "addr"},
		{"ip", "route"},
		{"ip", "-6", "route"},
		{"nft", "list", "ruleset"},
		{"tc", "-s", "qdisc", "show"},
	}
	for _, dev := range n.interfaces() {
		cmds = append(cmds, []string{"tc", "-s", "class", "show", "dev", dev})
	}
	cmds = append(cmds, []string{"ss", "-tanu"})
	for _, c := range cmds {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		out, errOut, err := n.run(ctx, c[0], c[1:], nil)
		cancel()
		fmt.Fprintf(&b, "### %s\n%s", strings.Join(c, " "), out)
		if err != nil {
			fmt.Fprintf(&b, "(%v: %s)\n", err, bytes.TrimSpace(errOut))
		}
	}
	return b.String()
}

// nsRunner runs the nftables driver's commands (nft and tc only) inside a
// node's namespace: the Agent's ExecRunner, moved into the namespace.
type nsRunner struct{ ns *netns }

var _ nftables.Runner = nsRunner{}

func (r nsRunner) Run(ctx context.Context, name string, args []string, stdin []byte) ([]byte, error) {
	if name != "nft" && name != "tc" {
		return nil, fmt.Errorf("forward e2e runner: refusing to run %q", name)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out, errOut, err := r.ns.run(ctx, name, args, stdin)
	if err != nil {
		if cerr := ctx.Err(); cerr != nil {
			return nil, cerr
		}
		// ip netns exec fails this way when the command is missing.
		if s := string(errOut); strings.Contains(s, "exec of") && strings.Contains(s, "No such file or directory") {
			err = fmt.Errorf("%w: %s", exec.ErrNotFound, s)
		}
		return out, &nftables.CommandError{Name: name, Args: args, Stderr: string(errOut), Err: err}
	}
	return out, nil
}

// TestOwnerPID runs without privileges: the stale cleanup reads the owning
// pid only from names with the suite's prefix.
func TestOwnerPID(t *testing.T) {
	for name, want := range map[string]int{
		nsName(3, "ent"):        os.Getpid(),
		"afe2e-42-1-t1":         42,
		"afe2e-0-1-t1":          0,
		"afe2e-x-1-t1":          0,
		"afe2e-":                0,
		"anixops-f2c-0a1b2c3d":  0,
		"f2d-other-42":          0,
		"cni-1234-5678-abcdef0": 0,
	} {
		if got := ownerPID(name); got != want {
			t.Errorf("ownerPID(%q) = %d, want %d", name, got, want)
		}
	}
}
