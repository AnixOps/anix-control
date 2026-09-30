-- Identity signing keys: Ed25519, private halves sealed under the identity
-- key-encryption key (identity/keystore). Times are Unix seconds, 0 = unset.
CREATE TABLE IF NOT EXISTS __PKG_PREFIX__signing_key (
  id VARCHAR(64) PRIMARY KEY,
  state VARCHAR(16) NOT NULL,
  public_key TEXT NOT NULL,
  sealed_private_key TEXT NOT NULL,
  created_at BIGINT NOT NULL,
  activated_at BIGINT NOT NULL,
  retired_at BIGINT NOT NULL
);
