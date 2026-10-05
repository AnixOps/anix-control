package anixops

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Names of what the driver owns on a host (docs/architecture/anixops-protocol.md
// section 6.2, decision P1).
const (
	// OwnerMark marks the driver's state file (and the unit's Description): a
	// directory or unit of the driver's name without it is foreign.
	OwnerMark = "anixops-forward-driver anixops v1"
	// UnitName is the systemd unit that runs the relay, separate from the
	// Agent so restarting or upgrading the Agent keeps forwarding.
	UnitName = "anixops-relay.service"
	// DefaultDir is where the driver writes the relay's configuration, its own
	// state, the stateless reset key and the link files (tls/).
	DefaultDir = "/var/lib/anixops-relay"
	// DefaultRuntimeDir is the unit's RuntimeDirectory: the control socket.
	DefaultRuntimeDir = "/run/anixops-relay"
	// DefaultBinary is where the Agent's package installs the relay.
	DefaultBinary = "/usr/lib/anixops-agent/anixops-relay"
	// DefaultUser is the account the relay runs as.
	DefaultUser = "anixops-relay"
	// DefaultLinkCert, DefaultLinkKey and DefaultLinkCA are where the Agent
	// puts the node's link certificate, its key and the link CA bundle.
	DefaultLinkCert = DefaultDir + "/tls/link.crt"
	DefaultLinkKey  = DefaultDir + "/tls/link.key"
	DefaultLinkCA   = DefaultDir + "/tls/link-ca.crt"
	// File names inside Dir and RuntimeDir.
	ConfigFile    = "relay.json"
	StateFile     = "state.json"
	ResetKeyFile  = "stateless-reset.key"
	ControlSocket = "control.sock"
	// DefaultReadyTimeout bounds the wait for the relay to answer after a
	// start.
	DefaultReadyTimeout = 10 * time.Second
	// MaxUpstreams bounds the upstreams of one hop.
	MaxUpstreams = 128
	// QUICSocketBuffer is the net.core.rmem_max and wmem_max QUIC carriers
	// need (anixops-protocol.md section 5.6); Probe reports the host's.
	QUICSocketBuffer = 7_500_000
)

// WireVersions are the wire versions of the engine's own link protocol this
// build speaks: the prototype's anixops/0.
var WireVersions = []uint32{0}

// Config is the driver's static configuration. The zero value is not usable;
// start from DefaultConfig. Render reads only the fields marked so, which
// keeps it a function of the configuration.
type Config struct {
	// Version is the engine version Capabilities reports ("anixops-relay
	// 4.2.0-rc.2"). Empty means the driver is unavailable (Unavailable says
	// why). Probe fills it.
	Version     string
	Unavailable string
	// IPv6 and UDP allow hops that need them (Render).
	IPv6 bool
	UDP  bool
	// Strategies are the balance strategies the driver renders (Render).
	Strategies []forwardv1.BalanceStrategy
	// BandwidthLimit, Quota and MaxConns allow hops with those limits
	// (Render). The quota is exact: the relay counts every byte it moves.
	BandwidthLimit bool
	Quota          bool
	MaxConns       bool
	// ProtocolVersions are the wire versions the relay binary speaks (Probe
	// reads them from `anixops-relay -V`).
	ProtocolVersions []uint32
	// Carriers are the concrete carriers the host serves and dials (Render):
	// PLAIN always, TLS_TCP and QUIC with the link files. Never AUTO.
	Carriers []forwardv1.AnixOpsCarrier
	// LinkCert, LinkKey and LinkCA are the node's link certificate, its key
	// and the CA bundle that signs its peers' link certificates, as paths the
	// relay reads (Render). Without all three the driver carries RAW and PLAIN
	// links only.
	LinkCert string
	LinkKey  string
	LinkCA   string
	// Dir holds the configuration the relay runs, the driver's state file and
	// the stateless reset key; RuntimeDir the control socket.
	Dir        string
	RuntimeDir string
	// ReadyTimeout bounds the wait for the relay to answer after a start.
	ReadyTimeout time.Duration
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
// directories and link file paths, the three carriers (Probe drops the
// encrypted ones when the files are missing) and no Version, so an unprobed
// driver reports itself unavailable.
func DefaultConfig() Config {
	return Config{
		IPv6:             true,
		UDP:              true,
		Strategies:       AllStrategies(),
		BandwidthLimit:   true,
		Quota:            true,
		MaxConns:         true,
		ProtocolVersions: slices.Clone(WireVersions),
		Carriers: []forwardv1.AnixOpsCarrier{
			forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP,
			forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC,
			forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN,
		},
		LinkCert:     DefaultLinkCert,
		LinkKey:      DefaultLinkKey,
		LinkCA:       DefaultLinkCA,
		Dir:          DefaultDir,
		RuntimeDir:   DefaultRuntimeDir,
		ReadyTimeout: DefaultReadyTimeout,
	}
}

// ErrInvalidConfig is returned by New for a configuration it cannot use.
var ErrInvalidConfig = errors.New("anixops driver: invalid configuration")

// pathText is what a configured path may look like: absolute, clean, and made
// of characters that need no quoting in a unit file or the relay's
// configuration.
var pathText = regexp.MustCompile(`^/[A-Za-z0-9._/-]{0,254}$`)

// versionText and reasonText bound what Capabilities may report.
var (
	versionText = regexp.MustCompile(`^[ -~]{0,64}$`)
	reasonText  = regexp.MustCompile(`^[ -~]{0,512}$`)
)

func checkPath(what, p string, optional bool) error {
	if p == "" && optional {
		return nil
	}
	if !pathText.MatchString(p) || filepath.Clean(p) != p {
		return fmt.Errorf("%w: %s %q is not a clean absolute path of letters, digits, '.', '_', '-' and '/'", ErrInvalidConfig, what, p)
	}
	return nil
}

func (c Config) check() error {
	for _, s := range c.Strategies {
		if !slices.Contains(AllStrategies(), s) {
			return fmt.Errorf("%w: unknown strategy %v", ErrInvalidConfig, s)
		}
	}
	for _, car := range c.Carriers {
		switch car {
		case forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP, forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC, forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_PLAIN:
		default:
			return fmt.Errorf("%w: carrier %v is not a concrete carrier (TLS_TCP, QUIC, PLAIN)", ErrInvalidConfig, car)
		}
	}
	if err := checkPath("Dir", c.Dir, false); err != nil {
		return err
	}
	if err := checkPath("RuntimeDir", c.RuntimeDir, false); err != nil {
		return err
	}
	set := 0
	for _, f := range []struct{ what, p string }{{"LinkCert", c.LinkCert}, {"LinkKey", c.LinkKey}, {"LinkCA", c.LinkCA}} {
		if err := checkPath(f.what, f.p, true); err != nil {
			return err
		}
		if f.p != "" {
			set++
		}
	}
	if set != 0 && set != 3 {
		return fmt.Errorf("%w: LinkCert, LinkKey and LinkCA go together", ErrInvalidConfig)
	}
	if (c.hasCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_TLS_TCP) || c.hasCarrier(forwardv1.AnixOpsCarrier_ANIXOPS_CARRIER_QUIC)) && set == 0 {
		return fmt.Errorf("%w: the TLS_TCP and QUIC carriers need the link files", ErrInvalidConfig)
	}
	if len(c.socketPath()) > 107 {
		return fmt.Errorf("%w: RuntimeDir %q is too long for a unix socket path", ErrInvalidConfig, c.RuntimeDir)
	}
	if c.ReadyTimeout <= 0 {
		return fmt.Errorf("%w: ReadyTimeout must be positive", ErrInvalidConfig)
	}
	if !versionText.MatchString(c.Version) || !reasonText.MatchString(c.Unavailable) {
		return fmt.Errorf("%w: version or unavailable reason is not short printable text", ErrInvalidConfig)
	}
	return nil
}

func (c Config) hasCarrier(x forwardv1.AnixOpsCarrier) bool { return slices.Contains(c.Carriers, x) }

func (c Config) linkFiles() bool { return c.LinkCert != "" }

func (c Config) configPath() string { return filepath.Join(c.Dir, ConfigFile) }
func (c Config) statePath() string  { return filepath.Join(c.Dir, StateFile) }
func (c Config) resetPath() string  { return filepath.Join(c.Dir, ResetKeyFile) }
func (c Config) socketPath() string { return filepath.Join(c.RuntimeDir, ControlSocket) }
