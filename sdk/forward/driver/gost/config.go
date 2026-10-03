package gost

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"time"

	forwardv1 "github.com/AnixOps/anix-control/sdk/api/forward/v1"
)

// Names of what the driver owns on a host (owner decisions H13 and H20).
const (
	// OwnerMark marks the driver's state file (and the unit's
	// Description): a configuration directory or unit of the driver's name
	// without it is foreign.
	OwnerMark = "anixops-forward-driver gost v1"
	// UnitName is the systemd unit that runs gost, owned by the Agent and
	// separate from it, so restarting or upgrading the Agent does not drop
	// forwarded connections (H20).
	UnitName = "anixops-gost.service"
	// PinnedVersion is the one gost v3 release this SDK's Agent ships (H20);
	// ci.yml's GOST_VERSION downloads the same release, checksum-verified.
	PinnedVersion = "3.2.6"
	// DefaultDir is where the driver writes gost's configuration and its
	// own state; the Agent owns it and the gost user reads it.
	DefaultDir = "/var/lib/anixops-gost"
	// DefaultRuntimeDir is the unit's RuntimeDirectory, where gost creates
	// its metrics socket.
	DefaultRuntimeDir = "/run/anixops-gost"
	// DefaultLinkCert, DefaultLinkKey and DefaultLinkCA are where the Agent
	// puts the node's link certificate, its key and the link CA bundle for
	// gost: inside DefaultDir, readable by the gost user and no one else.
	DefaultLinkCert = DefaultDir + "/tls/link.crt"
	DefaultLinkKey  = DefaultDir + "/tls/link.key"
	DefaultLinkCA   = DefaultDir + "/tls/link-ca.crt"
	// DefaultBinary is where the Agent's package installs the pinned gost.
	DefaultBinary = "/usr/lib/anixops-agent/gost"
	// ConfigFile, StateFile, MetricsSocket and APISocket are file names
	// inside Dir and RuntimeDir. gost creates both sockets; the API socket
	// is the driver's only way to change a running gost without a reload,
	// and its file permissions are its only key (no auth is configured).
	ConfigFile    = "gost.json"
	StateFile     = "state.json"
	MetricsSocket = "metrics.sock"
	APISocket     = "api.sock"
	// DefaultReadyTimeout bounds the wait for gost to serve a configuration
	// after a start or a reload.
	DefaultReadyTimeout = 10 * time.Second
	// MaxEntries bounds the gost nodes a weighted ROUND_ROBIN or IP_HASH hop
	// expands its upstreams into (see spread): weights are reduced by
	// their greatest common divisor and scaled down to about this many
	// entries, at least one per upstream.
	MaxEntries = 128
)

// Config is the driver's static configuration. The zero value is not
// usable; start from DefaultConfig. Render reads only the fields marked so,
// which keeps it a function of the configuration.
type Config struct {
	// Version is the engine version Capabilities reports ("gost 3.2.6").
	// Empty means the driver is unavailable (Unavailable says why). Probe
	// fills it.
	Version     string
	Unavailable string
	// IPv6 and UDP allow hops that need them (Render).
	IPv6 bool
	UDP  bool
	// Strategies are the balance strategies the driver renders (Render).
	Strategies []forwardv1.BalanceStrategy
	// BandwidthLimit and MaxConns allow hops with those limits (Render).
	BandwidthLimit bool
	MaxConns       bool
	// SoftQuota allows hops with quota_bytes (Render) and reports the quota
	// capability. gost has no byte quota: EnforceQuotas refuses a hop's
	// new connections once its counters reach the quota, checked as often
	// as the Agent calls it (forward-sdk.md section 6.2). Without it a hop
	// with quota_bytes is unsupported.
	SoftQuota bool
	// LinkCert, LinkKey and LinkCA are the node's link certificate, its
	// key and the CA bundle that signs its peers' link certificates, as
	// paths gost reads (Render). Without all three the driver carries RAW
	// links only; with them it terminates and originates TLS, WSS, QUIC
	// and gRPC links with mutual TLS.
	LinkCert string
	LinkKey  string
	LinkCA   string
	// Dir holds the configuration gost runs and the driver's state file.
	Dir string
	// RuntimeDir is where gost creates its metrics socket (Render).
	RuntimeDir string
	// ReadyTimeout bounds the wait for gost to serve a configuration.
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

// DefaultConfig answers a configuration with every feature on, the
// default directories and link certificate paths (Probe drops the paths
// when the files are missing, leaving RAW links only), the soft quota, and
// no Version (so an unprobed driver reports itself unavailable).
func DefaultConfig() Config {
	return Config{
		IPv6:           true,
		UDP:            true,
		Strategies:     AllStrategies(),
		BandwidthLimit: true,
		MaxConns:       true,
		SoftQuota:      true,
		LinkCert:       DefaultLinkCert,
		LinkKey:        DefaultLinkKey,
		LinkCA:         DefaultLinkCA,
		Dir:            DefaultDir,
		RuntimeDir:     DefaultRuntimeDir,
		ReadyTimeout:   DefaultReadyTimeout,
	}
}

// ErrInvalidConfig is returned by New for a configuration it cannot use.
var ErrInvalidConfig = errors.New("gost driver: invalid configuration")

// pathText is what a configured path may look like: absolute, clean, and
// made of characters that need no quoting in a unit file or gost's
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
	if len(c.metricsPath()) > 107 || len(c.apiPath()) > 107 {
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

// linkTLS reports whether the configuration carries encrypted links.
func (c Config) linkTLS() bool { return c.LinkCert != "" }

// configPath, statePath, metricsPath and apiPath are the files the driver
// uses.
func (c Config) configPath() string  { return filepath.Join(c.Dir, ConfigFile) }
func (c Config) statePath() string   { return filepath.Join(c.Dir, StateFile) }
func (c Config) metricsPath() string { return filepath.Join(c.RuntimeDir, MetricsSocket) }
func (c Config) apiPath() string     { return filepath.Join(c.RuntimeDir, APISocket) }
