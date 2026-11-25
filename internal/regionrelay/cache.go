package regionrelay

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sync/singleflight"
)

// CachePathPrefix is the only path the cache serves: CachePathPrefix +
// the lower-case hex SHA-256 of the file.
const CachePathPrefix = "/v1/sha256/"

// CacheSourceParam is the query parameter that names where the relay may
// download a file it does not have yet.
const CacheSourceParam = "src"

var sha256Hex = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Cache is the release file cache's HTTP handler.
type Cache struct {
	config CacheConfig
	logger *slog.Logger
	client *http.Client

	fetches singleflight.Group
	slots   chan struct{}
	// mu serialises eviction against new files.
	mu sync.Mutex

	// allowedSources are the addresses served; empty serves any.
	allowedSources []netip.Prefix
	// requestsMu guards requests, the open requests of each address.
	requestsMu sync.Mutex
	requests   map[string]int
	// misses are the downloads each address started (requestsMu).
	misses map[string]*missCount
	// sumsMu guards sums: the SHA-256 each source was found to have, so a
	// request that names another hash for a known source is refused without
	// downloading it again.
	sumsMu sync.Mutex
	sums   map[string]knownSum
	// now is time.Now, replaced in tests.
	now func() time.Time
}

// errOverLimit is a download larger than max_object_bytes.
var errOverLimit = errors.New("the file is larger than max_object_bytes")

// errHashMismatch is a download whose SHA-256 is not the one asked for.
var errHashMismatch = errors.New("the downloaded file's SHA-256 does not match")

// NewCache prepares the cache directory and the download client.
func NewCache(config CacheConfig, logger *slog.Logger) (*Cache, error) {
	if logger == nil {
		logger = slog.Default()
	}
	for _, dir := range []string{filepath.Join(config.Dir, "sha256"), filepath.Join(config.Dir, "tmp")} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return nil, err
		}
	}
	// Partial downloads of a previous run are worthless.
	if entries, err := os.ReadDir(filepath.Join(config.Dir, "tmp")); err == nil {
		for _, entry := range entries {
			_ = os.Remove(filepath.Join(config.Dir, "tmp", entry.Name()))
		}
	}
	client, err := downloadClient(config)
	if err != nil {
		return nil, err
	}
	var allowed []netip.Prefix
	for _, source := range config.AllowedSources {
		prefix, err := parseSource(source)
		if err != nil {
			return nil, fmt.Errorf("allowed_sources %q: %w", source, err)
		}
		allowed = append(allowed, prefix)
	}
	if config.MaxRequestsPerSource <= 0 {
		config.MaxRequestsPerSource = DefaultCacheRequestsPerSource
	}
	if config.MaxMissesPerSource <= 0 {
		config.MaxMissesPerSource = DefaultCacheMissesPerSource
	}
	return &Cache{
		config:         config,
		logger:         logger.With("component", "cache"),
		client:         client,
		slots:          make(chan struct{}, config.MaxFetches),
		now:            time.Now,
		allowedSources: allowed,
		requests:       map[string]int{},
		misses:         map[string]*missCount{},
		sums:           map[string]knownSum{},
	}, nil
}

// maxKnownSums bounds the remembered source hashes.
const maxKnownSums = 4096

// knownSumLifetime is how long a source's learned SHA-256 refuses other
// hashes for it.
var knownSumLifetime = time.Hour

// knownSum is a source's SHA-256 and when it was learned.
type knownSum struct {
	sum string
	at  time.Time
}

