package utils

import (
	"testing"
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
