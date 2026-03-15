package utils

import (
	"encoding/json"
	"testing"

	"github.com/anixops/v2board/internal/config"
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

func TestRedactEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"a@b.com", "a***@b.com"},
		{"admin@example.com", "ad***@example.com"},
		{"test.user@domain.org", "te***@domain.org"},
		{"invalid-email", "inva****mail"}, // 没有@，使用普通脱敏
	}

	for _, tt := range tests {
		result := RedactEmail(tt.input)
		if result != tt.expected {
			t.Errorf("RedactEmail(%q) = %q, want %q", tt.input, result, tt.expected)
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

func TestSensitiveString(t *testing.T) {
	s := SensitiveString("my-secret-api-key-12345")
	result := s.String()
	expected := "my-s****2345"
	if result != expected {
		t.Errorf("SensitiveString.String() = %q, want %q", result, expected)
	}

	// 确保 Raw() 返回原始值
	if s.Raw() != "my-secret-api-key-12345" {
		t.Errorf("SensitiveString.Raw() should return original value")
	}
}

func TestSensitiveString_MarshalJSON(t *testing.T) {
	s := SensitiveString("abcdefghijklmnop")
	data, err := json.Marshal(s)
	require.NoError(t, err)
	assert.Contains(t, string(data), "abcd****mnop")
}

func TestSensitiveString_Empty(t *testing.T) {
	s := SensitiveString("")
	assert.Equal(t, "", s.String())
	assert.Equal(t, "", s.Raw())
}

func TestRedactMap(t *testing.T) {
	input := map[string]interface{}{
		"username": "john",
		"password": "secret123456789", // 长度超过8才会部分显示
		"api_key":  "abcdefghijklmnop",
		"nested": map[string]interface{}{
			"token": "xyz789",
			"name":  "test",
		},
	}

	result := RedactMap(input)

	if result["username"] != "john" {
		t.Errorf("username should not be redacted")
	}
	if result["password"] != "secr****6789" {
		t.Errorf("password should be redacted, got %v", result["password"])
	}
	if result["api_key"] != "abcd****mnop" {
		t.Errorf("api_key should be redacted, got %v", result["api_key"])
	}

	nested := result["nested"].(map[string]interface{})
	if nested["token"] != "****" {
		t.Errorf("nested token should be redacted, got %v", nested["token"])
	}
	if nested["name"] != "test" {
		t.Errorf("nested name should not be redacted")
	}
}

func TestRedactMap_NonStringSensitive(t *testing.T) {
	input := map[string]interface{}{
		"password": 12345, // non-string value
		"api_key":  true,
	}

	result := RedactMap(input)

	assert.Equal(t, "[REDACTED]", result["password"])
	assert.Equal(t, "[REDACTED]", result["api_key"])
}

func TestRedactMap_Empty(t *testing.T) {
	// RedactMap(nil) returns empty map (not nil) because function uses make()
	result := RedactMap(nil)
	assert.NotNil(t, result)
	assert.Empty(t, result)

	// Empty map returns empty map
	result = RedactMap(map[string]interface{}{})
	assert.NotNil(t, result)
	assert.Empty(t, result)
}

func TestRedactJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "valid json with password",
			input:    `{"username":"john","password":"secret123456789"}`,
			contains: "secr****6789",
		},
		{
			name:     "valid json with api_key",
			input:    `{"api_key":"abcdefghijklmnop"}`,
			contains: "abcd****mnop",
		},
		{
			name:     "empty string",
			input:    "",
			contains: "",
		},
		{
			name:     "valid json with short password",
			input:    `{"password":"secret123"}`,
			contains: "secr****t123", // Redacted form for 9-char string
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactJSON(tt.input)
			if tt.contains != "" {
				assert.Contains(t, result, tt.contains)
			}
		})
	}
}

func TestRedactJSONRegex(t *testing.T) {
	input := `{"password": "secret123", "api_key": "key456"}`
	result := redactJSONRegex(input)
	assert.Contains(t, result, "[REDACTED]")
}

func TestLogSafe(t *testing.T) {
	log := NewLogSafe().
		SetRaw("action", "login").
		Set("api_key", "abcdefghijklmnop").
		SetEmail("email", "admin@example.com").
		SetIP("ip", "192.168.1.100")

	fields := log.Fields()

	if fields["action"] != "login" {
		t.Errorf("action should be 'login'")
	}
	if fields["api_key"] != "abcd****mnop" {
		t.Errorf("api_key should be redacted")
	}
	if fields["email"] != "ad***@example.com" {
		t.Errorf("email should be redacted")
	}
	if fields["ip"] != "192.168.*.*" {
		t.Errorf("ip should be redacted")
	}
}

func TestLogSafe_String(t *testing.T) {
	log := NewLogSafe().
		Set("username", "john").
		Set("password", "secret123456789")

	result := log.String()
	assert.Contains(t, result, "john")
	assert.Contains(t, result, "secr****6789")
}

func TestLogSafe_Set_NonString(t *testing.T) {
	log := NewLogSafe().
		Set("count", 123).
		Set("password", 456) // non-string sensitive

	fields := log.Fields()
	assert.Equal(t, 123, fields["count"])
	assert.Equal(t, "[REDACTED]", fields["password"])
}

func TestLogSafe_Chained(t *testing.T) {
	log := NewLogSafe().
		Set("a", "1").
		Set("b", "2").
		SetRaw("c", "3")

	assert.NotNil(t, log)
	assert.Len(t, log.Fields(), 3)
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

func TestParseToken(t *testing.T) {
	// Setup config
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret: "test-jwt-secret",
			Expire: 3600,
		},
	}
	config.Set(cfg)

	// Generate token
	token, err := GenerateToken(1, "test@example.com", true, cfg.JWT.Secret, cfg.JWT.Expire)
	require.NoError(t, err)

	// Parse using config secret
	claims, err := ParseToken(token)
	require.NoError(t, err)
	assert.Equal(t, uint(1), claims.UserID)
	assert.True(t, claims.IsAdmin)
}

func TestParseToken_NoConfig(t *testing.T) {
	// Clear config
	config.Set(nil)

	_, err := ParseToken("some-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config not loaded")
}
