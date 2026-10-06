package adminapitoken

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v4/internal/model"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var gormConfig = &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}

func migrate(t *testing.T, db *gorm.DB) {
	t.Helper()
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.AdminAPIToken{}, &model.OperationLog{}))
}

func openSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "kernel.db")+"?_pragma=busy_timeout(10000)"), gormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

// openPostgres opens a throwaway schema of ANIX_TEST_POSTGRES_DSN.
func openPostgres(t *testing.T) *gorm.DB {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("ANIX_TEST_POSTGRES_DSN"))
	if base == "" {
		t.Skip("ANIX_TEST_POSTGRES_DSN is not set")
	}
	admin, err := gorm.Open(postgres.Open(base), gormConfig)
	require.NoError(t, err)
	adminDB, err := admin.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = adminDB.Close() })
	var databaseName string
	require.NoError(t, admin.Raw("SELECT current_database()").Scan(&databaseName).Error)
	if !strings.Contains(strings.ToLower(databaseName), "test") && os.Getenv("ANIX_TEST_POSTGRES_ALLOW_UNSAFE") != "1" {
		t.Skipf("refusing to run destructive postgres test against database %q", databaseName)
	}
	suffix := make([]byte, 4)
	_, err = rand.Read(suffix)
	require.NoError(t, err)
	schema := "admin_api_token_" + hex.EncodeToString(suffix)
	require.NoError(t, admin.Exec(`CREATE SCHEMA "`+schema+`"`).Error)
	t.Cleanup(func() { _ = admin.Exec(`DROP SCHEMA IF EXISTS "` + schema + `" CASCADE`).Error })
	db, err := gorm.Open(postgres.Open(base+" search_path="+schema), gormConfig)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	migrate(t, db)
	return db
}

func forEachDatabase(t *testing.T, body func(t *testing.T, db *gorm.DB)) {
	t.Helper()
	t.Run("sqlite", func(t *testing.T) { body(t, openSQLite(t)) })
	t.Run("postgres", func(t *testing.T) { body(t, openPostgres(t)) })
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newUsers(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, user := range []model.User{
		{ID: 7, Email: "root@example.com", IsAdmin: 1},
		{ID: 8, Email: "staff@example.com", IsAdmin: 1, IsStaff: 1},
		{ID: 9, Email: "member@example.com"},
	} {
		user.Token = "token-" + user.Email
		user.UUID = "uuid-" + user.Email
		require.NoError(t, db.Create(&user).Error)
	}
}

func newService(t *testing.T, db *gorm.DB) (*Service, *clock) {
	t.Helper()
	newUsers(t, db)
	c := &clock{now: time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)}
	return New(db).WithClock(c.Now), c
}

func mustCreate(t *testing.T, s *Service, user uint, scope string, expires *time.Time) (string, model.AdminAPIToken) {
	t.Helper()
	token, row, err := s.Create(context.Background(), CreateInput{UserID: user, Name: "ci", Scope: scope, ExpiresAt: expires, Actor: "root@example.com", IP: "192.0.2.10"})
	require.NoError(t, err)
	return token, row
}

func TestGenerateShapeAndEntropy(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		token, hash, hint, err := Generate()
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(token, Prefix))
		assert.Len(t, token, TokenLength)
		assert.True(t, WellFormed(token))
		assert.Equal(t, Hash(token), hash)
		assert.Len(t, hash, 64)
		assert.Equal(t, token[len(token)-4:], hint)
		assert.False(t, seen[token], "tokens are unique")
		seen[token] = true
	}
	assert.False(t, WellFormed(""))
	assert.False(t, WellFormed(Prefix))
	assert.False(t, WellFormed(strings.Repeat("a", TokenLength)))
	assert.False(t, WellFormed(Prefix+strings.Repeat("a", 42)+"!"))
	assert.False(t, WellFormed(Prefix+strings.Repeat("a", 44)))
	assert.True(t, LooksLikeToken("  "+Prefix+"x"))
	assert.False(t, LooksLikeToken("eyJhbGciOi"))
}

