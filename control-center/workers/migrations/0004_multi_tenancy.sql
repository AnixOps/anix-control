-- Migration 0004: Multi-Tenancy Support
-- Add tenant isolation and enterprise features

-- Tenants table
CREATE TABLE IF NOT EXISTS tenants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    plan TEXT DEFAULT 'free' CHECK(plan IN ('free', 'pro', 'enterprise')),
    status TEXT DEFAULT 'active' CHECK(status IN ('active', 'suspended', 'cancelled')),
    settings TEXT,  -- JSON: theme, notifications, integrations
    quotas TEXT,    -- JSON: max_nodes, max_users, max_playbooks, storage_gb
    billing_email TEXT,
    stripe_customer_id TEXT,
    trial_ends_at TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

-- Custom roles per tenant
CREATE TABLE IF NOT EXISTS roles (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER,  -- NULL for system roles
    name TEXT NOT NULL,
    display_name TEXT,
    description TEXT,
    permissions TEXT,  -- JSON array of permission strings
    is_system INTEGER DEFAULT 0,  -- System roles cannot be deleted
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    UNIQUE(tenant_id, name)
);

-- User-tenant membership
CREATE TABLE IF NOT EXISTS tenant_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    role_id INTEGER,  -- Custom role
    invited_by INTEGER,
    joined_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE SET NULL,
    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE SET NULL,
    UNIQUE(tenant_id, user_id)
);

-- Add tenant_id to existing tables
ALTER TABLE users ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);
ALTER TABLE nodes ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);
ALTER TABLE playbooks ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);
ALTER TABLE tasks ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);
ALTER TABLE schedules ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);
ALTER TABLE node_groups ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);
ALTER TABLE notifications ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);
ALTER TABLE audit_logs ADD COLUMN tenant_id INTEGER REFERENCES tenants(id);

-- Tenant invitations
CREATE TABLE IF NOT EXISTS tenant_invitations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL,
    email TEXT NOT NULL,
    role_id INTEGER,
    invited_by INTEGER NOT NULL,
    token TEXT UNIQUE NOT NULL,
    expires_at TEXT NOT NULL,
    accepted_at TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY (invited_by) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (role_id) REFERENCES roles(id) ON DELETE SET NULL
);

-- Tenant API keys (for tenant-level API access)
CREATE TABLE IF NOT EXISTS tenant_api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    key_hash TEXT UNIQUE NOT NULL,
    key_prefix TEXT,  -- First 8 chars for identification
    permissions TEXT,  -- JSON array
    last_used TEXT,
    expires_at TEXT,
    created_by INTEGER,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- Resource usage tracking (for quotas)
CREATE TABLE IF NOT EXISTS tenant_usage (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    tenant_id INTEGER NOT NULL,
    period_start TEXT NOT NULL,
    period_end TEXT NOT NULL,
    nodes_count INTEGER DEFAULT 0,
    users_count INTEGER DEFAULT 0,
    playbooks_count INTEGER DEFAULT 0,
    tasks_count INTEGER DEFAULT 0,
    storage_bytes INTEGER DEFAULT 0,
    api_calls_count INTEGER DEFAULT 0,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (tenant_id) REFERENCES tenants(id) ON DELETE CASCADE
);

-- System-level permission definitions
CREATE TABLE IF NOT EXISTS permissions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    display_name TEXT,
    category TEXT,  -- nodes, users, playbooks, settings, etc.
    description TEXT,
    created_at TEXT DEFAULT (datetime('now'))
);

-- Insert default system roles
INSERT INTO roles (name, display_name, description, permissions, is_system) VALUES
('admin', 'Administrator', 'Full access to all features', '["*"]', 1),
('operator', 'Operator', 'Can manage nodes and run playbooks', '["nodes:read", "nodes:write", "nodes:execute", "playbooks:read", "playbooks:execute", "tasks:read", "tasks:create", "tasks:cancel", "schedules:read", "schedules:create"]', 1),
('viewer', 'Viewer', 'Read-only access', '["nodes:read", "playbooks:read", "tasks:read", "schedules:read", "dashboard:read"]', 1);

