-- Identity login state: shared attempt throttling, configuration documents
-- (seeded from Control) and MFA login attempts. Times are Unix seconds except
-- throttle, which uses milliseconds.
CREATE TABLE IF NOT EXISTS __PKG_PREFIX__throttle (
  throttle_key VARCHAR(255) PRIMARY KEY,
  failures INTEGER NOT NULL,
  window_start BIGINT NOT NULL,
  lock_until BIGINT NOT NULL,
  last_seen BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS __PKG_NAME_PREFIX__throttle_last_seen ON __PKG_PREFIX__throttle (last_seen);
CREATE TABLE IF NOT EXISTS __PKG_PREFIX__setting (
  setting_key VARCHAR(64) PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS __PKG_PREFIX__mfa_attempt (
  id VARCHAR(36) PRIMARY KEY,
  user_id BIGINT NOT NULL,
  ip VARCHAR(64) NOT NULL,
  user_agent VARCHAR(255) NOT NULL,
  success SMALLINT NOT NULL,
  method VARCHAR(20) NOT NULL,
  created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS __PKG_NAME_PREFIX__mfa_attempt_user ON __PKG_PREFIX__mfa_attempt (user_id, created_at);
