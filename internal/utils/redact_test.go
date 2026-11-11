package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// ========== JWT Tests ==========

func TestGenerateToken(t *testing.T) {
	token, err := GenerateToken(1, "test@example.com", false, "test-secret", 3600)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateToken_Admin(t *testing.T) {
	token, err := GenerateToken(2, "admin@example.com", true, "admin-secret", 7200)
	require.NoError(t, err)
	assert.NotEmpty(t, token)
}

func TestParseTokenWithSecret(t *testing.T) {
	// Generate a token first
	token, err := GenerateToken(1, "test@example.com", false, "test-secret", 3600)
	require.NoError(t, err)

	// Parse it
	claims, err := ParseTokenWithSecret(token, "test-secret")
	require.NoError(t, err)
	assert.Equal(t, uint(1), claims.UserID)
	assert.Equal(t, "test@example.com", claims.Email)
	assert.False(t, claims.IsAdmin)
}

func TestParseTokenWithSecret_InvalidToken(t *testing.T) {
	_, err := ParseTokenWithSecret("invalid-token", "test-secret")
	assert.Error(t, err)
}

func TestParseTokenWithSecret_WrongSecret(t *testing.T) {
	token, err := GenerateToken(1, "test@example.com", false, "correct-secret", 3600)
	require.NoError(t, err)

	_, err = ParseTokenWithSecret(token, "wrong-secret")
	assert.Error(t, err)
}

func TestParseTokenWithSecret_Expired(t *testing.T) {
	// Generate an already expired token
	token, err := GenerateToken(1, "test@example.com", false, "test-secret", -1)
	require.NoError(t, err)

	_, err = ParseTokenWithSecret(token, "test-secret")
	assert.Error(t, err)
}