-- Insert default permissions
INSERT INTO permissions (name, display_name, category, description) VALUES
-- Nodes
('nodes:read', 'View Nodes', 'nodes', 'View node list and details'),
('nodes:write', 'Manage Nodes', 'nodes', 'Create, update, and delete nodes'),
('nodes:execute', 'Execute Node Actions', 'nodes', 'Start, stop, restart nodes'),
('nodes:delete', 'Delete Nodes', 'nodes', 'Delete nodes'),
-- Users
('users:read', 'View Users', 'users', 'View user list and details'),
('users:write', 'Manage Users', 'users', 'Create and update users'),
('users:delete', 'Delete Users', 'users', 'Delete users'),
-- Playbooks
('playbooks:read', 'View Playbooks', 'playbooks', 'View playbook list and content'),
('playbooks:write', 'Manage Playbooks', 'playbooks', 'Create and update playbooks'),
('playbooks:execute', 'Execute Playbooks', 'playbooks', 'Run playbooks on nodes'),
('playbooks:delete', 'Delete Playbooks', 'playbooks', 'Delete playbooks'),
-- Tasks
('tasks:read', 'View Tasks', 'tasks', 'View task list and details'),
('tasks:create', 'Create Tasks', 'tasks', 'Create new tasks'),
('tasks:cancel', 'Cancel Tasks', 'tasks', 'Cancel running tasks'),
-- Schedules
('schedules:read', 'View Schedules', 'schedules', 'View schedule list'),
('schedules:create', 'Manage Schedules', 'schedules', 'Create and update schedules'),
('schedules:delete', 'Delete Schedules', 'schedules', 'Delete schedules'),
-- Settings
('settings:read', 'View Settings', 'settings', 'View tenant settings'),
('settings:write', 'Manage Settings', 'settings', 'Update tenant settings'),
-- Audit
('audit:read', 'View Audit Logs', 'audit', 'View audit logs'),
-- Backup
('backup:read', 'View Backups', 'backup', 'View backup list'),
('backup:create', 'Create Backups', 'backup', 'Create and restore backups'),
('backup:delete', 'Delete Backups', 'backup', 'Delete backups'),
-- MFA
('mfa:manage', 'Manage MFA', 'settings', 'Enable/disable MFA for users'),
-- Dashboard
('dashboard:read', 'View Dashboard', 'dashboard', 'View dashboard and statistics');

-- Indexes
CREATE INDEX IF NOT EXISTS idx_tenants_slug ON tenants(slug);
CREATE INDEX IF NOT EXISTS idx_tenants_status ON tenants(status);
CREATE INDEX IF NOT EXISTS idx_roles_tenant_id ON roles(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_members_tenant_id ON tenant_members(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_members_user_id ON tenant_members(user_id);
CREATE INDEX IF NOT EXISTS idx_tenant_invitations_tenant_id ON tenant_invitations(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_invitations_token ON tenant_invitations(token);
CREATE INDEX IF NOT EXISTS idx_tenant_api_keys_tenant_id ON tenant_api_keys(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tenant_usage_tenant_id ON tenant_usage(tenant_id);
CREATE INDEX IF NOT EXISTS idx_users_tenant_id ON users(tenant_id);
CREATE INDEX IF NOT EXISTS idx_nodes_tenant_id ON nodes(tenant_id);
CREATE INDEX IF NOT EXISTS idx_playbooks_tenant_id ON playbooks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_tasks_tenant_id ON tasks(tenant_id);
CREATE INDEX IF NOT EXISTS idx_schedules_tenant_id ON schedules(tenant_id);
CREATE INDEX IF NOT EXISTS idx_audit_logs_tenant_id ON audit_logs(tenant_id);
CREATE INDEX IF NOT EXISTS idx_permissions_category ON permissions(category);