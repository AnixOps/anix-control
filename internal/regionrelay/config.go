// Package regionrelay is the region relay (cmd/anix-relay,
// docs/design/region-relay): a machine inside a region, mainland China
// first, that the region's nodes dial instead of Control.
//
// It has two parts and nothing else: no web interface, no API, no node
// state.
//
//   - Forwards: TCP passthrough. Each node connection is copied byte for
//     byte to Control. TLS runs end to end between the node and Control:
//     the node verifies Control's certificate by Control's name, Control
//     verifies the node's client certificate, and the relay holds neither
//     key and cannot read or change the stream. Optionally the relay puts a
//     PROXY v2 header in front, so Control still sees the node's address
//     (Control believes it only from grpc.relay_sources).
//   - Cache: a read-only cache of release files addressed by their SHA-256.
//     A node asks for /v1/sha256/<hex>?src=<release URL>; the relay serves
//     its copy, or downloads src once (only from the configured origins),
//     checks the SHA-256 and keeps it. Integrity comes from the hash and
//     from the release signature the node still verifies; the relay has no
//     signing key, so it cannot make or change a release.
package regionrelay

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Defaults of the optional settings.
const (
	DefaultDialTimeout       = 10 * time.Second
	DefaultKeepAlive         = 30 * time.Second
	DefaultMaxConnections    = 20000
	DefaultCacheMaxBytes     = int64(20) << 30
	DefaultCacheObjectBytes  = int64(2) << 30
	DefaultCacheFetches      = 4
	DefaultCacheFetchTimeout = 30 * time.Minute
)

// Config is the relay's configuration file (YAML). Unknown keys are refused.
type Config struct {
	// Forwards are the TCP passthroughs, at least one unless Cache is set.
	Forwards []ForwardConfig `yaml:"forwards"`
	// Cache is the release file cache; nil runs none.
	Cache *CacheConfig `yaml:"cache"`
}

// ForwardConfig is one TCP passthrough.
type ForwardConfig struct {
	// Name labels the forward in the logs.
	Name string `yaml:"name"`
	// Listen is the address nodes dial (":50051").
	Listen string `yaml:"listen"`
	// Upstreams are Control's addresses (host:port), tried in order; the
	// relay stays on the last one that answered until it fails.
	Upstreams []string `yaml:"upstreams"`
	// ProxyProtocol puts a PROXY v2 header with the node's address in front
	// of every connection. Control must list this relay in
	// grpc.relay_sources, or it refuses the connection.
	ProxyProtocol bool `yaml:"proxy_protocol"`
	// MaxConnections bounds the open node connections; zero is
	// DefaultMaxConnections.
	MaxConnections int `yaml:"max_connections"`
	// DialTimeout bounds one upstream dial; zero is DefaultDialTimeout.
	DialTimeout time.Duration `yaml:"dial_timeout"`
	// KeepAlive is the TCP keepalive period on both sides; zero is
	// DefaultKeepAlive. There is no idle timeout: the control streams are
	// long-lived and carry their own keepalive.
	KeepAlive time.Duration `yaml:"keep_alive"`
}

// CacheConfig is the release file cache.
type CacheConfig struct {
	// Listen is the cache's HTTPS address (":8443").
	Listen string `yaml:"listen"`
	// TLSCertFile and TLSKeyFile are the cache's certificate, valid for the
	// name nodes use for the cache. Nodes verify it as any HTTPS server.
	TLSCertFile string `yaml:"tls_cert_file"`
	TLSKeyFile  string `yaml:"tls_key_file"`
	// Dir holds the cached files (Dir/sha256/<hex>) and partial downloads
	// (Dir/tmp).
	Dir string `yaml:"dir"`
	// Origins are the URL prefixes the relay downloads from
	// ("https://github.com/AnixOps/"). A src outside them is refused, so the
	// cache is not an open proxy. Each must be an https URL ending in "/".
	Origins []string `yaml:"origins"`
	// UpstreamProxy, when set, is an HTTP proxy the downloads go through
	// (for example over the backbone to Control's region).
	UpstreamProxy string `yaml:"upstream_proxy"`
	// AllowPrivateOrigins lets downloads reach private, loopback and
	// link-local addresses. Off: an origin or a redirect that resolves to
	// one is refused.
	AllowPrivateOrigins bool `yaml:"allow_private_origins"`
	// MaxBytes bounds the cache; the least recently used files go first.
	// Zero is DefaultCacheMaxBytes.
	MaxBytes int64 `yaml:"max_bytes"`
	// MaxObjectBytes bounds one file; zero is DefaultCacheObjectBytes.
	MaxObjectBytes int64 `yaml:"max_object_bytes"`
	// MaxFetches bounds the downloads that run at once; zero is
	// DefaultCacheFetches.
	MaxFetches int `yaml:"max_fetches"`
	// FetchTimeout bounds one download; zero is DefaultCacheFetchTimeout.
	FetchTimeout time.Duration `yaml:"fetch_timeout"`
}

