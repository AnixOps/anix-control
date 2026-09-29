-- Migration 0003: MFA Support
-- Add Multi-Factor Authentication tables

-- MFA settings table
CREATE TABLE IF NOT EXISTS user_mfa (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE,
    secret TEXT NOT NULL,
    recovery_codes TEXT NOT NULL,  -- JSON array of hashed recovery codes
    verified INTEGER DEFAULT 0,    -- 0 = not verified, 1 = verified/enabled
    created_at TEXT DEFAULT (datetime('now')),
    updated_at TEXT DEFAULT (datetime('now')),
    last_used_at TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Add MFA fields to users table
ALTER TABLE users ADD COLUMN mfa_enabled INTEGER DEFAULT 0;

-- MFA verification attempts (for rate limiting)
CREATE TABLE IF NOT EXISTS mfa_attempts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    attempt_at TEXT DEFAULT (datetime('now')),
    success INTEGER DEFAULT 0,
    ip TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_user_mfa_user_id ON user_mfa(user_id);
CREATE INDEX IF NOT EXISTS idx_user_mfa_verified ON user_mfa(verified);
CREATE INDEX IF NOT EXISTS idx_mfa_attempts_user_id ON mfa_attempts(user_id);
CREATE INDEX IF NOT EXISTS idx_mfa_attempts_attempt_at ON mfa_attempts(attempt_at);

-- Update existing users to have mfa_enabled = 0
UPDATE users SET mfa_enabled = 0 WHERE mfa_enabled IS NULL;