package config

import (
	"strings"
	"testing"
)

func TestSecretStrengthWarningsBoundary(t *testing.T) {
	for _, tc := range []struct {
		name   string
		secret string
		warn   bool
	}{
		{"31 random bytes", "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0x", true},
		{"32 random bytes", "aB3dE6gH9jK2mN5pQ8sT1vW4yZ7cF0xR", false},
		{"64 hex", strings.Repeat("0123456789abcdef", 4), false},
		{"empty is not reported", "", false},
		{"blank is not reported", "   ", false},
		{"one repeated byte", strings.Repeat("a", 40), true},
		{"four distinct bytes", strings.Repeat("abcd", 10), true},
		{"template value is long enough but still a template", "CHANGE-THIS-TO-A-VERY-LONG-RANDOM-STRING-IN-PRODUCTION", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &Config{}
			c.JWT.Secret = tc.secret
			got := c.SecretStrengthWarnings()
			if (len(got) == 1) != tc.warn || len(got) > 1 {
				t.Fatalf("secret %q: warnings %v, want warn=%v", tc.secret, got, tc.warn)
			}
		})
	}
}

func TestSecretStrengthWarningsNameTheSettingAndNeverTheValue(t *testing.T) {
	c := &Config{}
	c.JWT.Secret = "hunter2hunter2"
	c.App.APIToken = "short-token"
	c.GRPC.APIToken = strings.Repeat("x", 64)
	warnings := c.SecretStrengthWarnings()
	if len(warnings) != 3 {
		t.Fatalf("warnings %v, want one per weak secret", warnings)
	}
	for i, setting := range []string{"jwt.secret", "app.api_token", "grpc.api_token"} {
		if !strings.HasPrefix(warnings[i], setting+" is weak") {
			t.Errorf("warning %d does not start with %q: %s", i, setting, warnings[i])
		}
		if !strings.Contains(warnings[i], "openssl rand -hex 32") || !strings.Contains(warnings[i], EnvPrefix) {
			t.Errorf("warning %d does not say how to rotate: %s", i, warnings[i])
		}
	}
	for _, w := range warnings {
		for _, secret := range []string{"hunter2hunter2", "short-token", strings.Repeat("x", 64)} {
			if strings.Contains(w, secret) {
				t.Fatalf("a warning contains the secret value: %s", w)
			}
		}
	}
}

func TestSecretStrengthWarningsOfANilConfig(t *testing.T) {
	var nilConfig *Config
	if nilConfig.SecretStrengthWarnings() != nil {
		t.Error("a nil config reports warnings")
	}
}

func TestSecretStrengthDoesNotChangeValidation(t *testing.T) {
	c := &Config{Env: "production"}
	c.JWT.Secret = "short"
	if err := c.ValidateForServer(); err != nil {
		t.Fatalf("a short secret must still start: %v", err)
	}
}
