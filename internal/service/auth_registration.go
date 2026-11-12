package service

import (
	"strings"

	"github.com/AnixOps/anix-control/v5/internal/config"
)

type RegistrationPolicy struct {
	Enabled             bool
	RequireInvite       bool
	AllowedEmailDomains []string
	BlockedEmailDomains []string
}

func ResolveRegistrationPolicy(cfg *config.Config) RegistrationPolicy {
	enabled := true
	policy := RegistrationPolicy{
		Enabled: enabled,
	}
	if cfg == nil {
		return policy
	}

	raw := cfg.Auth.Registration
	if raw.Enabled != nil {
		policy.Enabled = *raw.Enabled
	}
	policy.RequireInvite = raw.RequireInvite
	policy.AllowedEmailDomains = normalizeDomainList(raw.AllowedEmailDomains)
	policy.BlockedEmailDomains = normalizeDomainList(raw.BlockedEmailDomains)
	return policy
}

func normalizeDomainList(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		domain := strings.ToLower(strings.TrimSpace(item))
		domain = strings.TrimPrefix(domain, "@")
		domain = strings.TrimPrefix(domain, ".")
		if domain == "" {
			continue
		}
		if _, ok := seen[domain]; ok {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out
}