// LoadConfig reads and validates the file at path.
func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path) // #nosec G304 -- the operator's -config path.
	if err != nil {
		return nil, err
	}
	return ParseConfig(raw)
}

// ParseConfig decodes and validates a configuration, filling the defaults.
func ParseConfig(raw []byte) (*Config, error) {
	decoder := yaml.NewDecoder(strings.NewReader(string(raw)))
	decoder.KnownFields(true)
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("relay configuration: %w", err)
	}
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("relay configuration: %w", err)
	}
	return &config, nil
}

func (c *Config) validate() error {
	if len(c.Forwards) == 0 && c.Cache == nil {
		return errors.New("nothing to run: set forwards, cache or both")
	}
	listens := map[string]string{}
	claim := func(address, owner string) error {
		if previous, taken := listens[address]; taken {
			return fmt.Errorf("%s and %s both listen on %s", previous, owner, address)
		}
		listens[address] = owner
		return nil
	}
	names := map[string]bool{}
	for i := range c.Forwards {
		forward := &c.Forwards[i]
		forward.Name = strings.TrimSpace(forward.Name)
		if forward.Name == "" {
			forward.Name = "forward-" + strconv.Itoa(i+1)
		}
		if names[forward.Name] {
			return fmt.Errorf("forward name %q is used twice", forward.Name)
		}
		names[forward.Name] = true
		owner := "forward " + strconv.Quote(forward.Name)
		if err := validListen(forward.Listen); err != nil {
			return fmt.Errorf("%s: listen: %w", owner, err)
		}
		if err := claim(forward.Listen, owner); err != nil {
			return err
		}
		if len(forward.Upstreams) == 0 {
			return fmt.Errorf("%s: no upstreams", owner)
		}
		for j, upstream := range forward.Upstreams {
			upstream = strings.TrimSpace(upstream)
			if err := validHostPort(upstream); err != nil {
				return fmt.Errorf("%s: upstream %q: %w", owner, upstream, err)
			}
			forward.Upstreams[j] = upstream
		}
		if forward.MaxConnections < 0 || forward.DialTimeout < 0 || forward.KeepAlive < 0 {
			return fmt.Errorf("%s: max_connections, dial_timeout and keep_alive cannot be negative", owner)
		}
		if forward.MaxConnections == 0 {
			forward.MaxConnections = DefaultMaxConnections
		}
		if forward.DialTimeout == 0 {
			forward.DialTimeout = DefaultDialTimeout
		}
		if forward.KeepAlive == 0 {
			forward.KeepAlive = DefaultKeepAlive
		}
	}
	if c.Cache == nil {
		return nil
	}
	cache := c.Cache
	if err := validListen(cache.Listen); err != nil {
		return fmt.Errorf("cache: listen: %w", err)
	}
	if err := claim(cache.Listen, "cache"); err != nil {
		return err
	}
	if strings.TrimSpace(cache.TLSCertFile) == "" || strings.TrimSpace(cache.TLSKeyFile) == "" {
		return errors.New("cache: tls_cert_file and tls_key_file are required: nodes download over HTTPS only")
	}
	if !filepath.IsAbs(cache.Dir) {
		return errors.New("cache: dir must be an absolute path")
	}
	if len(cache.Origins) == 0 {
		return errors.New("cache: no origins: the cache would have nothing to download from")
	}
	for i, origin := range cache.Origins {
		origin = strings.TrimSpace(origin)
		parsed, err := url.Parse(origin)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
			parsed.RawQuery != "" || parsed.Fragment != "" || !strings.HasSuffix(origin, "/") {
			return fmt.Errorf("cache: origin %q must be an https URL prefix ending in /, without credentials, query or fragment", origin)
		}
		cache.Origins[i] = origin
	}
	if proxy := strings.TrimSpace(cache.UpstreamProxy); proxy != "" {
		parsed, err := url.Parse(proxy)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return fmt.Errorf("cache: upstream_proxy %q must be an http or https URL", proxy)
		}
		cache.UpstreamProxy = proxy
	}
	if cache.MaxBytes < 0 || cache.MaxObjectBytes < 0 || cache.MaxFetches < 0 || cache.FetchTimeout < 0 {
		return errors.New("cache: max_bytes, max_object_bytes, max_fetches and fetch_timeout cannot be negative")
	}
	if cache.MaxBytes == 0 {
		cache.MaxBytes = DefaultCacheMaxBytes
	}
	if cache.MaxObjectBytes == 0 {
		cache.MaxObjectBytes = DefaultCacheObjectBytes
	}
	if cache.MaxObjectBytes > cache.MaxBytes {
		return errors.New("cache: max_object_bytes is over max_bytes")
	}
	if cache.MaxFetches == 0 {
		cache.MaxFetches = DefaultCacheFetches
	}
	if cache.FetchTimeout == 0 {
		cache.FetchTimeout = DefaultCacheFetchTimeout
	}
	return nil
}

func validListen(address string) error {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("port %q is not 1 to 65535", port)
	}
	return nil
}

func validHostPort(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if host == "" {
		return errors.New("no host")
	}
	return validListen(address)
}
