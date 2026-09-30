-- Identity accounts (identity/account). Invite codes stay with Control. user_id is the Control subscriber id
-- (v2_user.id). TOTP secrets are sealed and backup codes kept as keyed hashes
-- under the identity key-encryption key. Times are Unix seconds, flags 0/1.
CREATE TABLE IF NOT EXISTS __PKG_PREFIX__account (
  user_id BIGINT PRIMARY KEY,
  account_uuid VARCHAR(36) NOT NULL UNIQUE,
  email VARCHAR(255) NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  password_algo VARCHAR(20) NOT NULL,
  password_salt VARCHAR(64) NOT NULL,
  is_admin SMALLINT NOT NULL,
  is_staff SMALLINT NOT NULL,
  banned SMALLINT NOT NULL,
  token_version BIGINT NOT NULL,
  version BIGINT NOT NULL,
  invite_user_id BIGINT NOT NULL,
  created_at BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS __PKG_PREFIX__account_version ON __PKG_PREFIX__account (version);
CREATE TABLE IF NOT EXISTS __PKG_PREFIX__mfa (
  user_id BIGINT PRIMARY KEY,
  enabled SMALLINT NOT NULL,
  sealed_totp_secret TEXT NOT NULL,
  backup_code_hashes TEXT NOT NULL,
  enabled_at BIGINT NOT NULL,
  last_used BIGINT NOT NULL,
  last_method VARCHAR(20) NOT NULL,
  updated_at BIGINT NOT NULL
);
CREATE TABLE IF NOT EXISTS __PKG_PREFIX__import_run (
  import_id VARCHAR(64) PRIMARY KEY,
  checkpoint TEXT NOT NULL,
  accounts BIGINT NOT NULL,
  updated_at BIGINT NOT NULL
);