func TestCreateStoresOnlyTheHash(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, _ := newService(t, db)
		token, row := mustCreate(t, s, 7, model.AdminAPITokenScopeAdmin, nil)

		// No column of the token row, and no audit entry, holds the token.
		var raw []map[string]any
		require.NoError(t, db.Table("v4_kernel_admin_api_token").Find(&raw).Error)
		require.Len(t, raw, 1)
		encoded, err := json.Marshal(raw)
		require.NoError(t, err)
		assert.NotContains(t, string(encoded), token)
		assert.Equal(t, Hash(token), raw[0]["token_hash"])
		assert.NotContains(t, string(encoded), token[len(Prefix):len(Prefix)+12], "no part of the secret is kept")

		var logs []model.OperationLog
		require.NoError(t, db.Find(&logs).Error)
		require.Len(t, logs, 1)
		assert.Equal(t, AuditActionCreate, logs[0].Action)
		assert.Equal(t, AuditModule, logs[0].Module)
		assert.Equal(t, "root@example.com", logs[0].Username)
		assert.NotContains(t, logs[0].Content, token)
		assert.NotContains(t, logs[0].Content, Hash(token))
		assert.Contains(t, logs[0].Content, row.ID)

		// The record that is listed and serialized has no secret either.
		owner := uint(7)
		listed, err := s.List(context.Background(), ListFilter{UserID: &owner})
		require.NoError(t, err)
		require.Len(t, listed, 1)
		listedJSON, err := json.Marshal(listed)
		require.NoError(t, err)
		assert.NotContains(t, string(listedJSON), token)
		assert.NotContains(t, string(listedJSON), Hash(token))
		assert.NotContains(t, string(listedJSON), "token_hash")
		assert.Equal(t, token[len(token)-4:], listed[0].Hint)
	})
}

func TestNameLimitCountsCharactersNotBytes(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, _ := newService(t, db)
		ctx := context.Background()
		// 100 characters of 3 bytes each is 300 bytes and must pass; 101 must not.
		_, _, err := s.Create(ctx, CreateInput{UserID: 7, Name: strings.Repeat("令", MaxNameLength), Scope: "read"})
		require.NoError(t, err)
		_, _, err = s.Create(ctx, CreateInput{UserID: 7, Name: strings.Repeat("令", MaxNameLength+1), Scope: "read"})
		assert.ErrorIs(t, err, ErrInvalidRequest)
	})
}

func TestListCarriesTheOwnersEmail(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, _ := newService(t, db)
		ctx := context.Background()
		_, _, err := s.Create(ctx, CreateInput{UserID: 7, Name: "ci", Scope: "read"})
		require.NoError(t, err)
		var owner model.User
		require.NoError(t, db.Select("id", "email").Where("id = ?", 7).Take(&owner).Error)
		require.NotEmpty(t, owner.Email)
		listed, err := s.List(ctx, ListFilter{})
		require.NoError(t, err)
		require.NotEmpty(t, listed)
		for _, row := range listed {
			if row.UserID == 7 {
				assert.Equal(t, owner.Email, row.OwnerEmail)
			}
		}
		raw, err := json.Marshal(listed[0])
		require.NoError(t, err)
		assert.Contains(t, string(raw), `"owner_email"`)
	})
}

func TestCreateValidation(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, c := newService(t, db)
		ctx := context.Background()
		past, soon, far := c.now.Add(-time.Hour), c.now.Add(time.Hour), c.now.Add(MaxLifetime+time.Hour)
		for name, input := range map[string]CreateInput{
			"empty name":      {UserID: 7, Name: "  ", Scope: "read"},
			"long name":       {UserID: 7, Name: strings.Repeat("n", MaxNameLength+1), Scope: "read"},
			"control char":    {UserID: 7, Name: "a\nb", Scope: "read"},
			"unknown scope":   {UserID: 7, Name: "ci", Scope: "write"},
			"empty scope":     {UserID: 7, Name: "ci"},
			"expired already": {UserID: 7, Name: "ci", Scope: "read", ExpiresAt: &past},
			"too far":         {UserID: 7, Name: "ci", Scope: "read", ExpiresAt: &far},
		} {
			_, _, err := s.Create(ctx, input)
			assert.ErrorIs(t, err, ErrInvalidRequest, name)
		}
		for name, user := range map[string]uint{"member": 9, "missing": 4242} {
			_, _, err := s.Create(ctx, CreateInput{UserID: user, Name: "ci", Scope: "read"})
			assert.ErrorIs(t, err, ErrOwnerNotAdmin, name)
		}
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 8).Update("banned", 1).Error)
		_, _, err := s.Create(ctx, CreateInput{UserID: 8, Name: "ci", Scope: "read"})
		assert.ErrorIs(t, err, ErrOwnerNotAdmin, "banned")

		_, _, err = s.Create(ctx, CreateInput{UserID: 7, Name: "ci", Scope: "read", ExpiresAt: &soon})
		require.NoError(t, err)
		var count int64
		require.NoError(t, db.Model(&model.AdminAPIToken{}).Count(&count).Error)
		assert.EqualValues(t, 1, count, "refusals store nothing")
	})
}

