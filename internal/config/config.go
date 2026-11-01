package config

import (
	_ "embed"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/branding"
	"github.com/AnixOps/anix-control/v5/internal/requestorigin"
	"gopkg.in/yaml.v3"
)

var (
	cfg  *Config
	once sync.Once
)

// defaultsYAML holds the built-in values: the defaults of the operator keys,
// and the values of the keys that are no longer read. A configuration file
// and the ANIX_CONTROL_* variables are overlaid on them key by key.
//
//go:embed defaults.yaml
var defaultsYAML []byte

// Config is Control's configuration. Its YAML paths are the keys of
// keys.go: the operator keys, and the deprecated keys of
// deprecated_keys.yaml that are still read. The fields of keys that are no
// longer read keep their built-in values.
type Config struct {
	Env string `yaml:"env"`
	// DataDir is the data directory (data_dir): root.key and, later, the
	// database and the backups.
	DataDir string `yaml:"data_dir"`
	// Domain is the panel's name: the web, API, subscription and install
	// script address (https://<domain>), the name of the certificate Control
	// obtains in tls.mode acme, and the name nodes dial for gRPC unless
	// grpc.public_name says otherwise.
	Domain string `yaml:"domain"`
	// Listen is the plain HTTP address of tls.mode external, where a CDN or
	// a reverse proxy that terminates TLS sends the whole host. Empty uses
	// server.host:server.port (127.0.0.1:8080 by default) and, with the
	// legacy frontend.enable, server.host:frontend.port too.
	Listen        string              `yaml:"listen"`
	Server        ServerConfig        `yaml:"server"`
	Frontend      FrontendConfig      `yaml:"frontend"`
	Database      DatabaseConfig      `yaml:"database"`
	Cache         CacheConfig         `yaml:"cache"`
	Log           LogConfig           `yaml:"log"`
	JWT           JWTConfig           `yaml:"jwt"`
	Auth          AuthConfig          `yaml:"auth"`
	App           AppConfig           `yaml:"app"`
	TLS           TLSConfig           `yaml:"tls"`
	Forward       ForwardConfig       `yaml:"forward"`
	Plugins       PluginConfig        `yaml:"plugins"`
	GRPC          GRPCConfig          `yaml:"grpc"`
	ModuleRuntime ModuleRuntimeConfig `yaml:"module_runtime"`
	Identity      IdentityConfig      `yaml:"identity"`
	AgentControl  AgentControlConfig  `yaml:"agent_control"`
	AgentInstall  AgentInstallConfig  `yaml:"agent_install"`
	Alerts        AlertsConfig        `yaml:"alerts"`
	// Deprecated records the deprecated keys the configuration sets
	// (deprecated_keys.yaml): Load logs one warning per key.
	Deprecated DeprecatedConfig `yaml:"-"`
}

// ForwardConfig holds the switches of the v4.2 forwarding engines
// (docs/architecture/forward-sdk.md, anixops-protocol.md).
type ForwardConfig struct {
	// AnixOpsExperimental lets routes use the experimental AnixOps engine and
	// link (ENGINE_ANIXOPS, LINK_SECURITY_ANIXOPS): it sets the planner's and
	// the validation's EnableAnixOps. Off by default; the Agents need their own
	// Forward.AnixOps.Enable (their JSON configuration) to advertise the engine, since the planner
	// refuses a node that does not. The wire format of the prototype (ALPN
	// anixops/0) may change without notice.
	AnixOpsExperimental bool `yaml:"anixops_experimental"`
}

