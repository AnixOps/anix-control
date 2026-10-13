// Package builtin is the table of the official package hosts linked into
// the Control binary. The kernel starts a host by executing its own binary
// again as `anix-control package-host <id>` (cmd/server) with the package
// host environment of internal/pluginhost; Main is that command.
//
// The table names every official package of packages/shared/build_package.py
// (PACKAGE_SPECS): the packages with a host of their own run it, nat-egress
// and nftables-forward run the generic host (packages/shared/controlhost).
// TestHostsMatchThePackageMatrix keeps the two lists equal.
package builtin

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"

	affiliate "github.com/AnixOps/anix-control/v5/packages/affiliate/control"
	forward "github.com/AnixOps/anix-control/v5/packages/forward/control"
	gostmesh "github.com/AnixOps/anix-control/v5/packages/gost-mesh/control"
	identityplatform "github.com/AnixOps/anix-control/v5/packages/identity-platform/control"
	knowledge "github.com/AnixOps/anix-control/v5/packages/knowledge/control"
	machinetelemetry "github.com/AnixOps/anix-control/v5/packages/machine-telemetry/control"
	notification "github.com/AnixOps/anix-control/v5/packages/notification/control"
	order "github.com/AnixOps/anix-control/v5/packages/order/control"
	payment "github.com/AnixOps/anix-control/v5/packages/payment/control"
	plan "github.com/AnixOps/anix-control/v5/packages/plan/control"
	platform "github.com/AnixOps/anix-control/v5/packages/platform/control"
	protocolruntime "github.com/AnixOps/anix-control/v5/packages/protocol-runtime/control"
	proxynode "github.com/AnixOps/anix-control/v5/packages/proxy-node/control"
	"github.com/AnixOps/anix-control/v5/packages/shared/controlhost"
	subscription "github.com/AnixOps/anix-control/v5/packages/subscription/control"
	ticket "github.com/AnixOps/anix-control/v5/packages/ticket/control"
	wireguard "github.com/AnixOps/anix-control/v5/packages/wireguard/control"
)

// CommandName is the Control subcommand that runs a package host.
const CommandName = "package-host"

// VersionEnvironment carries the package release the kernel verified and
// starts the host for. The host serves exactly that version: it answers only
// requests for it (pluginhostsdk.Server).
const VersionEnvironment = "ANIX_CONTROL_HOST_PACKAGE_VERSION"

// Host is one official package's Control host.
type Host struct {
	// ID is the package id.
	ID string
	// Generic reports the generic host (no native routes).
	Generic bool
	// Run serves the host for one release of the package.
	Run func(ctx context.Context, version string) error
}

func generic(id string) Host {
	return Host{ID: id, Generic: true, Run: func(ctx context.Context, version string) error {
		return controlhost.Run(ctx, id, version)
	}}
}

var hosts = []Host{
	{ID: affiliate.PackageID, Run: affiliate.Run},
	{ID: forward.PackageID, Run: forward.Run},
	{ID: gostmesh.PackageID, Run: gostmesh.Run},
	{ID: identityplatform.PackageID, Run: identityplatform.Run},
	{ID: knowledge.PackageID, Run: knowledge.Run},
	{ID: machinetelemetry.PackageID, Run: machinetelemetry.Run},
	generic("nat-egress"),
	generic("nftables-forward"),
	{ID: notification.PackageID, Run: notification.Run},
	{ID: order.PackageID, Run: order.Run},
	{ID: payment.PackageID, Run: payment.Run},
	{ID: plan.PackageID, Run: plan.Run},
	{ID: platform.PackageID, Run: platform.Run},
	{ID: protocolruntime.PackageID, Run: protocolruntime.Run},
	{ID: proxynode.PackageID, Run: proxynode.Run},
	{ID: subscription.PackageID, Run: subscription.Run},
	{ID: ticket.PackageID, Run: ticket.Run},
	{ID: wireguard.PackageID, Run: wireguard.Run},
}

// Hosts returns every linked host, sorted by package id.
func Hosts() []Host {
	out := append([]Host(nil), hosts...)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// IDs returns the package ids of every linked host, sorted.
func IDs() []string {
	ids := make([]string, 0, len(hosts))
	for _, host := range Hosts() {
		ids = append(ids, host.ID)
	}
	return ids
}

// Lookup returns the host of a package id.
func Lookup(id string) (Host, bool) {
	for _, host := range hosts {
		if host.ID == id {
			return host, true
		}
	}
	return Host{}, false
}

// ErrUnknownHost is returned for a package id no linked host serves.
var ErrUnknownHost = errors.New("no built-in host serves this package")

// Run serves the host of one package release.
func Run(ctx context.Context, id, version string) error {
	host, ok := Lookup(strings.TrimSpace(id))
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownHost, id)
	}
	if strings.TrimSpace(version) == "" {
		return fmt.Errorf("%s: the package version is required", id)
	}
	return host.Run(ctx, version)
}

// Options configures Main.
type Options struct {
	// Getenv reads the host environment (os.Getenv).
	Getenv func(string) string
	// Version resolves the release to serve when the kernel did not set
	// VersionEnvironment (a network module started by an operator) and
	// checks the one it did set. Nil accepts the environment's version and
	// fails without one.
	Version func(id, requested string) (string, error)
	// Stderr receives usage and errors.
	Stderr io.Writer
}

// Main runs `package-host <id>` and returns the process exit code: 0 after
// a clean stop, 1 when the host fails, 2 for a usage error.
func Main(ctx context.Context, args []string, options Options) int {
	logger := log.Default()
	if options.Stderr != nil {
		logger = log.New(options.Stderr, "", log.LstdFlags)
	}
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		logger.Printf("usage: anix-control %s <package-id>; package ids: %s", CommandName, strings.Join(IDs(), ", "))
		return 2
	}
	id := args[0]
	if _, ok := Lookup(id); !ok {
		logger.Printf("%s: %v: %q (package ids: %s)", CommandName, ErrUnknownHost, id, strings.Join(IDs(), ", "))
		return 2
	}
	requested := ""
	if options.Getenv != nil {
		requested = strings.TrimSpace(options.Getenv(VersionEnvironment))
	}
	version := requested
	if options.Version != nil {
		resolved, err := options.Version(id, requested)
		if err != nil {
			logger.Printf("%s control host: %v", id, err)
			return 1
		}
		version = resolved
	}
	if version == "" {
		logger.Printf("%s control host: %s is not set and this binary embeds no %s package", id, VersionEnvironment, id)
		return 1
	}
	if err := Run(ctx, id, version); err != nil {
		logger.Printf("%s control host: %v", id, err)
		return 1
	}
	return 0
}