func TestCreateLimit(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, c := newService(t, db)
		expiring := c.now.Add(time.Hour)
		for i := range MaxActivePerUser {
			var expires *time.Time
			if i == 0 {
				expires = &expiring
			}
			mustCreate(t, s, 7, "read", expires)
		}
		_, _, err := s.Create(context.Background(), CreateInput{UserID: 7, Name: "one more", Scope: "read"})
		assert.ErrorIs(t, err, ErrTooMany)
		// Another administrator has their own allowance, and an expired
		// token does not count.
		_, _, err = s.Create(context.Background(), CreateInput{UserID: 8, Name: "ci", Scope: "read"})
		require.NoError(t, err)
		c.now = c.now.Add(2 * time.Hour)
		_, _, err = s.Create(context.Background(), CreateInput{UserID: 7, Name: "after expiry", Scope: "read"})
		require.NoError(t, err)
	})
}

func authErr(t *testing.T, s *Service, token string) *Denial {
	t.Helper()
	_, err := s.Authenticate(context.Background(), token, "192.0.2.1")
	require.Error(t, err)
	var denial *Denial
	require.True(t, errors.As(err, &denial), "a refusal is a Denial: %v", err)
	return denial
}

func TestAuthenticate(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, _ := newService(t, db)
		token, row := mustCreate(t, s, 7, model.AdminAPITokenScopeRead, nil)
		principal, err := s.Authenticate(context.Background(), token, "192.0.2.1")
		require.NoError(t, err)
		assert.Equal(t, row.ID, principal.TokenID)
		assert.EqualValues(t, 7, principal.UserID)
		assert.Equal(t, "root@example.com", principal.Email)
		assert.Equal(t, model.AdminAPITokenScopeRead, principal.Scope)

		// What the lookup keys on is not accepted as a token, and a token
		// that is nearly right is no token.
		assert.ErrorIs(t, authErr(t, s, Hash(token)), ErrInvalid)
		assert.ErrorIs(t, authErr(t, s, Prefix+Hash(token)[:43]), ErrInvalid)
		flipped := []byte(token)
		if flipped[len(flipped)-1] == 'A' {
			flipped[len(flipped)-1] = 'B'
		} else {
			flipped[len(flipped)-1] = 'A'
		}
		denial := authErr(t, s, string(flipped))
		assert.ErrorIs(t, denial, ErrInvalid)
		assert.Empty(t, denial.TokenID, "an unknown token names no token")
		assert.ErrorIs(t, authErr(t, s, ""), ErrInvalid)
		assert.ErrorIs(t, authErr(t, s, "not-a-token"), ErrInvalid)
	})
}

func TestAuthenticateExpiry(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, c := newService(t, db)
		expires := c.now.Add(time.Hour)
		token, row := mustCreate(t, s, 7, "admin", &expires)
		_, err := s.Authenticate(context.Background(), token, "")
		require.NoError(t, err)
		c.now = expires.Add(-time.Second)
		_, err = s.Authenticate(context.Background(), token, "")
		require.NoError(t, err)
		c.now = expires
		denial := authErr(t, s, token)
		assert.ErrorIs(t, denial, ErrExpired)
		assert.Equal(t, row.ID, denial.TokenID)
		assert.EqualValues(t, 7, denial.UserID)
	})
}