// downloadClient is the client of the origin downloads: https only, at
// most five redirects, every one https, and, unless allow_private_origins,
// no connection to a private, loopback or link-local address (an origin or
// a redirect could otherwise reach the relay's own network).
func downloadClient(config CacheConfig) (*http.Client, error) {
	var proxy func(*http.Request) (*url.URL, error)
	proxyAddress := ""
	if config.UpstreamProxy != "" {
		proxyURL, err := url.Parse(config.UpstreamProxy)
		if err != nil {
			return nil, err
		}
		proxy = http.ProxyURL(proxyURL)
		port := proxyURL.Port()
		if port == "" {
			port = map[string]string{"http": "80", "https": "443"}[proxyURL.Scheme]
		}
		proxyAddress = net.JoinHostPort(proxyURL.Hostname(), port)
	}
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	if !config.AllowPrivateOrigins {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			return refusePrivate(address)
		}
	}
	dial := dialer.DialContext
	transport := &http.Transport{
		Proxy: proxy,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			// The operator's own proxy may sit on the backbone's private
			// network; it is the one private address the relay dials.
			if proxyAddress != "" && address == proxyAddress {
				plain := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
				return plain.DialContext(ctx, network, address)
			}
			return dial(ctx, network, address)
		},
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		MaxIdleConnsPerHost:   2,
		IdleConnTimeout:       90 * time.Second,
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: redirectPolicy(config),
	}, nil
}

// redirectPolicy allows at most five redirects, each https and to an
// origin's host or one of redirect_hosts: an open redirect on an origin
// cannot send the relay to an arbitrary site.
func redirectPolicy(config CacheConfig) func(*http.Request, []*http.Request) error {
	hosts := map[string]bool{}
	for _, origin := range config.Origins {
		if parsed, err := url.Parse(origin); err == nil {
			hosts[strings.ToLower(parsed.Hostname())] = true
		}
	}
	for _, host := range config.RedirectHosts {
		hosts[strings.ToLower(host)] = true
	}
	return func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("more than five redirects")
		}
		if request.URL.Scheme != "https" || request.URL.User != nil {
			return fmt.Errorf("redirect to %s is not https", request.URL.Redacted())
		}
		if !hosts[strings.ToLower(request.URL.Hostname())] {
			return fmt.Errorf("redirect to %s is outside the origins and redirect_hosts", request.URL.Hostname())
		}
		return nil
	}
}

// refusePrivate refuses a dial to an address the relay must not reach for
// a node's request.
func refusePrivate(address string) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("dial address %q is not an IP", host)
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return fmt.Errorf("origin address %s is not public (cache.allow_private_origins is off)", ip)
	}
	for _, reserved := range reservedPrefixes {
		if reserved.Contains(ip) {
			return fmt.Errorf("origin address %s is in reserved %s (cache.allow_private_origins is off)", ip, reserved)
		}
	}
	return nil
}

// reservedPrefixes are the ranges besides private, loopback and link-local
// that are not public origins, or that translate into IPv4 (NAT64, 6to4)
// and could reach a private address that way.
var reservedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

// ServeHTTP serves GET and HEAD of CachePathPrefix<hex> and nothing else,
// to the allowed sources, a bounded number of requests per address.
func (c *Cache) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remote := remoteIP(r.RemoteAddr)
	if !c.sourceAllowed(remote) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if !c.claimRequest(remote) {
		http.Error(w, "too many requests from this address", http.StatusTooManyRequests)
		return
	}
	defer c.releaseRequest(remote)
	c.serveHTTP(w, r)
}

