// Package requestorigin resolves the scheme, host and client IP of an HTTP
// request. Forwarding headers (X-Forwarded-Proto, X-Forwarded-Host,
// X-Forwarded-For, X-Real-IP) are honoured only when the TCP peer is one of
// the configured reverse proxies (server.trusted_proxies); from any other
// peer they are ignored and the connection decides: https when it is TLS,
// the Host header, and the peer address.
//
// Every value the kernel builds a URL from (install scripts, subscription
// links, webhook URLs, the request_scheme and request_host sent to package
// hosts) must come from here, so a client cannot make the server hand out
// links to a host of its choosing.
package requestorigin

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// DefaultTrustedProxies is the trust list when server.trusted_proxies is not
// set: loopback only, so a reverse proxy on the same host (and Control's own
// UI server, which proxies /api to the API listener) keeps working.
var DefaultTrustedProxies = []string{"127.0.0.1/32", "::1/128"}

// Header names Control reads from a trusted proxy.
const (
	HeaderForwardedProto = "X-Forwarded-Proto"
	HeaderForwardedHost  = "X-Forwarded-Host"
	HeaderForwardedFor   = "X-Forwarded-For"
	HeaderRealIP         = "X-Real-IP"
)

// remoteIPHeaders mirrors gin's RemoteIPHeaders (set by ApplyTo), so
// ClientIP and gin's Context.ClientIP agree.
var remoteIPHeaders = []string{HeaderForwardedFor, HeaderRealIP}

// IsForwardingHeader reports whether name is a header a proxy uses to
// describe the original request (X-Forwarded-*, Forwarded, X-Real-IP). The
// kernel drops them from requests it hands on, after resolving the origin.
func IsForwardingHeader(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "forwarded" || name == "x-real-ip" || strings.HasPrefix(name, "x-forwarded-")
}

// Policy is a set of trusted proxy networks.
type Policy struct {
	networks []*net.IPNet
	cidrs    []string
}

// NewPolicy parses proxies (IP addresses or CIDRs). A nil list is the
// default (DefaultTrustedProxies); an empty, non-nil list trusts no proxy.
func NewPolicy(proxies []string) (*Policy, error) {
	if proxies == nil {
		proxies = DefaultTrustedProxies
	}
	policy := &Policy{}
	for _, raw := range proxies {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if !strings.Contains(value, "/") {
			ip := net.ParseIP(value)
			if ip == nil {
				return nil, fmt.Errorf("invalid trusted proxy %q: use an IP address or a CIDR", raw)
			}
			if ip.To4() != nil {
				value += "/32"
			} else {
				value += "/128"
			}
		}
		_, network, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy %q: use an IP address or a CIDR", raw)
		}
		policy.networks = append(policy.networks, network)
		policy.cidrs = append(policy.cidrs, network.String())
	}
	return policy, nil
}

// CIDRs returns the trusted networks in CIDR form.
func (p *Policy) CIDRs() []string {
	if p == nil {
		return nil
	}
	return append([]string(nil), p.cidrs...)
}

// ApplyTo configures a gin engine with the same trust list, so
// Context.ClientIP (rate limits, login throttling, audit logs) honours
// X-Forwarded-For and X-Real-IP only from these proxies.
func (p *Policy) ApplyTo(engine *gin.Engine) error {
	engine.ForwardedByClientIP = true
	engine.RemoteIPHeaders = append([]string(nil), remoteIPHeaders...)
	engine.TrustedPlatform = ""
	cidrs := p.CIDRs()
	if len(cidrs) == 0 {
		return engine.SetTrustedProxies(nil)
	}
	return engine.SetTrustedProxies(cidrs)
}