func TestAuthenticateOwnerStates(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, _ := newService(t, db)
		ctx := context.Background()
		banned, bannedRow := mustCreate(t, s, 7, "admin", nil)
		demoted, demotedRow := mustCreate(t, s, 8, "admin", nil)
		// A throwaway administrator, to delete.
		require.NoError(t, db.Create(&model.User{ID: 10, Email: "gone@example.com", Token: "t10", UUID: "u10", IsAdmin: 1}).Error)
		deleted, deletedRow := mustCreate(t, s, 10, "admin", nil)

		// Banning the owner stops the token at once.
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 7).Update("banned", 1).Error)
		denial := authErr(t, s, banned)
		assert.ErrorIs(t, denial, ErrOwnerDenied)
		assert.Equal(t, bannedRow.ID, denial.TokenID)
		assert.Equal(t, "owner banned", denial.Detail)

		// Unbanning does not bring the token back: it was revoked when it
		// was refused.
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 7).Update("banned", 0).Error)
		assert.ErrorIs(t, authErr(t, s, banned), ErrRevoked)
		var stored model.AdminAPIToken
		require.NoError(t, db.First(&stored, "id = ?", bannedRow.ID).Error)
		require.NotNil(t, stored.RevokedAt)
		assert.Equal(t, "owner_not_admin", stored.RevokeReason)

		// Demotion.
		_, err := s.Authenticate(ctx, demoted, "")
		require.NoError(t, err)
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 8).Update("is_admin", 0).Error)
		denial = authErr(t, s, demoted)
		assert.ErrorIs(t, denial, ErrOwnerDenied)
		assert.Equal(t, demotedRow.ID, denial.TokenID)
		assert.Equal(t, "owner not an administrator", denial.Detail)
		require.NoError(t, db.Model(&model.User{}).Where("id = ?", 8).Update("is_admin", 1).Error)
		assert.ErrorIs(t, authErr(t, s, demoted), ErrRevoked, "promoting again does not revive it")

		// Deletion.
		require.NoError(t, db.Delete(&model.User{}, 10).Error)
		denial = authErr(t, s, deleted)
		assert.ErrorIs(t, denial, ErrOwnerDenied)
		assert.Equal(t, deletedRow.ID, denial.TokenID)
		assert.Equal(t, "owner deleted", denial.Detail)
	})
}

func TestRevoke(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, _ := newService(t, db)
		ctx := context.Background()
		mine, mineRow := mustCreate(t, s, 7, "admin", nil)
		other, otherRow := mustCreate(t, s, 8, "read", nil)

		// An administrator cannot see or end another's token, and cannot tell
		// whether it exists.
		_, _, err := s.Revoke(ctx, otherRow.ID, Actor{UserID: 7, Name: "root@example.com"})
		assert.ErrorIs(t, err, ErrNotFound)
		_, _, err = s.Revoke(ctx, "no-such-id", Actor{UserID: 7})
		assert.ErrorIs(t, err, ErrNotFound)
		_, err = s.Authenticate(ctx, other, "")
		require.NoError(t, err)

		row, changed, err := s.Revoke(ctx, mineRow.ID, Actor{UserID: 7, Name: "root@example.com", IP: "192.0.2.9"})
		require.NoError(t, err)
		assert.True(t, changed)
		require.NotNil(t, row.RevokedAt)
		assert.Equal(t, "owner_revoked", row.RevokeReason)
		assert.ErrorIs(t, authErr(t, s, mine), ErrRevoked)

		// Revoking again changes nothing and writes no second entry.
		_, changed, err = s.Revoke(ctx, mineRow.ID, Actor{UserID: 7})
		require.NoError(t, err)
		assert.False(t, changed)
		var revocations int64
		require.NoError(t, db.Model(&model.OperationLog{}).Where("action = ?", AuditActionRevoke).Count(&revocations).Error)
		assert.EqualValues(t, 1, revocations)

		// A super administrator ends anyone's.
		row, changed, err = s.Revoke(ctx, otherRow.ID, Actor{UserID: 7, Name: "root@example.com", Super: true})
		require.NoError(t, err)
		assert.True(t, changed)
		assert.Equal(t, "admin_revoked", row.RevokeReason)
		assert.ErrorIs(t, authErr(t, s, other), ErrRevoked)

		var entry model.OperationLog
		require.NoError(t, db.Where("action = ?", AuditActionRevoke).Order("id DESC").First(&entry).Error)
		assert.NotContains(t, entry.Content, mine)
		assert.NotContains(t, entry.Content, other)
		assert.Contains(t, entry.Content, "admin_revoked")
	})
}

