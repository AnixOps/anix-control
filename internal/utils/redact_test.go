package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedact(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"abc", "****"},
		{"12345678", "****"},
		{"123456789", "1234****6789"},
		{"a1b2c3d4e5f6g7h8", "a1b2****g7h8"},
	}

	for _, tt := range tests {
		result := Redact(tt.input)
		if result != tt.expected {
			t.Errorf("Redact(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestRedactIP(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"192.168.1.100", "192.168.*.*"},
		{"10.0.0.1", "10.0.*.*"},
		{"2001:db8:85a3::8a2e:370:7334", "2001:db8:****"},
	}

	for _, tt := range tests {
		result := RedactIP(tt.input)
		if result != tt.expected {
			t.Errorf("RedactIP(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestLogSafe(t *testing.T) {
	log := NewLogSafe().
		SetRaw("action", "login").
		SetIP("ip", "192.168.1.100")

	fields := log.fields

	if fields["action"] != "login" {
		t.Errorf("action should be 'login'")
	}
	if fields["ip"] != "192.168.*.*" {
		t.Errorf("ip should be redacted")
	}
}

func TestLogSafe_String(t *testing.T) {
	log := NewLogSafe().
		SetRaw("username", "john").
		SetIP("ip", "192.168.1.100")

	result := log.String()
	assert.Contains(t, result, "john")
	assert.Contains(t, result, "192.168.*.*")
}
