package service

import (
	"encoding/json"
	"net"
	"net/url"
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/config"
)

const SystemConfigKeySubscribeDomains = "app.subscribe_domains"

// SystemConfigKeySubscribeDomainsOnly, when on, has the subscription paths
// answer only on the configured subscription domains: a request for a
// subscription on any other host (the panel's own domain, say) is a 404.
// It keeps the panel domain out of what clients fetch from the mainland, so
// a blocked subscription domain never takes the panel with it. It has no
// effect while no subscription domain is configured.
const SystemConfigKeySubscribeDomainsOnly = "app.subscribe_domains_only"

type SubscriptionSettings struct {
	SubscribePath    string   `json:"subscribe_path"`
	SubscribeDomains []string `json:"subscribe_domains"`
	// DomainsOnly is app.subscribe_domains_only, in force only while
	// SubscribeDomains is not empty.
	DomainsOnly bool `json:"subscribe_domains_only"`
}

func normalizeSubscriptionHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	if parsed, err := url.Parse(value); err == nil && parsed.Host != "" {
		value = parsed.Host
	}

	value = strings.TrimSpace(strings.Split(value, "/")[0])
	value = strings.Trim(strings.Split(value, "?")[0], "[]")
	value = strings.TrimSpace(strings.Split(value, "#")[0])
	value = strings.ToLower(value)
	if value == "" {
		return ""
	}

	if host, port, err := net.SplitHostPort(value); err == nil {
		host = strings.Trim(host, "[]")
		if host == "" {
			return ""
		}
		return net.JoinHostPort(host, port)
	}

	return value
}

func parseSubscriptionDomains(raw string) []string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}

	candidates := make([]string, 0)

	if strings.HasPrefix(value, "[") {
		var array []string
		if err := json.Unmarshal([]byte(value), &array); err == nil {
			candidates = append(candidates, array...)
		}
	}

	if len(candidates) == 0 {
		replacer := strings.NewReplacer("\r\n", "\n", "\r", "\n", ",", "\n", ";", "\n", "\t", "\n", " ", "\n")
		candidates = strings.Split(replacer.Replace(value), "\n")
	}

	domains := make([]string, 0, len(candidates))
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		host := normalizeSubscriptionHost(candidate)
		if host == "" {
			continue
		}
		if _, ok := seen[host]; ok {
			continue
		}
		seen[host] = struct{}{}
		domains = append(domains, host)
	}

	return domains
}

func currentSubscribePath(cfg *config.Config) string {
	if cfg != nil && strings.TrimSpace(cfg.App.SubscribePath) != "" {
		return "/" + strings.Trim(strings.TrimSpace(cfg.App.SubscribePath), "/")
	}
	return "/s"
}

func GetSubscriptionSettings(configService *SystemConfigService, cfg *config.Config) SubscriptionSettings {
	settings := SubscriptionSettings{
		SubscribePath: currentSubscribePath(cfg),
	}

	if configService == nil {
		return settings
	}

	raw, err := configService.Get(SystemConfigKeySubscribeDomains)
	if err != nil {
		return settings
	}

	settings.SubscribeDomains = parseSubscriptionDomains(raw)
	if len(settings.SubscribeDomains) > 0 {
		if only, err := configService.Get(SystemConfigKeySubscribeDomainsOnly); err == nil {
			settings.DomainsOnly = parseConfigSwitch(only)
		}
	}
	return settings
}

// parseConfigSwitch reads a switch stored as text ("true", "1", "on").
func parseConfigSwitch(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "on", "yes":
		return true
	}
	return false
}