// AlertsConfig configures the kernel alert monitor (internal/kernelalerts):
// certificates that were not renewed in time, CAs close to their end, and
// phased processes (the node credential split, the identity cutover) that
// have not moved for too long. Alerts are listed at GET
// /api/v4/kernel/alerts and sent to the administrators through the
// notification channels (Telegram for bound administrators, and the
// notification log).
type AlertsConfig struct {
	// Enabled turns the monitor off when false; nil means on.
	Enabled *bool `yaml:"enabled"`
	// CheckInterval is how often the certificate stores and the phased
	// processes are scanned (a duration, at least 1m; default 15m).
	CheckInterval string `yaml:"check_interval"`
	// LeafExpiryDays caps, in days, how close to its end a leaf certificate
	// (Agent, link, module) may get without having been renewed before it
	// alerts (default 14). The window is the smaller of this and one sixth
	// of the certificate's own lifetime, because a healthy certificate is
	// renewed at two thirds of its lifetime.
	LeafExpiryDays int `yaml:"leaf_expiry_days"`
	// CAExpiryDays is how many days before its end the current module or
	// forward link CA alerts (default 60).
	CAExpiryDays int `yaml:"ca_expiry_days"`
	// RenotifyInterval is the shortest time between two notifications of
	// the same alert (a duration, at least 1h; default 24h). Critical alerts
	// repeat four times as often, phase alerts seven times as rarely.
	RenotifyInterval string `yaml:"renotify_interval"`
	// PhaseStuckAfter is how long a phased process may stay untouched in a
	// non-final phase before it alerts (a duration, at least 1h; default
	// 72h); "0" turns the phase alerts off.
	PhaseStuckAfter string `yaml:"phase_stuck_after"`
}

// Alert monitor defaults.
const (
	DefaultAlertCheckInterval    = 15 * time.Minute
	DefaultAlertLeafExpiryDays   = 14
	DefaultAlertCAExpiryDays     = 60
	DefaultAlertRenotifyInterval = 24 * time.Hour
	DefaultAlertPhaseStuckAfter  = 72 * time.Hour
)

// AlertSettings are the alert monitor's settings with the defaults applied.
type AlertSettings struct {
	Enabled          bool
	CheckInterval    time.Duration
	LeafExpiryDays   int
	CAExpiryDays     int
	RenotifyInterval time.Duration
	// PhaseStuckAfter is zero when the phase alerts are off.
	PhaseStuckAfter time.Duration
}

// Settings returns the effective alert settings. It assumes a validated
// configuration; an unparsable duration falls back to its default.
func (a AlertsConfig) Settings() AlertSettings {
	settings := AlertSettings{
		Enabled:          a.Enabled == nil || *a.Enabled,
		CheckInterval:    alertDuration(a.CheckInterval, DefaultAlertCheckInterval),
		LeafExpiryDays:   a.LeafExpiryDays,
		CAExpiryDays:     a.CAExpiryDays,
		RenotifyInterval: alertDuration(a.RenotifyInterval, DefaultAlertRenotifyInterval),
		PhaseStuckAfter:  alertDuration(a.PhaseStuckAfter, DefaultAlertPhaseStuckAfter),
	}
	if settings.LeafExpiryDays <= 0 {
		settings.LeafExpiryDays = DefaultAlertLeafExpiryDays
	}
	if settings.CAExpiryDays <= 0 {
		settings.CAExpiryDays = DefaultAlertCAExpiryDays
	}
	return settings
}

// alertDuration parses a duration setting; empty and unparsable values give
// fallback, and "0" gives zero.
func alertDuration(raw string, fallback time.Duration) time.Duration {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed < 0 {
		return fallback
	}
	return parsed
}

func (a AlertsConfig) validate() error {
	for _, check := range []struct {
		key, raw string
		min      time.Duration
		allowOff bool
	}{
		{"alerts.check_interval", a.CheckInterval, time.Minute, false},
		{"alerts.renotify_interval", a.RenotifyInterval, time.Hour, false},
		{"alerts.phase_stuck_after", a.PhaseStuckAfter, time.Hour, true},
	} {
		raw := strings.TrimSpace(check.raw)
		if raw == "" {
			continue
		}
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return fmt.Errorf("invalid %s %q: use a duration such as 15m or 24h", check.key, check.raw)
		}
		if check.allowOff && parsed == 0 {
			continue
		}
		if parsed < check.min {
			return fmt.Errorf("invalid %s %q: at least %s", check.key, check.raw, check.min)
		}
	}
	if a.LeafExpiryDays < 0 || a.CAExpiryDays < 0 {
		return fmt.Errorf("alerts.leaf_expiry_days and alerts.ca_expiry_days may not be negative")
	}
	return nil
}

