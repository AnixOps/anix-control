package auth

import (
	"sync"
)

// Permission represents a permission
type Permission struct {
	Resource string `json:"resource"` // nodes, users, playbooks, etc.
	Action   string `json:"action"`   // read, write, execute, delete, *
}

// Role represents a role with permissions
type Role struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Permissions []Permission `json:"permissions"`
}

// RBACManager manages role-based access control
type RBACManager struct {
	mu    sync.RWMutex
	roles map[string]*Role
}

// NewRBACManager creates a new RBAC manager
func NewRBACManager() *RBACManager {
	m := &RBACManager{
		roles: make(map[string]*Role),
	}
	m.initDefaultRoles()
	return m
}

// initDefaultRoles initializes default roles
func (m *RBACManager) initDefaultRoles() {
	// Admin role - full access
	m.roles["admin"] = &Role{
		Name:        "admin",
		Description: "Full system access",
		Permissions: []Permission{
			{Resource: "*", Action: "*"},
		},
	}

	// Operator role - operational access
	m.roles["operator"] = &Role{
		Name:        "operator",
		Description: "Operational access",
		Permissions: []Permission{
			{Resource: "nodes", Action: "read"},
			{Resource: "nodes", Action: "write"},
			{Resource: "playbooks", Action: "execute"},
			{Resource: "logs", Action: "read"},
			{Resource: "dashboard", Action: "view"},
			{Resource: "tasks", Action: "read"},
			{Resource: "tasks", Action: "execute"},
		},
	}

	// Viewer role - read-only access
	m.roles["viewer"] = &Role{
		Name:        "viewer",
		Description: "Read-only access",
		Permissions: []Permission{
			{Resource: "nodes", Action: "read"},
			{Resource: "logs", Action: "read"},
			{Resource: "dashboard", Action: "view"},
		},
	}
}

// AddRole adds a custom role
func (m *RBACManager) AddRole(role *Role) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.roles[role.Name] = role
}

// GetRole retrieves a role by name
func (m *RBACManager) GetRole(name string) (*Role, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	role, ok := m.roles[name]
	return role, ok
}

// HasPermission checks if a role has a specific permission
func (m *RBACManager) HasPermission(roleName, resource, action string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	role, ok := m.roles[roleName]
	if !ok {
		return false
	}

	for _, perm := range role.Permissions {
		// Check for wildcard permission
		if perm.Resource == "*" && perm.Action == "*" {
			return true
		}

		// Check for wildcard resource
		if perm.Resource == "*" && perm.Action == action {
			return true
		}

		// Check for wildcard action
		if perm.Resource == resource && perm.Action == "*" {
			return true
		}

		// Check exact match
		if perm.Resource == resource && perm.Action == action {
			return true
		}
	}

	return false
}

// CheckPermission checks if a user with given role has permission
func (m *RBACManager) CheckPermission(role string, resource, action string) bool {
	return m.HasPermission(role, resource, action)
}

// ListRoles returns all role names
func (m *RBACManager) ListRoles() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.roles))
	for name := range m.roles {
		names = append(names, name)
	}
	return names
}

// GetRolePermissions returns all permissions for a role
func (m *RBACManager) GetRolePermissions(roleName string) []Permission {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if role, ok := m.roles[roleName]; ok {
		return role.Permissions
	}
	return nil
}
