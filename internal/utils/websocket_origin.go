package utils

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/AnixOps/anix-control/v3/internal/config"
)

// CheckWebSocketOrigin applies the server CORS origin allowlist to WebSocket
// upgrades while still allowing same-host browser upgrades and non-browser
// clients that do not send an Origin header.
func CheckWebSocketOrigin(r *http.Request) bool {
	var allowedOrigins []string
	if cfg := config.Get(); cfg != nil {
		allowedOrigins = cfg.Server.CORS.AllowedOrigins
	}
	return IsWebSocketOriginAllowed(r, allowedOrigins)
}

// IsWebSocketOriginAllowed validates a WebSocket Origin header against the
// request host and an explicit origin allowlist.
func IsWebSocketOriginAllowed(r *http.Request, allowedOrigins []string) bool {
	if r == nil {
		return false
	}

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}

	canonicalOrigin, originHost, ok := canonicalWebSocketOrigin(origin)
	if !ok {
		return false
	}

	requestHost := normalizeWebSocketHost(r.Host)
	if requestHost != "" && originHost == requestHost {
		return true
	}

	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed == "" {
			continue
		}
		if allowed == "*" {
			return true
		}
		canonicalAllowed, _, ok := canonicalWebSocketOrigin(allowed)
		if ok && canonicalAllowed == canonicalOrigin {
			return true
		}
	}

	return false
}

func canonicalWebSocketOrigin(raw string) (string, string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", "", false
	}

	scheme := strings.ToLower(parsed.Scheme)
	switch scheme {
	case "http", "https", "ws", "wss":
	default:
		return "", "", false
	}

	host := normalizeWebSocketHost(parsed.Host)
	if host == "" {
		return "", "", false
	}

	switch scheme {
	case "ws":
		scheme = "http"
	case "wss":
		scheme = "https"
	}
	return scheme + "://" + host, host, true
}

func normalizeWebSocketHost(raw string) string {
	host := strings.TrimSpace(strings.ToLower(raw))
	if host == "" {
		return ""
	}
	host = strings.TrimSuffix(host, ".")

	splitHost, port, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}

	splitHost = strings.TrimSuffix(strings.ToLower(splitHost), ".")
	if port == "80" || port == "443" {
		return splitHost
	}
	return net.JoinHostPort(splitHost, port)
}