// AgentInstallConfig configures one-command node onboarding (forward-sdk.md,
// section 9): the signed install script Control serves at /install.sh, the
// release metadata it reads from /install/agent.json, and where the Agent
// release is downloaded from.
type AgentInstallConfig struct {
	// PublicURL is the Control address nodes reach, as https://host[:port];
	// empty uses the request's origin (server.trusted_proxies decides
	// whether X-Forwarded-* count).
	PublicURL string `yaml:"public_url"`
	// GRPCTarget is the host:port of the gRPC listener nodes dial (TLS);
	// empty uses PublicURL's host and grpc.port.
	GRPCTarget string `yaml:"grpc_target"`
	// AgentVersion is the Agent release tag installed; empty uses the Agent
	// release this Control was released with (config/AGENT_RELEASE, set by
	// the release build), or "v" + app.version in a development build.
	AgentVersion string `yaml:"agent_version"`
	// ArtifactDir holds Agent release assets as <dir>/<tag>/<asset> with
	// their .dgst (and .sig) files; Control then serves them at
	// /install/agent/<tag>/<asset> (the "control" mirror) and publishes their
	// SHA-256 in /install/agent.json. Empty: the control mirror falls back to
	// GitHub releases.
	ArtifactDir string `yaml:"artifact_dir"`
	// CNMirrorURL is the base URL of a mainland mirror laid out like GitHub
	// release downloads (<base>/<tag>/<asset>). The script never trusts a
	// checksum from the mirror: it takes it from Control or GitHub.
	CNMirrorURL string `yaml:"cn_mirror_url"`
	// SignatureFile is the release signature of the embedded install
	// script (base64 Ed25519 by plugins.official_public_key), served at
	// /install.sh.sig after Control checks it. The release image sets it.
	SignatureFile string `yaml:"signature_file"`
}

// AgentControlConfig holds the retired agent_control keys. AnixOps Agents
// authenticate with enrolled client certificates only (the former
// agent_control.mtls: required), and the legacy agent transports are gone, so
// neither key has an effect; they stay in the struct so that an older
// configuration file still loads.
type AgentControlConfig struct {
	// MTLS is ignored: the mode is always required.
	MTLS string `yaml:"mtls"`
	// LegacySunset is ignored: there are no legacy agent transports.
	LegacySunset string `yaml:"legacy_sunset"`
}

// Validate checks agent_install: absolute https URLs without a query, a
// host:port gRPC target and absolute file paths.
func (a AgentInstallConfig) Validate() error {
	for name, value := range map[string]string{"public_url": a.PublicURL, "cn_mirror_url": a.CNMirrorURL} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
			parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
			return fmt.Errorf("invalid agent_install.%s %q: use an absolute https URL without credentials, query or fragment (nodes download over https only)", name, value)
		}
	}
	if target := strings.TrimSpace(a.GRPCTarget); target != "" {
		host, port, err := net.SplitHostPort(target)
		number, portErr := strconv.Atoi(port)
		if err != nil || host == "" || portErr != nil || number < 1 || number > 65535 {
			return fmt.Errorf("invalid agent_install.grpc_target %q: use host:port", target)
		}
	}
	if version := strings.TrimSpace(a.AgentVersion); version != "" && !agentReleaseTagPattern.MatchString(version) {
		return fmt.Errorf("invalid agent_install.agent_version %q: use a release tag such as v4.2.0 or v4.2.0-rc.1", version)
	}
	for name, value := range map[string]string{"artifact_dir": a.ArtifactDir, "signature_file": a.SignatureFile} {
		if value = strings.TrimSpace(value); value != "" && !filepath.IsAbs(value) {
			return fmt.Errorf("invalid agent_install.%s %q: use an absolute path", name, value)
		}
	}
	return nil
}