// TrustedIP reports whether ip is in a trusted network.
func (p *Policy) TrustedIP(ip net.IP) bool {
	if p == nil || ip == nil {
		return false
	}
	for _, network := range p.networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// TrustedPeer reports whether the request's TCP peer is a trusted proxy.
func (p *Policy) TrustedPeer(r *http.Request) bool {
	return r != nil && p.TrustedIP(peerIP(r))
}

var current atomic.Pointer[Policy]

func init() {
	policy, err := NewPolicy(nil)
	if err != nil {
		panic(err)
	}
	current.Store(policy)
}

// SetDefault makes policy the process-wide policy Resolve uses. The server
// sets it once at startup from server.trusted_proxies.
func SetDefault(policy *Policy) {
	if policy != nil {
		current.Store(policy)
	}
}

// Default returns the process-wide policy.
func Default() *Policy { return current.Load() }

// Origin is where a request was addressed and who sent it.
type Origin struct {
	// Scheme is "http" or "https".
	Scheme string
	// Host is a validated host[:port], lower-cased; empty when the request
	// carried no valid host.
	Host string
	// ClientIP is the client address (empty when none could be parsed).
	ClientIP string
	// Forwarded is true when a trusted proxy's headers set Scheme or Host.
	Forwarded bool
}

// BaseURL is scheme://host, or "" when the host is unknown.
func (o Origin) BaseURL() string {
	if o.Host == "" {
		return ""
	}
	return o.Scheme + "://" + o.Host
}

// Resolve resolves r with the process-wide policy.
func Resolve(r *http.Request) Origin { return Default().Resolve(r) }

// Resolve returns r's origin. Forwarding headers are read only when the TCP
// peer is trusted; the last value of a header (the one the nearest proxy
// wrote) is used, and a value that is not a plain http/https scheme or a
// valid host[:port] is ignored.
func (p *Policy) Resolve(r *http.Request) Origin {
	origin := Origin{Scheme: "http"}
	if r == nil {
		return origin
	}
	if r.TLS != nil {
		origin.Scheme = "https"
	}
	if host, ok := NormalizeHost(r.Host); ok {
		origin.Host = host
	}
	origin.ClientIP = p.ClientIP(r)
	if !p.TrustedPeer(r) {
		return origin
	}
	if proto := strings.ToLower(lastHeaderValue(r.Header, HeaderForwardedProto)); proto == "http" || proto == "https" {
		origin.Scheme = proto
		origin.Forwarded = true
	}
	if host, ok := NormalizeHost(lastHeaderValue(r.Header, HeaderForwardedHost)); ok {
		origin.Host = host
		origin.Forwarded = true
	}
	return origin
}

// ClientIP returns the client address the way gin's Context.ClientIP does
// on an engine configured with ApplyTo: the peer address, unless the peer
// is trusted, in which case X-Forwarded-For is walked from the right past
// trusted proxies (then X-Real-IP).
func (p *Policy) ClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	peer := peerIP(r)
	if peer == nil {
		return ""
	}
	if !p.TrustedIP(peer) {
		return peer.String()
	}
	for _, name := range remoteIPHeaders {
		if ip, ok := p.headerClientIP(r.Header.Get(name)); ok {
			return ip
		}
	}
	return peer.String()
}

func (p *Policy) headerClientIP(header string) (string, bool) {
	if header == "" {
		return "", false
	}
	items := strings.Split(header, ",")
	for i := len(items) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(items[i]))
		if ip == nil {
			break
		}
		if i == 0 || !p.TrustedIP(ip) {
			return ip.String(), true
		}
	}
	return "", false
}

func peerIP(r *http.Request) net.IP {
	// As gin's Context.RemoteIP: an address without a port is no peer.
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return nil
	}
	return net.ParseIP(host)
}

func lastHeaderValue(header http.Header, name string) string {
	values := header.Values(name)
	for i := len(values) - 1; i >= 0; i-- {
		items := strings.Split(values[i], ",")
		for j := len(items) - 1; j >= 0; j-- {
			if item := strings.TrimSpace(items[j]); item != "" {
				return item
			}
		}
	}
	return ""
}

// NormalizeHost validates a host[:port] authority and lower-cases it. It
// rejects anything else: a scheme, a path, user info, spaces or control
// characters, an empty or out-of-range port, an unbracketed IPv6 literal,
// and names that are not DNS-like labels or IP addresses.
func NormalizeHost(value string) (string, bool) {
	if value == "" || len(value) > 255 {
		return "", false
	}
	for _, character := range value {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("-._:[]", character) {
			continue
		}
		return "", false
	}
	value = strings.ToLower(value)
	host, port := value, ""
	if strings.HasPrefix(value, "[") {
		end := strings.Index(value, "]")
		if end < 0 {
			return "", false
		}
		host, port = value[1:end], value[end+1:]
		if port != "" {
			if !strings.HasPrefix(port, ":") {
				return "", false
			}
			port = port[1:]
			if !validPort(port) {
				return "", false
			}
		}
		if ip := net.ParseIP(host); ip == nil || ip.To4() != nil || strings.Contains(host, "%") {
			return "", false
		}
		return value, true
	}
	if strings.Contains(value, ":") {
		var err error
		host, port, err = net.SplitHostPort(value)
		if err != nil || !validPort(port) {
			return "", false
		}
	}
	if !validHostname(host) {
		return "", false
	}
	return value, true
}

func validPort(port string) bool {
	if port == "" || len(port) > 5 {
		return false
	}
	number, err := strconv.Atoi(port)
	return err == nil && number >= 1 && number <= 65535
}

func validHostname(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "[]:") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}