func (c *Cache) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sum, ok := strings.CutPrefix(r.URL.Path, CachePathPrefix)
	if !ok || !sha256Hex.MatchString(sum) {
		http.NotFound(w, r)
		return
	}
	path := c.path(sum)
	if file, err := os.Open(path); err == nil { // #nosec G304 -- path joins the cache directory and a validated SHA-256 hex name.
		c.serve(w, r, file, sum, "hit")
		return
	}
	source := r.URL.Query().Get(CacheSourceParam)
	if source == "" {
		http.NotFound(w, r)
		return
	}
	// Everything below works on the canonical source: a query, a fragment
	// or another spelling of the host would otherwise make a new entry in
	// the known hashes and the download dedupe for the same file.
	canonical, ok := c.canonicalSource(source)
	if !ok {
		c.logger.Warn("refused a source outside the origins", "sha256", sum, "src", redact(source), "remote", r.RemoteAddr)
		http.Error(w, "source is not an allowed origin", http.StatusForbidden)
		return
	}
	source = canonical
	if known, ok := c.knownSum(source); ok && known != sum {
		http.Error(w, "the source does not have this SHA-256", http.StatusNotFound)
		return
	}
	// A miss may start a download: bounded per address, so one host of
	// the region cannot keep the download slots busy (re-audit M1).
	remote := remoteIP(r.RemoteAddr)
	if !c.claimMiss(remote, source) {
		http.Error(w, "too many downloads from this address", http.StatusTooManyRequests)
		return
	}
	defer c.releaseMiss(remote, source)
	// Keyed by the source alone: requests for it with any hashes share one
	// download, which answers the source's real SHA-256.
	got, err, _ := c.fetches.Do(source, func() (any, error) {
		return c.fetch(r.Context(), source)
	})
	if err == nil && got.(string) != sum {
		err = fmt.Errorf("%w: got %s", errHashMismatch, got)
	}
	if err != nil {
		c.logger.Warn("download failed", "sha256", sum, "src", redact(source), "error", err)
		status := http.StatusBadGateway
		switch {
		case errors.Is(err, errOverLimit):
			status = http.StatusRequestEntityTooLarge
		case errors.Is(err, context.Canceled):
			status = http.StatusServiceUnavailable
		}
		http.Error(w, "the file could not be downloaded", status)
		return
	}
	file, err := os.Open(path) // #nosec G304 -- path joins the cache directory and a validated SHA-256 hex name.
	if err != nil {
		// Evicted between the download and now: only when the cache is far
		// too small for the files in flight.
		http.Error(w, "the file could not be kept", http.StatusServiceUnavailable)
		return
	}
	c.serve(w, r, file, sum, "miss")
}