// agentReleaseTagPattern matches the release tags the Agent is published
// with.
var agentReleaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-(alpha|beta|rc)(\.[0-9]+(\.(dev|hf)\.[0-9]+)?)?)?$`)

// IdentityConfig configures the identity module when Control runs it as a
// local package host.
type IdentityConfig struct {
	// KEK is the base64 or hex 32-byte key that seals the identity signing
	// keys. It is passed to the local identity-platform host only; a network
	// module gets its own copy (ANIX_IDENTITY_KEK_FILE). Without it identity
	// signs no tokens and the kernel's own HS256 tokens stay in use.
	KEK string `yaml:"kek"`
}

// PluginConfig controls the official plugin trust root and the package
// hosts of the built-in packages, which always run. Third-party plugin
// execution is intentionally unsupported by the control kernel. Agent plugin
// dispatch and topology execution are off until M4 and have no setting
// (service.AgentPluginDispatchEnabled).
type PluginConfig struct {
	OfficialPublicKey                  string `yaml:"official_public_key"`
	IdentityBootstrapPackageDir        string `yaml:"identity_bootstrap_package_dir"`
	ControlPollInterval                string `yaml:"control_poll_interval"`
	ControlHostRuntimeDir              string `yaml:"control_host_runtime_dir"`
	ControlHostArtifactDir             string `yaml:"control_host_artifact_dir"`
	ControlHostStartupTimeout          string `yaml:"control_host_startup_timeout"`
	ControlHostRequestTimeout          string `yaml:"control_host_request_timeout"`
	ControlHostWebSocketSessionTimeout string `yaml:"control_host_websocket_session_timeout"`
	ControlHostMaxRequestBytes         int64  `yaml:"control_host_max_request_bytes"`
	ControlHostMaxResponseBytes        int64  `yaml:"control_host_max_response_bytes"`
}

const (
	// DefaultControlHostPayloadBytes is the request and response body limit
	// used when plugins.control_host_max_*_bytes is unset.
	DefaultControlHostPayloadBytes int64 = 1 << 20
	// MaxControlHostPayloadBytes is the largest accepted configured limit.
	MaxControlHostPayloadBytes int64 = 64 << 20
)

// ControlHostRequestBodyLimit returns the effective
// plugins.control_host_max_request_bytes: the largest /api/v2 request body the
// gateway forwards to a package host.
func (p PluginConfig) ControlHostRequestBodyLimit() int64 {
	return effectiveControlHostPayloadLimit(p.ControlHostMaxRequestBytes)
}

// ControlHostResponseBodyLimit returns the effective
// plugins.control_host_max_response_bytes: the largest package response body,
// enforced by the package bridge, the package-host SDK, and the kernel's host
// client.
func (p PluginConfig) ControlHostResponseBodyLimit() int64 {
	return effectiveControlHostPayloadLimit(p.ControlHostMaxResponseBytes)
}

// ValidatePayloadLimits rejects negative or oversized payload limits. Zero
// means "use the default".
func (p PluginConfig) ValidatePayloadLimits() error {
	for _, limit := range []struct {
		key   string
		value int64
	}{
		{"plugins.control_host_max_request_bytes", p.ControlHostMaxRequestBytes},
		{"plugins.control_host_max_response_bytes", p.ControlHostMaxResponseBytes},
	} {
		if limit.value < 0 || limit.value > MaxControlHostPayloadBytes {
			return fmt.Errorf("invalid %s %d: must be between 0 (default %d) and %d", limit.key, limit.value, DefaultControlHostPayloadBytes, MaxControlHostPayloadBytes)
		}
	}
	return nil
}

func effectiveControlHostPayloadLimit(value int64) int64 {
	if value <= 0 || value > MaxControlHostPayloadBytes {
		return DefaultControlHostPayloadBytes
	}
	return value
}

// ModuleRuntimeConfig controls network modules: packages that run as
// separate services and talk to the kernel over gRPC with mTLS.
type ModuleRuntimeConfig struct {
	Enabled bool `yaml:"enabled"`
	// Listen is the address of the kernel's mTLS module listener (ModulePKI
	// and the remote package bridge).
	Listen string `yaml:"listen"`
	// Cluster names the deployment in every module SPIFFE ID.
	Cluster string `yaml:"cluster"`
	// PKI is "builtin" (the kernel's own CA, with enrollment) or "external"
	// (certificates issued elsewhere, for example by cert-manager).
	PKI string `yaml:"pki"`
	// CAKEK is the base64 or hex 32-byte key that seals the built-in CA key.
	CAKEK string `yaml:"ca_kek"`
	// CertLifetime bounds module and kernel certificates (default 24h).
	CertLifetime string `yaml:"cert_lifetime"`
	// External PKI: the trust bundle and the kernel's own certificate.
	TrustBundleFile string `yaml:"trust_bundle_file"`
	CertFile        string `yaml:"cert_file"`
	KeyFile         string `yaml:"key_file"`
	// DatabaseHost ("host" or "host:port") is the PostgreSQL address in
	// storage leases for remote modules, when it differs from the kernel's.
	DatabaseHost string `yaml:"database_host"`
	// BindTimeout bounds how long starting a remote package waits for a
	// healthy instance (default 2m).
	BindTimeout string `yaml:"bind_timeout"`
}

// Module runtime PKI modes.
const (
	ModulePKIBuiltin  = "builtin"
	ModulePKIExternal = "external"
)

// GRPCConfig controls the node-facing gRPC server (AnixOps Agent nodes connect here).
type GRPCConfig struct {
	// Enable turns the listener on; with domain set it is always on.
	Enable bool   `yaml:"enabled"`
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	// PublicName is the DNS name nodes dial for gRPC when it is not domain:
	// a name that resolves straight to this machine while domain sits behind
	// a CDN, which cannot carry the Agents' mutual TLS. Control obtains a
	// publicly trusted certificate for it over ACME (or serves
	// grpc.tls_cert_file, or tls.cert_file in tls.mode external). Empty:
	// domain.
	PublicName string `yaml:"public_name"`
	// EdgeProxies are the addresses (IPs or CIDRs) of the reverse proxy that
	// terminates the nodes' TLS on another machine (docs/design/edge-grpc).
	// Non-empty: the gRPC listener serves plain HTTP/2 to those addresses
	// only and takes each node's client certificate from the proxy's
	// X-Anix-Client-Cert header. Empty: the listener is TLS.
	EdgeProxies []string `yaml:"edge_proxies"`
	// TLSCertFile and TLSKeyFile are a certificate of the gRPC listener's own
	// (PEM files, reloaded when they change). Empty: the certificate Control
	// manages for the gRPC name.
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
}

// AuthConfig defines authentication security settings.
type AuthConfig struct {
	LoginRateLimit    LoginRateLimitConfig   `yaml:"login_rate_limit"`
	RegisterRateLimit LoginRateLimitConfig   `yaml:"register_rate_limit"`
	Registration      RegistrationAuthConfig `yaml:"registration"`
}

// LoginRateLimitConfig controls brute-force protection for login.
type LoginRateLimitConfig struct {
	Enabled        *bool `yaml:"enabled"`
	MaxAttempts    int   `yaml:"max_attempts"`
	WindowSeconds  int   `yaml:"window_seconds"`
	LockoutSeconds int   `yaml:"lockout_seconds"`
}

// RegistrationAuthConfig controls public account registration policy.
type RegistrationAuthConfig struct {
	Enabled             *bool    `yaml:"enabled"`
	RequireInvite       bool     `yaml:"require_invite"`
	AllowedEmailDomains []string `yaml:"allowed_email_domains"`
	BlockedEmailDomains []string `yaml:"blocked_email_domains"`
}

// TLSConfig selects who terminates TLS for the web side (tls.mode) and,
// in files mode, the certificate files.
type TLSConfig struct {
	// Mode is acme (Control obtains and renews a certificate for domain and
	// serves https on 443, http on 80 for the ACME challenge and the
	// redirect), files (Control serves https on 443 with cert_file and
	// key_file, reloaded when they change) or external (a CDN or a reverse
	// proxy terminates TLS and sends plain http to listen). Empty: acme when
	// domain is set, external otherwise.
	Mode string `yaml:"mode"`
	// CertFile and KeyFile are PEM files: the web certificate in files mode;
	// in external mode the gRPC listener's certificate (instead of one
	// obtained over ACME).
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
	// Enable is not read: the web side's TLS follows Mode. With cert_file
	// and no mode the configuration is refused, since it used to serve
	// https on the web port.
	//
	// Deprecated: set tls.mode.
	Enable bool `yaml:"enable"`
	// Domain is not read (it was only logged); a log line points at the
	// top-level domain key.
	//
	// Deprecated: set domain.
	Domain string `yaml:"domain"`
}

// ServerConfig defines backend server settings.
type ServerConfig struct {
	Host           string     `yaml:"host"`
	Port           int        `yaml:"port"`
	Mode           string     `yaml:"mode"`
	ReadTimeout    int        `yaml:"read_timeout"`
	WriteTimeout   int        `yaml:"write_timeout"`
	TrustedProxies []string   `yaml:"trusted_proxies"` // proxies whose X-Forwarded-* count (internal/requestorigin); nil = loopback, empty = none
	CORS           CORSConfig `yaml:"cors"`
	// ShutdownDrainDelay is how long the server keeps accepting requests after
	// /readyz starts failing on shutdown, so load balancers and Kubernetes
	// endpoints stop routing to it first (Go duration, e.g. "5s"; empty = 0).
	ShutdownDrainDelay string `yaml:"shutdown_drain_delay"`
}

// DrainDelay returns the parsed server.shutdown_drain_delay (0 when unset).
func (s ServerConfig) DrainDelay() (time.Duration, error) {
	if strings.TrimSpace(s.ShutdownDrainDelay) == "" {
		return 0, nil
	}
	delay, err := time.ParseDuration(strings.TrimSpace(s.ShutdownDrainDelay))
	if err != nil || delay < 0 || delay > 5*time.Minute {
		return 0, fmt.Errorf("invalid server.shutdown_drain_delay %q: use a duration between 0 and 5m", s.ShutdownDrainDelay)
	}
	return delay, nil
}

// CORSConfig defines Cross-Origin Resource Sharing settings.
type CORSConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials"`
	MaxAge           int      `yaml:"max_age"` // preflight cache duration in seconds
}

