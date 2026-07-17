package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/AnixOps/anix-control/v4/internal/config"
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

func ValidateRegistrationEmail(email string, policy RegistrationPolicy) error {
	domain, err := emailDomain(email)
	if err != nil {
		return err
	}

	if len(policy.BlockedEmailDomains) > 0 && domainMatchesAny(domain, policy.BlockedEmailDomains) {
		return fmt.Errorf("email domain %s is not allowed", domain)
	}
	if len(policy.AllowedEmailDomains) > 0 && !domainMatchesAny(domain, policy.AllowedEmailDomains) {
		return fmt.Errorf("email domain %s is not in the allowlist", domain)
	}
	return nil
}

func emailDomain(email string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(email))
	parts := strings.Split(normalized, "@")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("invalid email address")
	}
	return strings.TrimPrefix(parts[1], "."), nil
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

func domainMatchesAny(domain string, patterns []string) bool {
	for _, pattern := range patterns {
		if domain == pattern || strings.HasSuffix(domain, "."+pattern) {
			return true
		}
	}
	return false
}
