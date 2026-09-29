-- AnixOps D1 Database Schema
-- Migration 0002: Orchestration Tables

-- 节点组表
CREATE TABLE IF NOT EXISTS node_groups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    parent_id INTEGER,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (parent_id) REFERENCES node_groups(id) ON DELETE SET NULL
);

-- 更新 nodes 表添加 group_id
-- SQLite 不支持 ADD COLUMN IF NOT EXISTS，需要先检查
-- 在应用层处理或使用 PRAGMA table_info 检查

-- 增强版 Playbook 表 (添加新字段)
-- SQLite 需要重建表来添加字段，这里创建新表
CREATE TABLE IF NOT EXISTS playbooks_v2 (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    description TEXT,
    category TEXT DEFAULT 'custom' CHECK(category IN ('security', 'infrastructure', 'proxy', 'maintenance', 'ssl', 'custom')),
    source TEXT DEFAULT 'custom' CHECK(source IN ('built-in', 'github', 'custom')),
    github_repo TEXT,
    github_path TEXT,
    version TEXT DEFAULT '1.0.0',
    storage_key TEXT NOT NULL,
    variables TEXT,  -- JSON schema for variables
    author TEXT,
    tags TEXT,  -- JSON array
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

-- 迁移旧数据
INSERT OR IGNORE INTO playbooks_v2 (id, name, storage_key, description, created_at, updated_at)
SELECT id, name, storage_key, description, created_at, updated_at FROM playbooks;

-- 删除旧表，重命名新表
DROP TABLE IF EXISTS playbooks;
ALTER TABLE playbooks_v2 RENAME TO playbooks;

-- 执行任务表
CREATE TABLE IF NOT EXISTS tasks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT UNIQUE NOT NULL,  -- UUID
    playbook_id INTEGER NOT NULL,
    playbook_name TEXT NOT NULL,
    status TEXT DEFAULT 'pending' CHECK(status IN ('pending', 'running', 'success', 'failed', 'cancelled')),
    trigger_type TEXT DEFAULT 'manual' CHECK(trigger_type IN ('manual', 'scheduled', 'webhook', 'api')),
    triggered_by INTEGER,
    target_nodes TEXT,  -- JSON array of node IDs or names
    variables TEXT,  -- JSON object
    result TEXT,  -- JSON object with results per node
    error TEXT,
    started_at TEXT,
    completed_at TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (playbook_id) REFERENCES playbooks(id) ON DELETE CASCADE,
    FOREIGN KEY (triggered_by) REFERENCES users(id) ON DELETE SET NULL
);

-- 任务日志表
CREATE TABLE IF NOT EXISTS task_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id TEXT NOT NULL,
    node_id INTEGER,
    node_name TEXT,
    level TEXT DEFAULT 'info' CHECK(level IN ('debug', 'info', 'warning', 'error')),
    message TEXT,
    metadata TEXT,  -- JSON
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (task_id) REFERENCES tasks(task_id) ON DELETE CASCADE,
    FOREIGN KEY (node_id) REFERENCES nodes(id) ON DELETE SET NULL
);

-- 调度任务表
CREATE TABLE IF NOT EXISTS schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    playbook_id INTEGER NOT NULL,
    playbook_name TEXT NOT NULL,
    cron TEXT NOT NULL,
    timezone TEXT DEFAULT 'UTC',
    target_nodes TEXT,  -- JSON array
    variables TEXT,  -- JSON object
    enabled INTEGER DEFAULT 1,
    last_run TEXT,
    next_run TEXT,
    last_task_id TEXT,
    created_by INTEGER,
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (playbook_id) REFERENCES playbooks(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- 插件表
CREATE TABLE IF NOT EXISTS plugins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT UNIQUE NOT NULL,
    display_name TEXT,
    version TEXT DEFAULT '1.0.0',
    description TEXT,
    author TEXT DEFAULT 'AnixOps',
    type TEXT DEFAULT 'core' CHECK(type IN ('core', 'server', 'custom')),
    enabled INTEGER DEFAULT 1,
    config TEXT,  -- JSON configuration
    permissions TEXT,  -- JSON array of required permissions
    installed_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now'))
);

