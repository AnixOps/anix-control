package auth_test

import (
	"testing"
	"time"

	"github.com/AnixOps/anix-control/control-center/internal/security/auth"
)

func TestNewJWTManager(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")
	if jwt == nil {
		t.Fatal("expected non-nil JWT manager")
	}
}

func TestGenerateAccessToken(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	token, err := jwt.GenerateAccessToken("user-123", "admin", []string{"read", "write"}, "local")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestGenerateRefreshToken(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	token, err := jwt.GenerateRefreshToken("user-123", "admin", "local")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if token == "" {
		t.Error("expected non-empty token")
	}
}

func TestValidateToken(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	// Generate and validate access token
	accessToken, _ := jwt.GenerateAccessToken("user-123", "admin", nil, "local")

	claims, err := jwt.ValidateToken(accessToken)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if claims.Subject != "user-123" {
		t.Errorf("expected subject 'user-123', got '%s'", claims.Subject)
	}

	if claims.Role != "admin" {
		t.Errorf("expected role 'admin', got '%s'", claims.Role)
	}
}

func TestValidateToken_Invalid(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	// Test with invalid token
	_, err := jwt.ValidateToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid token")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	jwt1 := auth.NewJWTManager("secret-1", 3600, 86400, "test-issuer")
	jwt2 := auth.NewJWTManager("secret-2", 3600, 86400, "test-issuer")

	token, _ := jwt1.GenerateAccessToken("user-123", "admin", nil, "local")

	// Validate with different secret
	_, err := jwt2.ValidateToken(token)
	if err == nil {
		t.Error("expected error for token signed with different secret")
	}
}

func TestRefreshAccessToken(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	// Generate refresh token
	refreshToken, _ := jwt.GenerateRefreshToken("user-123", "admin", "local")

	// Refresh access token
	newAccessToken, err := jwt.RefreshAccessToken(refreshToken)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Validate new access token
	claims, err := jwt.ValidateToken(newAccessToken)
	if err != nil {
		t.Fatalf("expected no error validating new token, got %v", err)
	}

	if claims.Subject != "user-123" {
		t.Errorf("expected subject 'user-123', got '%s'", claims.Subject)
	}
}

func TestHashPassword(t *testing.T) {
	password := "test-password-123"

	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if hash == "" {
		t.Error("expected non-empty hash")
	}

	if hash == password {
		t.Error("expected hash to be different from password")
	}
}

func TestCheckPassword(t *testing.T) {
	password := "test-password-123"

	hash, _ := auth.HashPassword(password)

	// Test correct password
	if !auth.CheckPassword(password, hash) {
		t.Error("expected password check to succeed")
	}

	// Test wrong password
	if auth.CheckPassword("wrong-password", hash) {
		t.Error("expected password check to fail")
	}
}

func TestRBACManager(t *testing.T) {
	mgr := auth.NewRBACManager()

	if mgr == nil {
		t.Fatal("expected non-nil RBAC manager")
	}
}

func TestRBACManager_GetRole(t *testing.T) {
	mgr := auth.NewRBACManager()

	// Get default roles
	admin, ok := mgr.GetRole("admin")
	if !ok {
		t.Fatal("expected to find admin role")
	}

	if admin.Name != "admin" {
		t.Errorf("expected role name 'admin', got '%s'", admin.Name)
	}
}

func TestRBACManager_HasPermission(t *testing.T) {
	mgr := auth.NewRBACManager()

	// Test admin has all permissions
	if !mgr.HasPermission("admin", "nodes", "read") {
		t.Error("expected admin to have all permissions")
	}

	if !mgr.HasPermission("admin", "users", "delete") {
		t.Error("expected admin to have all permissions")
	}

	// Test operator has specific permissions
	if !mgr.HasPermission("operator", "nodes", "read") {
		t.Error("expected operator to have nodes:read")
	}

	if !mgr.HasPermission("operator", "playbooks", "execute") {
		t.Error("expected operator to have playbooks:execute")
	}

	// Test operator doesn't have admin permissions
	if mgr.HasPermission("operator", "users", "delete") {
		t.Error("expected operator not to have users:delete")
	}

	// Test viewer has read-only access
	if !mgr.HasPermission("viewer", "nodes", "read") {
		t.Error("expected viewer to have nodes:read")
	}

	if mgr.HasPermission("viewer", "nodes", "write") {
		t.Error("expected viewer not to have nodes:write")
	}
}

func TestRBACManager_AddRole(t *testing.T) {
	mgr := auth.NewRBACManager()

	// Add custom role
	customRole := &auth.Role{
		Name:        "custom",
		Description: "Custom role",
		Permissions: []auth.Permission{
			{Resource: "dashboard", Action: "view"},
		},
	}

	mgr.AddRole(customRole)

	role, ok := mgr.GetRole("custom")
	if !ok {
		t.Fatal("expected to find custom role")
	}

	if role.Name != "custom" {
		t.Errorf("expected role name 'custom', got '%s'", role.Name)
	}
}

func TestRBACManager_ListRoles(t *testing.T) {
	mgr := auth.NewRBACManager()

	roles := mgr.ListRoles()

	if len(roles) < 3 {
		t.Errorf("expected at least 3 default roles, got %d", len(roles))
	}
}

func TestTokenExpiration(t *testing.T) {
	// Create a manager with very short expiration
	jwt := auth.NewJWTManager("test-secret", 1, 1, "test-issuer")

	token, _ := jwt.GenerateAccessToken("user-123", "admin", nil, "local")

	// Wait for token to expire
	time.Sleep(2 * time.Second)

	// Token should be expired
	_, err := jwt.ValidateToken(token)
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestGenerateAPIToken(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	token, err := jwt.GenerateAPIToken("user-123", "my-api-key")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if token == "" {
		t.Error("expected non-empty API token")
	}

	// Validate the token
	claims, err := jwt.ValidateToken(token)
	if err != nil {
		t.Fatalf("expected no error validating API token, got %v", err)
	}

	if claims.Role != "api" {
		t.Errorf("expected role 'api', got '%s'", claims.Role)
	}
}

func TestRefreshAccessToken_InvalidToken(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	// Try to refresh with invalid token
	_, err := jwt.RefreshAccessToken("invalid-token")
	if err == nil {
		t.Error("expected error for invalid refresh token")
	}
}

func TestRBACManager_CheckPermission(t *testing.T) {
	mgr := auth.NewRBACManager()

	// Test CheckPermission (alias for HasPermission)
	if !mgr.CheckPermission("admin", "nodes", "read") {
		t.Error("expected admin to have nodes:read permission")
	}

	if !mgr.CheckPermission("operator", "playbooks", "execute") {
		t.Error("expected operator to have playbooks:execute permission")
	}

	if mgr.CheckPermission("viewer", "nodes", "write") {
		t.Error("expected viewer not to have nodes:write permission")
	}

	// Test non-existent role
	if mgr.CheckPermission("nonexistent", "nodes", "read") {
		t.Error("expected nonexistent role to have no permissions")
	}
}

func TestRBACManager_GetRolePermissions(t *testing.T) {
	mgr := auth.NewRBACManager()

	// Get admin permissions
	adminPerms := mgr.GetRolePermissions("admin")
	if len(adminPerms) != 1 {
		t.Errorf("expected 1 admin permission, got %d", len(adminPerms))
	}

	if adminPerms[0].Resource != "*" || adminPerms[0].Action != "*" {
		t.Error("expected admin to have wildcard permission")
	}

	// Get operator permissions
	operatorPerms := mgr.GetRolePermissions("operator")
	if len(operatorPerms) == 0 {
		t.Error("expected operator to have permissions")
	}

	// Get viewer permissions
	viewerPerms := mgr.GetRolePermissions("viewer")
	if len(viewerPerms) == 0 {
		t.Error("expected viewer to have permissions")
	}

	// Get non-existent role permissions
	nonexistentPerms := mgr.GetRolePermissions("nonexistent")
	if nonexistentPerms != nil {
		t.Error("expected nil for non-existent role")
	}
}

func TestClaims_AllFields(t *testing.T) {
	jwt := auth.NewJWTManager("test-secret", 3600, 86400, "test-issuer")

	token, _ := jwt.GenerateAccessToken("user-456", "operator", []string{"read", "write"}, "oauth")

	claims, _ := jwt.ValidateToken(token)

	if claims.Subject != "user-456" {
		t.Errorf("expected subject 'user-456', got '%s'", claims.Subject)
	}
	if claims.Role != "operator" {
		t.Errorf("expected role 'operator', got '%s'", claims.Role)
	}
	if len(claims.Scopes) != 2 {
		t.Errorf("expected 2 scopes, got %d", len(claims.Scopes))
	}
	if claims.AuthProvider != "oauth" {
		t.Errorf("expected auth provider 'oauth', got '%s'", claims.AuthProvider)
	}
	if claims.Issuer != "test-issuer" {
		t.Errorf("expected issuer 'test-issuer', got '%s'", claims.Issuer)
	}
}

func TestRole_Struct(t *testing.T) {
	role := &auth.Role{
		Name:        "test-role",
		Description: "Test role description",
		Permissions: []auth.Permission{
			{Resource: "test", Action: "read"},
			{Resource: "test", Action: "write"},
		},
	}

	if role.Name != "test-role" {
		t.Errorf("expected name 'test-role', got '%s'", role.Name)
	}
	if len(role.Permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(role.Permissions))
	}
}

func TestPermission_Struct(t *testing.T) {
	perm := auth.Permission{Resource: "nodes", Action: "read"}

	if perm.Resource != "nodes" {
		t.Errorf("expected resource 'nodes', got '%s'", perm.Resource)
	}
	if perm.Action != "read" {
		t.Errorf("expected action 'read', got '%s'", perm.Action)
	}
}