func TestListFilters(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, c := newService(t, db)
		ctx := context.Background()
		expiring := c.now.Add(time.Hour)
		_, active := mustCreate(t, s, 7, "read", nil)
		_, revoked := mustCreate(t, s, 7, "read", nil)
		_, expired := mustCreate(t, s, 7, "read", &expiring)
		_, others := mustCreate(t, s, 8, "admin", nil)
		_, _, err := s.Revoke(ctx, revoked.ID, Actor{UserID: 7})
		require.NoError(t, err)
		c.now = c.now.Add(2 * time.Hour)

		ids := func(filter ListFilter) []string {
			rows, err := s.List(ctx, filter)
			require.NoError(t, err)
			out := []string{}
			for _, row := range rows {
				out = append(out, row.ID)
			}
			return out
		}
		owner := uint(7)
		assert.ElementsMatch(t, []string{active.ID}, ids(ListFilter{UserID: &owner}))
		assert.ElementsMatch(t, []string{active.ID, revoked.ID, expired.ID}, ids(ListFilter{UserID: &owner, IncludeInactive: true}))
		assert.ElementsMatch(t, []string{active.ID, others.ID}, ids(ListFilter{}))
	})
}

func TestLastUsedIsThrottled(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, c := newService(t, db)
		token, row := mustCreate(t, s, 7, "read", nil)
		lastUsed := func() (*time.Time, string) {
			var stored model.AdminAPIToken
			require.NoError(t, db.First(&stored, "id = ?", row.ID).Error)
			return stored.LastUsedAt, stored.LastUsedIP
		}
		at, _ := lastUsed()
		assert.Nil(t, at, "never used")

		_, err := s.Authenticate(context.Background(), token, "192.0.2.1")
		require.NoError(t, err)
		first, ip := lastUsed()
		require.NotNil(t, first)
		assert.Equal(t, "192.0.2.1", ip)

		// Uses within the interval write nothing.
		c.now = c.now.Add(lastUsedInterval - time.Second)
		_, err = s.Authenticate(context.Background(), token, "192.0.2.2")
		require.NoError(t, err)
		second, ip := lastUsed()
		assert.True(t, first.Equal(*second), "no write inside the interval")
		assert.Equal(t, "192.0.2.1", ip)

		c.now = c.now.Add(2 * time.Second)
		_, err = s.Authenticate(context.Background(), token, "192.0.2.3")
		require.NoError(t, err)
		third, ip := lastUsed()
		assert.True(t, third.After(*first))
		assert.Equal(t, "192.0.2.3", ip)
	})
}

func TestDeniedAuditIsThrottledAndCarriesNoSecret(t *testing.T) {
	forEachDatabase(t, func(t *testing.T, db *gorm.DB) {
		s, c := newService(t, db)
		ctx := context.Background()
		token, row := mustCreate(t, s, 7, "read", nil)
		event := Event{TokenID: row.ID, UserID: 7, IP: "192.0.2.5", Method: "POST", Path: "/api/v2/admin/users", Reason: "scope_read_only"}
		for range 5 {
			s.RecordDenied(ctx, event)
		}
		s.RecordDenied(ctx, Event{IP: "192.0.2.6", Method: "GET", Path: "/api/v3/plugins", Reason: "unknown"})
		s.RecordDenied(ctx, Event{IP: "192.0.2.6", Method: "GET", Path: "/api/v3/plugins", Reason: "unknown"})
		var denied []model.OperationLog
		require.NoError(t, db.Where("action = ?", AuditActionUseDenied).Find(&denied).Error)
		assert.Len(t, denied, 2, "one entry per subject and reason a minute")

		c.now = c.now.Add(deniedAuditInterval)
		s.RecordDenied(ctx, event)
		require.NoError(t, db.Where("action = ?", AuditActionUseDenied).Find(&denied).Error)
		assert.Len(t, denied, 3)
		for _, entry := range denied {
			assert.NotContains(t, entry.Content, token)
			assert.Equal(t, 2, entry.Status)
		}
		assert.Contains(t, denied[0].Content, row.ID)
		assert.Contains(t, denied[0].Content, "/api/v2/admin/users")

		s.RecordUse(ctx, Event{TokenID: row.ID, UserID: 7, Actor: "root@example.com", IP: "192.0.2.5", Method: "POST", Path: "/api/v2/admin/users", Status: 200})
		var use model.OperationLog
		require.NoError(t, db.Where("action = ?", AuditActionUse).First(&use).Error)
		assert.Equal(t, 1, use.Status)
		assert.Contains(t, use.Content, `"status_code":200`)
	})
}
