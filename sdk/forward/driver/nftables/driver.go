package nftables

import (
	"context"
	"fmt"
	"slices"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
	"github.com/AnixOps/anix-control/sdk/forward/driver"
)

// Names of what the driver owns on a host.
const (
	// Family and Table name the one nftables table the driver owns.
	Family = "inet"
	Table  = "anixops_fwd"
	// OwnerComment is the table's comment, the ownership mark of the
	// driver contract: a table "inet anixops_fwd" without it is foreign.
	// nft cannot change a table's comment in place, so it never changes;
	// the applied generation and digest are recorded elsewhere (F2c).
	OwnerComment = "anixops-forward-driver v1"
)

// Driver is the nftables forward driver. It is safe for concurrent use: it
// keeps no state besides its configuration.
type Driver struct {
	cfg Config
}

var _ driver.Driver = (*Driver)(nil)

// New answers a driver with the given configuration, or ErrInvalidConfig.
func New(cfg Config) (*Driver, error) {
	if err := cfg.check(); err != nil {
		return nil, err
	}
	cfg.Strategies = slices.Clone(cfg.Strategies)
	cfg.MSSClampInterfaces = slices.Clone(cfg.MSSClampInterfaces)
	slices.Sort(cfg.MSSClampInterfaces)
	return &Driver{cfg: cfg}, nil
}

// Engine answers ENGINE_NFTABLES.
func (d *Driver) Engine() forwardv1.Engine { return forwardv1.Engine_ENGINE_NFTABLES }

// Config answers a copy of the driver's configuration.
func (d *Driver) Config() Config {
	c := d.cfg
	c.Strategies = slices.Clone(c.Strategies)
	c.MSSClampInterfaces = slices.Clone(c.MSSClampInterfaces)
	return c
}

// Capabilities answers the static capabilities of the configuration. It
// does not probe the host yet: an unprobed configuration (no Version) is
// unavailable. F2c replaces this with a probe of nft and the kernel that
// fills a Config before New.
func (d *Driver) Capabilities(ctx context.Context) (*forwardv1.EngineCapabilities, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := &forwardv1.EngineCapabilities{
		Engine:         forwardv1.Engine_ENGINE_NFTABLES,
		Version:        d.cfg.Version,
		Available:      d.cfg.Version != "",
		Ipv6:           d.cfg.IPv6,
		Udp:            d.cfg.UDP,
		Strategies:     slices.Clone(d.cfg.Strategies),
		LinkSecurities: []forwardv1.LinkSecurity{forwardv1.LinkSecurity_LINK_SECURITY_RAW},
		BandwidthLimit: d.cfg.BandwidthLimit,
		Quota:          d.cfg.Quota,
		MaxConns:       d.cfg.MaxConns,
	}
	slices.Sort(c.Strategies)
	if !c.Available {
		c.UnavailableReason = "nftables host not probed: the static configuration has no version (host probing comes with Apply, F2c)"
	}
	return c, nil
}

// errNotYet is what the host-facing methods answer until F2c.
func errNotYet(method string) error {
	return fmt.Errorf("%w: nftables %s is not implemented yet (F2c)", driver.ErrUnsupported, method)
}

// Apply is not implemented yet.
//
// TODO(F2c): check the ownership comment of an existing table first (the
// rendered script flushes the table, so it must never run on a foreign
// one), diff against `nft -j list table inet anixops_fwd`, delete objects
// of removed hops after reading their final counters, record the applied
// generation, state_hash and digest, then `nft -c -f` and `nft -f`.
func (d *Driver) Apply(ctx context.Context, _ driver.Artifact) (driver.ApplyResult, error) {
	if err := ctx.Err(); err != nil {
		return driver.ApplyResult{}, err
	}
	return driver.ApplyResult{}, errNotYet("Apply")
}

// Observe is not implemented yet.
//
// TODO(F2c): read the named counters, quotas and maps of the table.
func (d *Driver) Observe(ctx context.Context) (driver.Observation, error) {
	if err := ctx.Err(); err != nil {
		return driver.Observation{}, err
	}
	return driver.Observation{}, errNotYet("Observe")
}

// SetUpstreams is not implemented yet.
//
// TODO(F2c): rewrite the elements of the hop's balancing maps (the slots
// of the hop's upstreams in rotation, by weight; see Slots).
func (d *Driver) SetUpstreams(ctx context.Context, _ string, _ uint32, _ []driver.Upstream) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errNotYet("SetUpstreams")
}

// Remove is not implemented yet.
//
// TODO(F2c): delete the table when it carries OwnerComment, and the tc
// handles and sysctl drop-in the driver owns.
func (d *Driver) Remove(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return errNotYet("Remove")
}