// FrontendConfig defines static frontend serving settings.
type FrontendConfig struct {
	Enable bool   `yaml:"enable"`
	Port   int    `yaml:"port"`
	Path   string `yaml:"path"`
}

// DatabaseConfig defines database connection settings.
type DatabaseConfig struct {
	Driver   string `yaml:"driver"`
	Database string `yaml:"database"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	// SSLMode is the libpq sslmode (disable, allow, prefer, require,
	// verify-ca, verify-full); empty means disable.
	SSLMode string `yaml:"sslmode"`
	// TimeZone is the PostgreSQL session time zone; empty means Asia/Shanghai.
	TimeZone string `yaml:"timezone"`
	// DSN, when set, is used verbatim instead of the fields above.
	DSN             string `yaml:"dsn"`
	LogLevel        string `yaml:"log_level"`
	MaxIdleConns    int    `yaml:"max_idle_conns"`
	MaxOpenConns    int    `yaml:"max_open_conns"`
	ConnMaxLifetime int    `yaml:"conn_max_lifetime"`
}

// CacheConfig defines cache settings.
type CacheConfig struct {
	Driver        string `yaml:"driver"`
	RedisHost     string `yaml:"redis_host"`
	RedisPort     int    `yaml:"redis_port"`
	RedisPassword string `yaml:"redis_password"`
	RedisDB       int    `yaml:"redis_db"`
}

// LogConfig defines log settings.
type LogConfig struct {
	// Format is "text" (default: the historical plain log lines) or "json"
	// (one JSON object per line on stdout, for container log collectors).
	Format     string `yaml:"format"`
	Level      string `yaml:"level"`
	Output     string `yaml:"output"`
	FilePath   string `yaml:"file_path"`
	MaxSize    int    `yaml:"max_size"`
	MaxBackups int    `yaml:"max_backups"`
	MaxAge     int    `yaml:"max_age"`
}

// JWTConfig defines JWT settings.
type JWTConfig struct {
	Secret string `yaml:"secret"`
	Expire int    `yaml:"expire"`
}

// AppConfig defines generic app settings.
type AppConfig struct {
	Name             string `yaml:"name"`
	Version          string `yaml:"version"`
	TrafficLogEnable bool   `yaml:"traffic_log_enable"`
	SubscribePath    string `yaml:"subscribe_path"`
	// app.edition is gone: every official package, orders, payments and
	// affiliate included, is built into Control. A file that still sets it
	// loads, and the key is ignored like any other unknown key.
}

// Load reads the configuration once per process (LoadFile) and logs one
// warning per deprecated key it sets and per unknown ANIX_CONTROL_*
// variable. A non-empty path names a YAML file; an empty path starts from
// the built-in defaults alone.
func Load(path string) (*Config, error) {
	var err error
	once.Do(func() {
		var loaded *Config
		loaded, err = load(path, os.Environ())
		if err == nil {
			cfg = loaded
		}
	})

	return cfg, err
}

func load(path string, environ []string) (*Config, error) {
	loaded, report, err := LoadFile(path, environ)
	for _, warning := range report.Warnings() {
		log.Print(warning)
	}
	return loaded, err
}

// validateTrustedProxies checks server.trusted_proxies.
func validateTrustedProxies(proxies []string) error {
	if _, err := requestorigin.NewPolicy(proxies); err != nil {
		return fmt.Errorf("server.trusted_proxies: %w", err)
	}
	return nil
}

// Defaults returns a fresh copy of the built-in values.
func Defaults() *Config {
	defaults := &Config{}
	if err := yaml.Unmarshal(defaultsYAML, defaults); err != nil {
		panic(fmt.Sprintf("embedded defaults.yaml is invalid: %v", err))
	}
	normalizeBrandDefaults(defaults)
	return defaults
}

// placeholderJWTSecrets are the example values shipped in the config templates.
var placeholderJWTSecrets = map[string]bool{
	"your-jwt-secret-key-change-in-production":               true,
	"CHANGE-THIS-TO-A-VERY-LONG-RANDOM-STRING-IN-PRODUCTION": true,
}

// PlaceholderJWTSecret tells whether secret is a template value of the old
// configuration files: Control does not adopt one (the root key derives the
// secret instead).
func PlaceholderJWTSecret(secret string) bool {
	return placeholderJWTSecrets[strings.TrimSpace(secret)]
}

// ValidateForServer rejects settings the server must not start with. The
// JWT secret, once the root key is resolved, must not be a template value.
func (c *Config) ValidateForServer() error {
	if c == nil {
		return fmt.Errorf("configuration is missing")
	}
	if err := c.ModuleRuntime.validate(); err != nil {
		return err
	}
	if placeholderJWTSecrets[strings.TrimSpace(c.JWT.Secret)] {
		return fmt.Errorf("jwt.secret still has the template value: remove it, and the root key derives the secret")
	}
	return nil
}

func (m ModuleRuntimeConfig) validate() error {
	if !m.Enabled {
		return nil
	}
	if _, err := time.ParseDuration(m.CertLifetimeOrDefault()); err != nil {
		return fmt.Errorf("invalid module_runtime.cert_lifetime %q", m.CertLifetime)
	}
	if value := strings.TrimSpace(m.BindTimeout); value != "" {
		if timeout, err := time.ParseDuration(value); err != nil || timeout <= 0 {
			return fmt.Errorf("invalid module_runtime.bind_timeout %q", m.BindTimeout)
		}
	}
	switch m.PKIOrDefault() {
	case ModulePKIBuiltin:
		// The built-in CA's key is the root key, which every server has.
	case ModulePKIExternal:
		if m.TrustBundleFile == "" || m.CertFile == "" || m.KeyFile == "" {
			return fmt.Errorf("module_runtime.trust_bundle_file, cert_file and key_file are required for the external PKI")
		}
	default:
		return fmt.Errorf("module_runtime.pki must be %q or %q", ModulePKIBuiltin, ModulePKIExternal)
	}
	return nil
}

// ListenOrDefault returns the module listener address, ":7443" when empty.
func (m ModuleRuntimeConfig) ListenOrDefault() string {
	if strings.TrimSpace(m.Listen) == "" {
		return ":7443"
	}
	return strings.TrimSpace(m.Listen)
}

// BindTimeoutOrDefault returns the remote start timeout, 2m when empty or
// invalid (validate rejects invalid values).
func (m ModuleRuntimeConfig) BindTimeoutOrDefault() time.Duration {
	if timeout, err := time.ParseDuration(strings.TrimSpace(m.BindTimeout)); err == nil && timeout > 0 {
		return timeout
	}
	return 2 * time.Minute
}

// ClusterOrDefault returns the configured cluster name, "default" when empty.
func (m ModuleRuntimeConfig) ClusterOrDefault() string {
	if strings.TrimSpace(m.Cluster) == "" {
		return "default"
	}
	return strings.TrimSpace(m.Cluster)
}

// PKIOrDefault returns the configured PKI mode, "builtin" when empty.
func (m ModuleRuntimeConfig) PKIOrDefault() string {
	if strings.TrimSpace(m.PKI) == "" {
		return ModulePKIBuiltin
	}
	return strings.TrimSpace(m.PKI)
}

// BuiltinCA reports whether the kernel runs its built-in CA: pki is builtin
// and either the module runtime is enabled (which requires ca_kek) or
// ca_kek is set. The CA does not depend on the module listener: agent
// enrollment uses it with module_runtime.enabled false.
func (m ModuleRuntimeConfig) BuiltinCA() bool {
	return m.PKIOrDefault() == ModulePKIBuiltin && (m.Enabled || strings.TrimSpace(m.CAKEK) != "")
}

// CertLifetimeOrDefault returns the configured certificate lifetime, 24h when
// empty.
func (m ModuleRuntimeConfig) CertLifetimeOrDefault() string {
	if strings.TrimSpace(m.CertLifetime) == "" {
		return "24h"
	}
	return strings.TrimSpace(m.CertLifetime)
}

func (d DatabaseConfig) validateConnectionSettings() error {
	switch d.SSLMode {
	case "", "disable", "allow", "prefer", "require", "verify-ca", "verify-full":
	default:
		return fmt.Errorf("invalid database.sslmode %q", d.SSLMode)
	}
	if d.TimeZone != "" {
		if _, err := time.LoadLocation(d.TimeZone); err != nil || strings.ContainsAny(d.TimeZone, " \t'\"\\") {
			return fmt.Errorf("invalid database.timezone %q", d.TimeZone)
		}
	}
	return nil
}

// Get returns the loaded config.
func Get() *Config {
	return cfg
}

// Set replaces the loaded config, mainly for tests.
func Set(c *Config) {
	normalizeBrandDefaults(c)
	cfg = c
}

// PrepareSubscribePath normalizes the configured public subscription path,
// updates the in-memory configuration when present, and fails before route
// registration if the path could overlap the versioned API namespace.
func PrepareSubscribePath(c *Config) string {
	value := ""
	if c != nil {
		value = c.App.SubscribePath
	}

	normalized, err := NormalizeSubscribePath(value)
	if err != nil {
		panic("invalid app.subscribe_path: " + err.Error())
	}
	if c != nil {
		c.App.SubscribePath = normalized
		Set(c)
	}
	return normalized
}

// NormalizeSubscribePath cleans the public subscription path and rejects
// values that could overlap the versioned API namespace.
func NormalizeSubscribePath(value string) (string, error) {
	cleaned := strings.Trim(strings.TrimSpace(value), "/")
	if cleaned == "" {
		return "s", nil
	}

	normalized := strings.TrimPrefix(path.Clean("/"+cleaned), "/")
	if normalized == "" || normalized == "." {
		return "s", nil
	}
	if strings.ContainsAny(normalized, ":*") {
		return "", fmt.Errorf("subscription path must not contain Gin parameters or wildcards")
	}
	if normalized == "api/v2" || strings.HasPrefix(normalized, "api/v2/") {
		return "", fmt.Errorf("subscription path must not overlap /api/v2")
	}
	if normalized == "install" || normalized == "install.sh" || strings.HasPrefix(normalized, "install/") {
		return "", fmt.Errorf("subscription path must not overlap the Agent installer's /install.sh and /install/")
	}

	return normalized, nil
}

func normalizeBrandDefaults(c *Config) {
	if c == nil {
		return
	}
	if strings.TrimSpace(c.App.Name) == "" {
		c.App.Name = branding.ControlName
	}
}
