package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/AnixOps/anix-control/v4/internal/config"
)

func TestLogSecretStrengthWarnsOncePerWeakSecretAndNeverEchoesIt(t *testing.T) {
	cfg := &config.Config{}
	cfg.JWT.Secret = "tooshortsecret"
	cfg.GRPC.APIToken = strings.Repeat("k", 48)
	var lines []string
	logSecretStrength(func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }, cfg)
	if len(lines) != 2 {
		t.Fatalf("lines %v, want one per weak secret", lines)
	}
	for _, line := range lines {
		if !strings.HasPrefix(line, "WARNING: ") || strings.Contains(line, "tooshortsecret") || strings.Contains(line, strings.Repeat("k", 48)) {
			t.Errorf("unexpected line: %s", line)
		}
	}
}

func TestLogSecretStrengthIsQuietForAStrongConfig(t *testing.T) {
	cfg := &config.Config{}
	cfg.JWT.Secret = strings.Repeat("0123456789abcdef", 4)
	logSecretStrength(func(format string, args ...any) { t.Errorf("unexpected log: "+format, args...) }, cfg)
}
