package account

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/identity/secretbox"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Schema mirrors the product migration for the tests.
const Schema = `
CREATE TABLE t_account (user_id BIGINT PRIMARY KEY, account_uuid VARCHAR(36) NOT NULL UNIQUE, email VARCHAR(255) NOT NULL UNIQUE,
  password_hash TEXT NOT NULL, password_algo VARCHAR(20) NOT NULL, password_salt VARCHAR(64) NOT NULL,
  is_admin SMALLINT NOT NULL, is_staff SMALLINT NOT NULL, banned SMALLINT NOT NULL,
  token_version BIGINT NOT NULL, version BIGINT NOT NULL, invite_user_id BIGINT NOT NULL,
  created_at BIGINT NOT NULL, updated_at BIGINT NOT NULL);
CREATE TABLE t_mfa (user_id BIGINT PRIMARY KEY, enabled SMALLINT NOT NULL, sealed_totp_secret TEXT NOT NULL,
  backup_code_hashes TEXT NOT NULL, enabled_at BIGINT NOT NULL, last_used BIGINT NOT NULL, last_method VARCHAR(20) NOT NULL,
  updated_at BIGINT NOT NULL);
CREATE TABLE t_import_run (import_id VARCHAR(64) PRIMARY KEY, checkpoint TEXT NOT NULL, accounts BIGINT NOT NULL,
  updated_at BIGINT NOT NULL);`

func NewTestStore(t *testing.T) (*Store, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.Exec(Schema).Error)
	box, err := secretbox.New([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	return &Store{DB: db, Secrets: box, Tables: Tables{Account: "t_account", MFA: "t_mfa", ImportRun: "t_import_run"}}, db
}

func digest(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func member(id uint64, email string) Account {
	return Account{UserID: id, AccountUUID: "0b6f1c3e-8d5a-4a6e-9a0c-6b2f1d9e4a" + string(rune('0'+id%10)) + "0", Email: email,
		PasswordHash: "$2a$10$hash", CreatedAt: time.Unix(1700000000, 0)}
}

func TestImportIsIdempotentAndVersionsOnlyChanges(t *testing.T) {
	store, _ := NewTestStore(t)
	ctx := context.Background()
	batch := []Imported{
		{Account: member(1, "a@example.test"), MFA: &MFA{Enabled: true, TOTPSecret: "JBSWY3DPEHPK3PXP", BackupCodeHashes: []string{digest("AAAA-BBBB")}}},
		{Account: member(2, "b@example.test")},
	}
	result, err := store.Import(ctx, "import-1", "0", ImportBatch{Accounts: batch})
	require.NoError(t, err)
	require.Equal(t, ImportResult{Accounts: 2}, result)

	accounts, err := store.Get(ctx, []uint64{2, 1, 99})
	require.NoError(t, err)
	require.Len(t, accounts, 2)
	require.Equal(t, uint64(1), accounts[0].Version)
	require.Equal(t, uint64(1), accounts[0].TokenVersion)
	require.True(t, accounts[0].MFAEnabled)
	require.False(t, accounts[1].MFAEnabled)

	_, err = store.Import(ctx, "import-1", "2", ImportBatch{Accounts: batch})
	require.NoError(t, err)
	accounts, err = store.Get(ctx, []uint64{1})
	require.NoError(t, err)
	require.Equal(t, uint64(1), accounts[0].Version, "an unchanged account keeps its version")

	changed := batch[:1]
	changed[0].Account.Banned = true
	_, err = store.Import(ctx, "import-2", "1", ImportBatch{Accounts: changed})
	require.NoError(t, err)
	accounts, err = store.Get(ctx, []uint64{1})
	require.NoError(t, err)
	require.Equal(t, uint64(2), accounts[0].Version)
	require.True(t, accounts[0].Banned)

	exported, err := store.ChangedAfter(ctx, 1, 10)
	require.NoError(t, err)
	require.Len(t, exported, 1)
	require.Equal(t, uint64(1), exported[0].UserID)
}

func TestMFASecretsAreSealedAndBackupCodesKeyed(t *testing.T) {
	store, db := NewTestStore(t)
	ctx := context.Background()
	_, err := store.Import(ctx, "import-1", "0", ImportBatch{Accounts: []Imported{{Account: member(1, "a@example.test"), MFA: &MFA{
		Enabled: true, TOTPSecret: "JBSWY3DPEHPK3PXP", BackupCodeHashes: []string{digest("AAAA-BBBB")},
	}}}})
	require.NoError(t, err)

	var row mfaRow
	require.NoError(t, db.Table("t_mfa").Where("user_id = ?", 1).Take(&row).Error)
	require.NotContains(t, row.SealedTOTPSecret, "JBSWY3DPEHPK3PXP")
	require.NotContains(t, row.BackupCodeHashes, digest("AAAA-BBBB"), "only keyed hashes are stored")

	mfa, keyed, err := store.MFA(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, "JBSWY3DPEHPK3PXP", mfa.TOTPSecret)
	require.True(t, store.BackupCodeMatches(1, digest("AAAA-BBBB"), keyed))
	require.False(t, store.BackupCodeMatches(1, digest("AAAA-CCCC"), keyed))
	require.False(t, store.BackupCodeMatches(2, digest("AAAA-BBBB"), keyed), "hashes are bound to their account")

	_, err = store.Import(ctx, "import-2", "0", ImportBatch{Accounts: []Imported{{Account: member(1, "a@example.test")}}})
	require.NoError(t, err)
	mfa, _, err = store.MFA(ctx, 1)
	require.NoError(t, err)
	require.Nil(t, mfa, "MFA removed at the source is removed")

	result, err := store.Import(ctx, "import-3", "0", ImportBatch{Deleted: []uint64{1, 99}})
	require.NoError(t, err)
	require.EqualValues(t, 2, result.Deleted, "deleting an account that is already gone is not an error")
	accounts, err := store.Get(ctx, []uint64{1})
	require.NoError(t, err)
	require.Empty(t, accounts, "an account whose subscriber was deleted is deleted")
}

func TestImportRejectsIncompleteAccounts(t *testing.T) {
	store, _ := NewTestStore(t)
	_, err := store.Import(context.Background(), "import-1", "0", ImportBatch{Accounts: []Imported{{Account: Account{UserID: 1, Email: "a@example.test"}}}})
	require.Error(t, err)
	_, err = store.Import(context.Background(), "", "0", ImportBatch{})
	require.Error(t, err)
	_, err = (&Store{}).Get(context.Background(), []uint64{1})
	require.ErrorIs(t, err, ErrNotConfigured)
}
