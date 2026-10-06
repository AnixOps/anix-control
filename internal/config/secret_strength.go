package config

import (
	"fmt"
	"strings"
)

// MinSharedSecretBytes is the length below which a shared secret (jwt.secret,
// app.api_token, grpc.api_token) is reported as weak at startup. A shorter one
// still works: refusing to start would break every installation that has one.
const MinSharedSecretBytes = 32

// SecretStrengthWarnings reports the shared secrets that are set but weak:
// shorter than MinSharedSecretBytes, a template value, or one repeated byte or
// a handful of distinct bytes. An unset secret is not reported (jwt
// production requirements are in ValidateForServer, and an empty api token
// disables its feature). Each finding names the setting and how to rotate it
// and never contains the secret value.
func (c *Config) SecretStrengthWarnings() []string {
	if c == nil {
		return nil
	}
	secrets := []struct{ setting, env, value string }{
		{"jwt.secret", EnvPrefix + "JWT_SECRET", c.JWT.Secret},
		{"app.api_token", EnvPrefix + "APP_API_TOKEN", c.App.APIToken},
		{"grpc.api_token", EnvPrefix + "GRPC_API_TOKEN", c.GRPC.APIToken},
	}
	var warnings []string
	for _, s := range secrets {
		reason := weakSecretReason(strings.TrimSpace(s.value))
		if reason == "" {
			continue
		}
		warnings = append(warnings, fmt.Sprintf("%s is weak (%s): generate a new one with `openssl rand -hex 32` and set %s (or %s%s) at the next restart; changing jwt.secret signs everyone out",
			s.setting, reason, s.env, s.env, EnvFileSuffix))
	}
	return warnings
}

// weakSecretReason names why a non-empty secret is weak, "" when it is not.
func weakSecretReason(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) < MinSharedSecretBytes {
		return fmt.Sprintf("%d bytes, at least %d expected", len(secret), MinSharedSecretBytes)
	}
	if placeholderJWTSecrets[secret] {
		return "a template value"
	}
	distinct := map[byte]bool{}
	for i := 0; i < len(secret); i++ {
		distinct[secret[i]] = true
	}
	if len(distinct) <= 4 {
		return fmt.Sprintf("only %d distinct characters", len(distinct))
	}
	return ""
}
