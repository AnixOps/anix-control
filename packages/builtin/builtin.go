// Package builtin is the table of the official package hosts linked into
// the Control binary. The kernel runs a host inside its own process
// (internal/pluginhost ManagerConfig.InProcess, cmd/server).
//
// The table names every official package of packages/shared/build_package.py
// (PACKAGE_SPECS). TestHostsMatchThePackageMatrix keeps the two lists equal.
package builtin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	forward "github.com/AnixOps/anix-control/v5/packages/forward/control"
	identityplatform "github.com/AnixOps/anix-control/v5/packages/identity-platform/control"
	knowledge "github.com/AnixOps/anix-control/v5/packages/knowledge/control"
	machinetelemetry "github.com/AnixOps/anix-control/v5/packages/machine-telemetry/control"
	notification "github.com/AnixOps/anix-control/v5/packages/notification/control"
	plan "github.com/AnixOps/anix-control/v5/packages/plan/control"
	platform "github.com/AnixOps/anix-control/v5/packages/platform/control"
	protocolruntime "github.com/AnixOps/anix-control/v5/packages/protocol-runtime/control"
	proxynode "github.com/AnixOps/anix-control/v5/packages/proxy-node/control"
	subscription "github.com/AnixOps/anix-control/v5/packages/subscription/control"
	ticket "github.com/AnixOps/anix-control/v5/packages/ticket/control"
	wireguard "github.com/AnixOps/anix-control/v5/packages/wireguard/control"
)

// Host is one official package's Control host.
type Host struct {
	// ID is the package id.
	ID string
	// Run serves the host for one release of the package.
	Run func(ctx context.Context, version string) error
}

var hosts = []Host{
	{ID: forward.PackageID, Run: forward.Run},
	{ID: identityplatform.PackageID, Run: identityplatform.Run},
	{ID: knowledge.PackageID, Run: knowledge.Run},
	{ID: machinetelemetry.PackageID, Run: machinetelemetry.Run},
	{ID: notification.PackageID, Run: notification.Run},
	{ID: plan.PackageID, Run: plan.Run},
	{ID: platform.PackageID, Run: platform.Run},
	{ID: protocolruntime.PackageID, Run: protocolruntime.Run},
	{ID: proxynode.PackageID, Run: proxynode.Run},
	{ID: subscription.PackageID, Run: subscription.Run},
	{ID: ticket.PackageID, Run: ticket.Run},
	{ID: wireguard.PackageID, Run: wireguard.Run},
}

// Register adds the host of a package that is not part of the community set.
// The commercial packages register theirs from a file of their own
// (commercial.go), so a build without that file links none of them. It is
// meant for init functions: a package id registered twice panics.
func Register(host Host) {
	if _, ok := Lookup(host.ID); ok {
		panic("builtin: package host registered twice: " + host.ID)
	}
	hosts = append(hosts, host)
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