func (c *Cache) serve(w http.ResponseWriter, r *http.Request, file *os.File, sum, result string) {
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		http.Error(w, "the file could not be read", http.StatusInternalServerError)
		return
	}
	// The access time of a hit decides the eviction order.
	now := c.now()
	_ = os.Chtimes(file.Name(), now, now)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"sha256:`+sum+`"`)
	c.logger.Info("served", "sha256", sum, "result", result, "bytes", info.Size(), "remote", r.RemoteAddr)
	http.ServeContent(w, r, "", time.Time{}, file)
}

func (c *Cache) path(sum string) string {
	return filepath.Join(c.config.Dir, "sha256", sum)
}

// canonicalSource answers source in its canonical form, https://<lower
// case host><escaped path>, and whether it starts with one of the origins.
// A source with a user, a query or a fragment is refused: release
// downloads have none, and each would be another key for the same file.
func (c *Cache) canonicalSource(source string) (string, bool) {
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host == "" || parsed.Opaque != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.ContainsAny(source, "?#") {
		return "", false
	}
	// Compare the parsed form, so dot segments or a changed host spelling
	// cannot step outside a prefix.
	if parsed.Path != "" && parsed.Path != cleanURLPath(parsed.Path) {
		return "", false
	}
	// Escape from the decoded path, so another percent-encoding of the same
	// path is the same source.
	canonical := "https://" + strings.ToLower(parsed.Host) + (&url.URL{Path: parsed.Path}).EscapedPath()
	for _, origin := range c.config.Origins {
		prefix, err := url.Parse(origin)
		if err != nil {
			continue
		}
		want := prefix.Scheme + "://" + strings.ToLower(prefix.Host) + prefix.EscapedPath()
		if strings.HasPrefix(canonical, want) {
			return canonical, true
		}
	}
	return "", false
}

// cleanURLPath is path cleaning that keeps a trailing slash.
func cleanURLPath(path string) string {
	cleaned := filepath.ToSlash(filepath.Clean("/" + path))
	if strings.HasSuffix(path, "/") && !strings.HasSuffix(cleaned, "/") {
		cleaned += "/"
	}
	return cleaned
}

// fetch downloads source and keeps it under its SHA-256, which it
// answers; every request for the source compares that with its own.
func (c *Cache) fetch(ctx context.Context, source string) (string, error) {
	// The download outlives the first asker: others may be waiting on it.
	ctx = context.WithoutCancel(ctx)
	select {
	case c.slots <- struct{}{}:
	case <-time.After(time.Minute):
		return "", fmt.Errorf("%w: %d downloads already running", context.Canceled, cap(c.slots))
	}
	defer func() { <-c.slots }()
	ctx, cancel := context.WithTimeout(ctx, c.config.FetchTimeout)
	defer cancel()

	// A download of the source may have finished while this one waited
	// for a slot (re-audit M1).
	if known, ok := c.knownSum(source); ok {
		if _, err := os.Stat(c.path(known)); err == nil {
			return known, nil
		}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil) // #nosec G704 -- ServeHTTP admits only sources under the configured https origins (allowed), and downloadClient refuses non-https redirects and private addresses.
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "anix-relay")
	response, err := c.client.Do(request) // #nosec G704 -- source is under a configured origin; downloadClient refuses non-https redirects and private addresses.
	if err != nil {
		return "", err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("origin answered %s", response.Status)
	}
	if response.ContentLength > c.config.MaxObjectBytes {
		return "", fmt.Errorf("%w: %d bytes", errOverLimit, response.ContentLength)
	}
	temp, err := c.tempFile()
	if err != nil {
		return "", err
	}
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(temp.Name())
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(temp, hash), io.LimitReader(response.Body, c.config.MaxObjectBytes+1))
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if written > c.config.MaxObjectBytes {
		return "", fmt.Errorf("%w: over %d bytes", errOverLimit, c.config.MaxObjectBytes)
	}
	sum := hex.EncodeToString(hash.Sum(nil))
	c.rememberSum(source, sum)
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := os.Stat(c.path(sum)); err == nil {
		return sum, nil
	}
	if err := c.evict(written); err != nil {
		return "", err
	}
	if err := os.Rename(temp.Name(), c.path(sum)); err != nil {
		return "", err
	}
	keep = true
	stored := c.now()
	_ = os.Chtimes(c.path(sum), stored, stored)
	c.logger.Info("cached", "sha256", sum, "bytes", written, "src", redact(source))
	return sum, nil
}

// knownSum answers the SHA-256 source was last found to have, for
// knownSumLifetime: after that the source may have been replaced (a
// re-published release) and is downloaded again.
func (c *Cache) knownSum(source string) (string, bool) {
	c.sumsMu.Lock()
	defer c.sumsMu.Unlock()
	known, ok := c.sums[source]
	if !ok || c.now().Sub(known.at) >= knownSumLifetime {
		delete(c.sums, source)
		return "", false
	}
	return known.sum, true
}

func (c *Cache) rememberSum(source, sum string) {
	c.sumsMu.Lock()
	defer c.sumsMu.Unlock()
	if _, ok := c.sums[source]; !ok && len(c.sums) >= maxKnownSums {
		// Forget everything rather than track an order: the price is one
		// more download per source.
		c.sums = map[string]knownSum{}
	}
	c.sums[source] = knownSum{sum: sum, at: c.now()}
}

func remoteIP(remote string) netip.Addr {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap()
}

func (c *Cache) sourceAllowed(ip netip.Addr) bool {
	if len(c.allowedSources) == 0 {
		return true
	}
	for _, prefix := range c.allowedSources {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// sourceGroup is the key a source's limits count against: the IPv4
// address, or the IPv6 /64, since one host usually holds a whole /64 and
// could otherwise take a new address for every request (re-audit N6).
func sourceGroup(ip netip.Addr) string {
	ip = ip.Unmap()
	if ip.Is4() {
		return ip.String()
	}
	prefix, err := ip.WithZone("").Prefix(64)
	if err != nil {
		return ip.String()
	}
	return prefix.String()
}

// missWindow is the period max_misses_per_source counts over.
const missWindow = time.Hour

// maxMissAddresses is when claimMiss drops the finished windows.
const maxMissAddresses = 4096

// missCount is an address's downloads: the sources it waits on now (with
// how many of its requests wait on each), and how many it started in the
// window.
type missCount struct {
	open    map[string]int
	started int
	since   time.Time
}

// claimMiss admits ip's miss of source: at most max_misses_per_source
// sources started in a missWindow, and never as many sources at once as
// there are download slots, so other addresses always get one. More
// requests of ip for a source it already waits on share that download.
func (c *Cache) claimMiss(ip netip.Addr, source string) bool {
	key := sourceGroup(ip)
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	now := c.now()
	if len(c.misses) >= maxMissAddresses {
		for other, count := range c.misses {
			if len(count.open) == 0 && now.Sub(count.since) >= missWindow {
				delete(c.misses, other)
			}
		}
	}
	m := c.misses[key]
	if m == nil {
		m = &missCount{open: map[string]int{}, since: now}
		c.misses[key] = m
	}
	if now.Sub(m.since) >= missWindow {
		m.started, m.since = 0, now
	}
	if m.open[source] > 0 {
		m.open[source]++
		return true
	}
	openLimit := cap(c.slots) - 1
	if openLimit < 1 {
		openLimit = 1
	}
	if m.started >= c.config.MaxMissesPerSource || len(m.open) >= openLimit {
		return false
	}
	m.started++
	m.open[source] = 1
	return true
}

func (c *Cache) releaseMiss(ip netip.Addr, source string) {
	key := sourceGroup(ip)
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	m := c.misses[key]
	if m == nil {
		return
	}
	if m.open[source] <= 1 {
		delete(m.open, source)
	} else {
		m.open[source]--
	}
	if len(m.open) == 0 && c.now().Sub(m.since) >= missWindow {
		delete(c.misses, key)
	}
}

func (c *Cache) claimRequest(ip netip.Addr) bool {
	key := sourceGroup(ip)
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	if c.requests[key] >= c.config.MaxRequestsPerSource {
		return false
	}
	c.requests[key]++
	return true
}

func (c *Cache) releaseRequest(ip netip.Addr) {
	key := sourceGroup(ip)
	c.requestsMu.Lock()
	defer c.requestsMu.Unlock()
	if c.requests[key] <= 1 {
		delete(c.requests, key)
		return
	}
	c.requests[key]--
}

func (c *Cache) tempFile() (*os.File, error) {
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(c.config.Dir, "tmp", hex.EncodeToString(suffix[:])), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600) // #nosec G304 -- a random hex name in the cache's own tmp directory.
}

// evict removes the least recently used files until incoming more bytes
// fit under max_bytes. Called with mu held.
func (c *Cache) evict(incoming int64) error {
	type entry struct {
		path string
		size int64
		used time.Time
	}
	var entries []entry
	var total int64
	err := filepath.WalkDir(filepath.Join(c.config.Dir, "sha256"), func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		entries = append(entries, entry{path: path, size: info.Size(), used: info.ModTime()})
		total += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	if total+incoming <= c.config.MaxBytes {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].used.Before(entries[j].used) })
	for _, e := range entries {
		if total+incoming <= c.config.MaxBytes {
			break
		}
		if err := os.Remove(e.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		total -= e.size
		c.logger.Info("evicted", "sha256", filepath.Base(e.path), "bytes", e.size)
	}
	return nil
}

// redact drops the query of a source URL from the logs: release URLs carry
// none, but a signed URL would carry a token.
func redact(source string) string {
	parsed, err := url.Parse(source)
	if err != nil {
		return "(unparsable)"
	}
	parsed.RawQuery = ""
	parsed.User = nil
	return parsed.String()
}
