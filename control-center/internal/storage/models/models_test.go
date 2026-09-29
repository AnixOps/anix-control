package models

import (
	"testing"
	"time"
)

func TestUser_TableName(t *testing.T) {
	u := User{}
	if u.TableName() != "users" {
		t.Errorf("expected table name 'users', got '%s'", u.TableName())
	}
}

func TestAPIToken_TableName(t *testing.T) {
	a := APIToken{}
	if a.TableName() != "api_tokens" {
		t.Errorf("expected table name 'api_tokens', got '%s'", a.TableName())
	}
}

func TestAuditLog_TableName(t *testing.T) {
	a := AuditLog{}
	if a.TableName() != "audit_logs" {
		t.Errorf("expected table name 'audit_logs', got '%s'", a.TableName())
	}
}

func TestUser_Fields(t *testing.T) {
	now := time.Now()
	u := User{
		ID:           1,
		Email:        "test@example.com",
		PasswordHash: "hash",
		Role:         "admin",
		AuthProvider: "local",
		Enabled:      true,
		LastLoginAt:  &now,
	}

	if u.ID != 1 {
		t.Errorf("expected ID 1, got %d", u.ID)
	}
	if u.Email != "test@example.com" {
		t.Errorf("expected email 'test@example.com', got '%s'", u.Email)
	}
	if u.PasswordHash != "hash" {
		t.Errorf("expected password hash 'hash', got '%s'", u.PasswordHash)
	}
	if u.Role != "admin" {
		t.Errorf("expected role 'admin', got '%s'", u.Role)
	}
	if u.AuthProvider != "local" {
		t.Errorf("expected auth provider 'local', got '%s'", u.AuthProvider)
	}
	if !u.Enabled {
		t.Error("expected enabled to be true")
	}
}

func TestAPIToken_Fields(t *testing.T) {
	now := time.Now()
	a := APIToken{
		ID:        1,
		UserID:    1,
		Name:      "test-token",
		Token:     "secret-token",
		ExpiresAt: &now,
		LastUsed:  &now,
	}

	if a.ID != 1 {
		t.Errorf("expected ID 1, got %d", a.ID)
	}
	if a.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", a.UserID)
	}
	if a.Name != "test-token" {
		t.Errorf("expected name 'test-token', got '%s'", a.Name)
	}
	if a.Token != "secret-token" {
		t.Errorf("expected token 'secret-token', got '%s'", a.Token)
	}
}

func TestAuditLog_Fields(t *testing.T) {
	a := AuditLog{
		ID:        1,
		UserID:    1,
		Action:    "login",
		Resource:  "auth",
		IP:        "127.0.0.1",
		UserAgent: "test-agent",
		Details:   "test details",
	}

	if a.ID != 1 {
		t.Errorf("expected ID 1, got %d", a.ID)
	}
	if a.UserID != 1 {
		t.Errorf("expected UserID 1, got %d", a.UserID)
	}
	if a.Action != "login" {
		t.Errorf("expected action 'login', got '%s'", a.Action)
	}
	if a.Resource != "auth" {
		t.Errorf("expected resource 'auth', got '%s'", a.Resource)
	}
	if a.IP != "127.0.0.1" {
		t.Errorf("expected IP '127.0.0.1', got '%s'", a.IP)
	}
}
