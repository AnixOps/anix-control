CREATE TABLE IF NOT EXISTS identity_platform_projection (
  projection_key TEXT PRIMARY KEY,
  projection_value TEXT NOT NULL,
  updated_at BIGINT NOT NULL
);
