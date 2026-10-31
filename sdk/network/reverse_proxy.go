package network

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	networkv1 "github.com/AnixOps/anix-control/sdk/api/network/v1"
)

// Reverse proxy bounds.
const (
	MaxProxySites       = 256
	MaxProxySiteHosts   = 64
	MaxProxyRoutes      = 64
	MaxProxyHeaders     = 32
	MaxCertificateBytes = 64 * 1024
)

// Headers a route may not set: the proxy writes them itself.
var reservedProxyHeaders = map[string]bool{
	"host": true, "content-length": true, "transfer-encoding": true, "connection": true,
	"keep-alive": true, "proxy-connection": true, "te": true, "trailer": true, "upgrade": true,
}

func checkReverseProxy(proxy *networkv1.ReverseProxy, proxies map[string]*networkv1.ReverseProxy,
	egresses map[string]*networkv1.Egress, transports map[string]*networkv1.Transport, listens map[string]string,
) error {
	if err := checkID(proxy.GetId(), proxies); err != nil {
		return err
	}
	if err := checkReverseProxyFields(proxy); err != nil {
		return err
	}
	listen := proxy.GetListen()
	key := net.JoinHostPort(canonical(listen.GetAddress()), strconv.Itoa(int(listen.GetPort())))
	if other, taken := listens[key]; taken {
		return fmt.Errorf("listen %s is taken by %q", key, other)
	}
	listens[key] = proxy.GetId()
	return checkNext(proxy.GetEgressId(), proxy.GetTransportId(), egresses, transports)
}

// CheckReverseProxy checks what a reverse proxy says on its own: its id,
// listen endpoint, certificate, sites and routes; Check also checks its
// listen address and next hop against the plan.
func CheckReverseProxy(proxy *networkv1.ReverseProxy) error {
	if !idPattern.MatchString(proxy.GetId()) {
		return invalid("id %q must be 1 to 64 ASCII letters, digits, '-' and '_'", proxy.GetId())
	}
	if err := checkReverseProxyFields(proxy); err != nil {
		return invalid("%v", err)
	}
	return nil
}

func checkReverseProxyFields(proxy *networkv1.ReverseProxy) error {
	if err := checkListen(proxy.GetListen()); err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	cert, key := proxy.GetCertificatePem(), proxy.GetPrivateKeyPem()
	if (cert == "") != (key == "") {
		return fmt.Errorf("certificate_pem and private_key_pem are both set or both empty")
	}
	if len(cert) > MaxCertificateBytes || len(key) > MaxCertificateBytes {
		return fmt.Errorf("certificate_pem and private_key_pem are at most %d bytes", MaxCertificateBytes)
	}
	if cert != "" && (!strings.Contains(cert, "-----BEGIN CERTIFICATE-----") || !strings.Contains(key, "PRIVATE KEY-----")) {
		return fmt.Errorf("certificate_pem and private_key_pem must be PEM")
	}
	sites := proxy.GetSites()
	if len(sites) == 0 || len(sites) > MaxProxySites {
		return fmt.Errorf("1 to %d sites are required", MaxProxySites)
	}
	hosts := map[string]bool{}
	for i, site := range sites {
		if err := checkProxySite(site, hosts); err != nil {
			return fmt.Errorf("sites[%d]: %w", i, err)
		}
	}
	return nil
}

func checkProxySite(site *networkv1.ReverseProxySite, hosts map[string]bool) error {
	if len(site.GetHosts()) == 0 || len(site.GetHosts()) > MaxProxySiteHosts {
		return fmt.Errorf("1 to %d hosts are required", MaxProxySiteHosts)
	}
	for _, host := range site.GetHosts() {
		pattern, err := proxyHostPattern(host)
		if err != nil {
			return err
		}
		if hosts[pattern] {
			return fmt.Errorf("host %q is listed twice", host)
		}
		hosts[pattern] = true
	}
	routes := site.GetRoutes()
	if site.GetRedirectHttps() {
		if len(routes) != 0 {
			return fmt.Errorf("a redirect_https site has no routes")
		}
		return nil
	}
	if len(routes) == 0 || len(routes) > MaxProxyRoutes {
		return fmt.Errorf("1 to %d routes are required", MaxProxyRoutes)
	}
	paths := map[string]bool{}
	for j, route := range routes {
		path := route.GetPath()
		if path == "" {
			path = "/"
		}
		if paths[path] {
			return fmt.Errorf("routes[%d]: path %q is listed twice", j, path)
		}
		paths[path] = true
		if err := checkProxyRoute(route); err != nil {
			return fmt.Errorf("routes[%d]: %w", j, err)
		}
	}
	return nil
}

// proxyHostPattern returns the lowercased pattern of an "example.com",
// "*.example.com" or "*" host.
func proxyHostPattern(host string) (string, error) {
	if host == "*" {
		return host, nil
	}
	name := strings.TrimPrefix(host, "*.")
	if !validDNSName(name) {
		return "", fmt.Errorf("host %q is not a name, *.name or *", host)
	}
	return strings.ToLower(host), nil
}

