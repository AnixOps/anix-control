package nftables

import (
	"errors"
	"fmt"
	"math/bits"
	"regexp"
	"slices"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Defaults of Config (forward-sdk.md section 6.1; owner decision H13 for the
// mark mask).
const (
	// DefaultMarkMask is the connection mark bits the driver owns: 4095
	// hops per node. It is configurable per node to stay clear of other
	// mark users (Docker, WireGuard, policy routing).
	DefaultMarkMask uint32 = 0x0fff0000
	// DefaultDirectionBit is the packet mark bit that tells tc the
	// direction of a rate-limited packet: set for reply (down) packets.
	DefaultDirectionBit uint32 = 0x00000001
	// DefaultSlots is the size of every hop's balancing map: the modulus
	// of its numgen or jhash expression. It is fixed when the hop is
	// rendered, so SetUpstreams can change which upstreams are in
	// rotation and their weights by rewriting map elements only. It covers
	// the most upstreams a hop can have (64 targets plus 16 next-hop nodes
	// under the PREFERRED direct mode).
	DefaultSlots uint32 = 128
	// MaxSlots bounds Config.Slots.
	MaxSlots uint32 = 4096
	// DefaultTCHandle is the major handle of the HTB root qdisc the driver
	// owns on each of Config.LimitInterfaces ("af00:"). A root qdisc with
	// another handle is foreign: the driver never replaces it.
	DefaultTCHandle uint16 = 0xaf00
)

// maxTCMark is the largest mark index of a rate-limited hop: its tc class
// minors are 2*mark and 2*mark+1, below 0xffff.
const maxTCMark = 0x7ffe

// Config is the driver's static configuration. The zero value is not
// usable; start from DefaultConfig.
type Config struct {
	// Version is the engine version Capabilities reports ("nft 1.0.9").
	// Empty means the driver is unavailable (Unavailable says why), and
	// Capabilities answers Available false. Probe fills it, and turns off
	// the features below the host lacks, before New.
	Version string
	// Unavailable is why the driver cannot run on this host when Version
	// is empty ("nft not installed", "no CAP_NET_ADMIN").
	Unavailable string
	// IPv6 and UDP allow hops that need them.
	IPv6 bool
	UDP  bool
	// Strategies are the balance strategies the driver renders.
	// LEAST_CONN renders as weighted random; the Agent's health loop
	// re-weights it from connection counts (forward-sdk.md section 7.1).
	Strategies []forwardv1.BalanceStrategy
	// BandwidthLimit, Quota and MaxConns allow hops with those limits.
	// Render marks packets of rate-limited hops for tc; Apply adds an HTB
	// class per hop and direction on every LimitInterfaces device, so
	// BandwidthLimit needs at least one (Probe turns it off without).
	BandwidthLimit bool
	Quota          bool
	MaxConns       bool
	// MarkMask is the contiguous run of connection mark bits the driver
	// owns. A hop's mark (an index from the planner, 1 up to the mask's
	// width) is shifted into it. Bits outside the mask are preserved.
	MarkMask uint32
	// DirectionBit is the single packet mark bit set on reply packets of
	// rate-limited hops; it must lie outside MarkMask.
	DirectionBit uint32
	// Slots is every balancing map's size; see DefaultSlots.
	Slots uint32
	// MSSClampInterfaces are the encapsulating egress interfaces (a
	// WireGuard or GRE device in front of a private line) on which the
	// SYNs of forwarded connections have their MSS clamped to the route's
	// MTU. Empty for none.
	MSSClampInterfaces []string
	// LimitInterfaces are the egress interfaces on which Apply limits the
	// bandwidth of rate-limited hops (both directions of a forwarded
	// connection leave the node as egress, on the interface facing the
	// client or the upstream). Empty for none.
	LimitInterfaces []string
	// TCHandle is the major handle of the driver's root qdisc on
	// LimitInterfaces; see DefaultTCHandle.
	TCHandle uint16
}

// AllStrategies lists every balance strategy the driver can render.
func AllStrategies() []forwardv1.BalanceStrategy {
	return []forwardv1.BalanceStrategy{
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_ROUND_ROBIN,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_RANDOM,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_IP_HASH,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_LEAST_CONN,
		forwardv1.BalanceStrategy_BALANCE_STRATEGY_FAILOVER,
	}
}

// DefaultConfig answers a configuration with every feature on, the default
// mark mask, direction bit and slots, no MSS clamping and no Version (so an
// unprobed driver reports itself unavailable).
func DefaultConfig() Config {
	return Config{
		IPv6:           true,
		UDP:            true,
		Strategies:     AllStrategies(),
		BandwidthLimit: true,
		Quota:          true,
		MaxConns:       true,
		MarkMask:       DefaultMarkMask,
		DirectionBit:   DefaultDirectionBit,
		Slots:          DefaultSlots,
		TCHandle:       DefaultTCHandle,
	}
}

// ErrInvalidConfig is returned by New for a configuration it cannot use.
var ErrInvalidConfig = errors.New("nftables driver: invalid configuration")

// interfaceName is a Linux interface name (IFNAMSIZ 16, so at most 15
// bytes) restricted to characters that need no quoting in nft.
var interfaceName = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,15}$`)

// versionText is what Capabilities may report as a version.
var versionText = regexp.MustCompile(`^[ -~]{0,64}$`)

// reasonText is what Capabilities may report as an unavailable reason.
var reasonText = regexp.MustCompile(`^[ -~]{0,512}$`)

func (c Config) check() error {
	bad := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidConfig, fmt.Sprintf(format, args...))
	}
	if c.MarkMask == 0 {
		return bad("mark mask is zero")
	}
	shift := bits.TrailingZeros32(c.MarkMask)
	if run := c.MarkMask >> shift; run&(run+1) != 0 {
		return bad("mark mask %#08x is not one contiguous run of bits", c.MarkMask)
	}
	if bits.OnesCount32(c.DirectionBit) != 1 {
		return bad("direction bit %#08x is not a single bit", c.DirectionBit)
	}
	if c.DirectionBit&c.MarkMask != 0 {
		return bad("direction bit %#08x lies inside the mark mask %#08x", c.DirectionBit, c.MarkMask)
	}
	if c.Slots < 1 || c.Slots > MaxSlots {
		return bad("slots %d outside 1..%d", c.Slots, MaxSlots)
	}
	for _, s := range c.Strategies {
		if !slices.Contains(AllStrategies(), s) {
			return bad("unknown strategy %v", s)
		}
	}
	for _, list := range []struct {
		what  string
		names []string
	}{{"MSS clamp", c.MSSClampInterfaces}, {"limit", c.LimitInterfaces}} {
		seen := map[string]bool{}
		for _, name := range list.names {
			if !interfaceName.MatchString(name) {
				return bad("%s interface %q is not an interface name", list.what, name)
			}
			if seen[name] {
				return bad("%s interface %q twice", list.what, name)
			}
			seen[name] = true
		}
	}
	if c.TCHandle == 0 || c.TCHandle == 0xffff {
		return bad("tc handle %#x is reserved", c.TCHandle)
	}
	if !versionText.MatchString(c.Version) || !reasonText.MatchString(c.Unavailable) {
		return bad("version or unavailable reason is not short printable text")
	}
	return nil
}

// markShift answers the shift that puts a mark index into the mask.
func (c Config) markShift() int { return bits.TrailingZeros32(c.MarkMask) }

// maxMark answers the largest mark index the mask holds.
func (c Config) maxMark() uint32 { return c.MarkMask >> c.markShift() }