-- 通知表增强 (添加更多类型)
CREATE TABLE IF NOT EXISTS notifications (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    type TEXT DEFAULT 'info' CHECK(type IN ('info', 'success', 'warning', 'error', 'task', 'system')),
    title TEXT NOT NULL,
    message TEXT,
    resource_type TEXT,  -- node, playbook, task, system
    resource_id TEXT,
    read INTEGER DEFAULT 0,
    action_url TEXT,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Webhook 表
CREATE TABLE IF NOT EXISTS webhooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    secret TEXT,
    events TEXT,  -- JSON array of events to subscribe
    enabled INTEGER DEFAULT 1,
    last_triggered TEXT,
    created_by INTEGER,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- GitHub 同步配置表
CREATE TABLE IF NOT EXISTS github_sync_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    repo_url TEXT NOT NULL,
    branch TEXT DEFAULT 'main',
    path TEXT DEFAULT '/',
    token TEXT,  -- GitHub token (encrypted in KV ideally)
    auto_sync INTEGER DEFAULT 0,
    sync_interval INTEGER DEFAULT 3600,  -- seconds
    last_sync TEXT,
    last_commit TEXT,
    created_by INTEGER,
    created_at TEXT DEFAULT (datetime('now')),
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL
);

-- 新索引
CREATE INDEX IF NOT EXISTS idx_node_groups_parent ON node_groups(parent_id);
CREATE INDEX IF NOT EXISTS idx_playbooks_category ON playbooks(category);
CREATE INDEX IF NOT EXISTS idx_playbooks_source ON playbooks(source);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_task_id ON tasks(task_id);
CREATE INDEX IF NOT EXISTS idx_tasks_playbook_id ON tasks(playbook_id);
CREATE INDEX IF NOT EXISTS idx_tasks_triggered_by ON tasks(triggered_by);
CREATE INDEX IF NOT EXISTS idx_tasks_created_at ON tasks(created_at);
CREATE INDEX IF NOT EXISTS idx_task_logs_task_id ON task_logs(task_id);
CREATE INDEX IF NOT EXISTS idx_task_logs_level ON task_logs(level);
CREATE INDEX IF NOT EXISTS idx_schedules_enabled ON schedules(enabled);
CREATE INDEX IF NOT EXISTS idx_schedules_next_run ON schedules(next_run);
CREATE INDEX IF NOT EXISTS idx_schedules_playbook_id ON schedules(playbook_id);
CREATE INDEX IF NOT EXISTS idx_plugins_enabled ON plugins(enabled);
CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id);
CREATE INDEX IF NOT EXISTS idx_notifications_read ON notifications(read);
CREATE INDEX IF NOT EXISTS idx_notifications_type ON notifications(type);
CREATE INDEX IF NOT EXISTS idx_webhooks_enabled ON webhooks(enabled);
CREATE INDEX IF NOT EXISTS idx_github_sync_enabled ON github_sync_configs(auto_sync);

-- 插入内置插件
INSERT OR IGNORE INTO plugins (name, display_name, version, description, author, type, enabled) VALUES
    ('webssh', 'WebSSH Terminal', '1.0.0', 'Browser-based SSH terminal for secure node access', 'AnixOps', 'core', 1),
    ('monitor', 'Node Monitor', '1.0.0', 'Real-time node monitoring and metrics collection', 'AnixOps', 'core', 1),
    ('backup', 'Backup Manager', '1.0.0', 'Automated backup and restore for configurations', 'AnixOps', 'core', 1),
    ('alert', 'Alert Center', '1.0.0', 'Notification and alerting system', 'AnixOps', 'core', 1),
    ('logs', 'Log Viewer', '1.0.0', 'Log aggregation and search', 'AnixOps', 'core', 1),
    ('scheduler', 'Task Scheduler', '1.0.0', 'Cron-based task scheduling', 'AnixOps', 'core', 1);

-- 插入默认节点组
INSERT OR IGNORE INTO node_groups (name, description) VALUES
    ('All Nodes', 'Default group containing all nodes'),
    ('Production', 'Production environment nodes'),
    ('Development', 'Development environment nodes'),
    ('Proxy Servers', 'Proxy and VPN servers');