func validDNSName(name string) bool {
	if name == "" || len(name) > 253 {
		return false
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
			if !letter && (c < '0' || c > '9') && c != '-' && c != '_' {
				return false
			}
		}
	}
	return true
}

func checkProxyRoute(route *networkv1.ReverseProxyRoute) error {
	if path := route.GetPath(); path != "" && path != "/" {
		if !strings.HasPrefix(path, "/") || strings.HasSuffix(path, "/") || strings.ContainsAny(path, "?# \t\r\n") || len(path) > MaxTextBytes {
			return fmt.Errorf("path %q must start with / and have no trailing /, query or spaces", path)
		}
	}
	switch route.GetHostHeader() {
	case "", "client", "upstream":
	default:
		return fmt.Errorf("host_header %q must be client or upstream", route.GetHostHeader())
	}
	if host := route.GetHost(); host != "" && !validProxyHostValue(host) {
		return fmt.Errorf("host %q is not a host[:port]", host)
	}
	if len(route.GetHeaders()) > MaxProxyHeaders {
		return fmt.Errorf("at most %d headers", MaxProxyHeaders)
	}
	for name, value := range route.GetHeaders() {
		if !validHeaderName(name) {
			return fmt.Errorf("header %q is not a header name", name)
		}
		if reservedProxyHeaders[strings.ToLower(name)] {
			return fmt.Errorf("header %q is set by the proxy", name)
		}
		if strings.ContainsAny(value, "\r\n\x00") || len(value) > MaxTextBytes {
			return fmt.Errorf("header %q has a bad value", name)
		}
	}
	upstreams := route.GetUpstreams()
	if len(upstreams) == 0 || len(upstreams) > MaxForwardTargets {
		return fmt.Errorf("1 to %d upstreams are required", MaxForwardTargets)
	}
	for k, upstream := range upstreams {
		if err := checkHostPort(net.JoinHostPort(upstream.GetHost(), strconv.Itoa(int(upstream.GetPort())))); err != nil {
			return fmt.Errorf("upstreams[%d]: %v", k, err)
		}
	}
	policy := route.GetPolicy()
	if p := policy.GetProxyProtocol(); (p != "" && p != "off") || policy.GetBandwidthBps() != 0 || policy.GetMaxConns() != 0 {
		return fmt.Errorf("policy: proxy_protocol, bandwidth_bps and max_conns are not used by a reverse proxy")
	}
	// The same balancing, health and breaker bounds as a TCP port forward.
	asForward := &networkv1.PortForward{Protocols: []string{"tcp"}, Targets: upstreams, Policy: policy}
	if err := checkForwardPolicy(asForward); err != nil {
		return fmt.Errorf("policy: %w", err)
	}
	return nil
}

func validProxyHostValue(host string) bool {
	name := host
	if h, port, err := net.SplitHostPort(host); err == nil {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > MaxPort {
			return false
		}
		name = h
	}
	if strings.HasPrefix(name, "[") && strings.HasSuffix(name, "]") {
		name = name[1 : len(name)-1]
	}
	return net.ParseIP(name) != nil || validDNSName(name)
}

// validHeaderName reports whether name is an RFC 9110 token.
func validHeaderName(name string) bool {
	if name == "" || len(name) > 256 {
		return false
	}
	for _, c := range name {
		if c > 0x7e || c <= 0x20 || strings.ContainsRune("\"(),/:;<=>?@[\\]{}", c) {
			return false
		}
	}
	return true
}

func checkReverseProxyStatus(proxies []*networkv1.ReverseProxyStatus) error {
	if len(proxies) > MaxComponents {
		return invalid("more than %d reverse proxies", MaxComponents)
	}
	for i, proxy := range proxies {
		if !idPattern.MatchString(proxy.GetId()) {
			return invalid("reverse_proxies[%d]: bad id", i)
		}
		if networkv1.ComponentState_name[int32(proxy.GetState())] == "" {
			return invalid("reverse_proxies[%d]: unknown state", i)
		}
		if err := checkText(proxy.GetDetail()); err != nil {
			return invalid("reverse_proxies[%d]: %v", i, err)
		}
		if len(proxy.GetUpstreams()) > MaxProxyRoutes*MaxForwardTargets {
			return invalid("reverse_proxies[%d]: too many upstreams", i)
		}
		for j, target := range proxy.GetUpstreams() {
			if err := checkTargetHealth(target); err != nil {
				return invalid("reverse_proxies[%d]: upstreams[%d]: %v", i, j, err)
			}
		}
	}
	return nil
}

func checkTargetHealth(target *networkv1.ForwardTargetHealth) error {
	if target.GetHost() == "" || target.GetPort() == 0 || target.GetPort() > MaxPort {
		return fmt.Errorf("host and a port from 1 to %d are required", MaxPort)
	}
	if err := checkText(target.GetHost()); err != nil {
		return err
	}
	if networkv1.TargetHealthState_name[int32(target.GetState())] == "" {
		return fmt.Errorf("unknown state")
	}
	if target.GetCheckedAtUnixMs() < 0 || target.GetCircuitOpenUntilUnixMs() < 0 {
		return fmt.Errorf("negative time")
	}
	return nil
}
