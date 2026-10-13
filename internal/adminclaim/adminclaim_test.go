package adminclaim

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/database"
	"github.com/AnixOps/anix-control/v5/internal/database/migrations"
	"github.com/AnixOps/anix-control/v5/internal/identityimport"
	"github.com/AnixOps/anix-control/v5/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// openDB is a migrated SQLite database opened as Control opens it (one
// writer, immediate transactions).
func openDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Open(&config.DatabaseConfig{Driver: "sqlite", Database: filepath.Join(t.TempDir(), "control.db")})
	require.NoError(t, err)
	t.Cleanup(func() { _ = database.CloseHandle(db) })
	_, err = migrations.Migrate(context.Background(), db, migrations.Options{ProgramVersion: "4.2.1"})
	require.NoError(t, err)
	return db
}

// captureLogs sends the standard logger and slog into one buffer for the
// test.
func captureLogs(t *testing.T) *lockedBuffer {
	t.Helper()
	buffer := &lockedBuffer{}
	previousWriter, previousFlags := log.Writer(), log.Flags()
	previousSlog := slog.Default()
	log.SetOutput(buffer)
	slog.SetDefault(slog.New(slog.NewTextHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
		slog.SetDefault(previousSlog)
	})
	return buffer
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// dump is every value of every table, as text.
func dump(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var tables []string
	require.NoError(t, db.Raw("SELECT name FROM sqlite_master WHERE type = 'table'").Scan(&tables).Error)
	var out strings.Builder
	for _, table := range tables {
		var rows []map[string]any
		require.NoError(t, db.Table(table).Find(&rows).Error)
		for _, row := range rows {
			for column, value := range row {
				fmt.Fprintf(&out, "%s.%s=%v\n", table, column, value)
			}
		}
	}
	return out.String()
}

func metaValue(t *testing.T, db *gorm.DB) (pending, bool) {
	t.Helper()
	var entries []migrations.MetaEntry
	require.NoError(t, db.Where("key = ?", MetaKey).Find(&entries).Error)
	if len(entries) == 0 {
		return pending{}, false
	}
	var record pending
	require.NoError(t, json.Unmarshal([]byte(entries[0].Value), &record))
	return record, true
}

func admins(t *testing.T, db *gorm.DB) []model.User {
	t.Helper()
	var users []model.User
	require.NoError(t, db.Where("is_admin = ?", 1).Find(&users).Error)
	return users
}

var issuedAt = time.Date(2026, 10, 13, 8, 0, 0, 0, time.UTC)

func fixedClock(at time.Time) func() time.Time { return func() time.Time { return at } }

func TestNewTokenFormatAndEntropy(t *testing.T) {
	random := bytes.NewReader(bytes.Repeat([]byte{0xfb}, 32))
	token, err := NewToken(random)
	require.NoError(t, err)
	assert.Equal(t, TokenPrefix+base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0xfb}, 32)), token)
	assert.Len(t, token, len("anixclaim_")+43)
	assert.True(t, WellFormed(token))
	assert.NotContains(t, token, "=", "unpadded")
	assert.NotContains(t, token, "+")
	assert.NotContains(t, token, "/")

	_, err = NewToken(bytes.NewReader(make([]byte, 31)))
	require.Error(t, err, "a short read is an error, never a shorter token")

	seen := map[string]bool{}
	for range 2000 {
		issued, err := NewToken(cryptoReader{})
		require.NoError(t, err)
		require.True(t, WellFormed(issued))
		decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(issued, TokenPrefix))
		require.NoError(t, err)
		require.Len(t, decoded, 32, "256 bits")
		require.False(t, seen[issued], "a token repeats")
		seen[issued] = true
	}

	for _, malformed := range []string{
		"", "anixclaim_", strings.TrimPrefix(token, "anix"), "ANIXCLAIM_" + token[10:], token + "A", token[:len(token)-1],
		token[:len(token)-1] + "=", token[:20] + "+" + token[21:], token[:20] + "/" + token[21:],
	} {
		assert.False(t, WellFormed(malformed), malformed)
	}
}

func TestIssueStoresOnlyTheHashAndTheExpiry(t *testing.T) {
	logs := captureLogs(t)
	db := openDB(t)
	issued, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)
	assert.True(t, WellFormed(issued.Token))
	assert.Equal(t, issuedAt.Add(24*time.Hour), issued.ExpiresAt)

	record, ok := metaValue(t, db)
	require.True(t, ok)
	assert.Equal(t, Hash(issued.Token), record.SHA256)
	assert.Len(t, record.SHA256, 64)
	assert.Equal(t, issuedAt.Add(24*time.Hour), record.ExpiresAt.UTC())
	assert.Equal(t, issuedAt, record.CreatedAt.UTC())

	stored := dump(t, db)
	assert.Contains(t, stored, Hash(issued.Token))
	assert.NotContains(t, stored, issued.Token, "the token is in the database")
	assert.NotContains(t, stored, strings.TrimPrefix(issued.Token, TokenPrefix))
	assert.NotContains(t, logs.String(), issued.Token, "the token is in a log")

	for _, ttl := range []time.Duration{0, -time.Hour, 59 * time.Second, 24*time.Hour + time.Second, 7 * 24 * time.Hour} {
		_, err := Issue(context.Background(), db, ttl, issuedAt)
		assert.ErrorIs(t, err, ErrInvalidTTL, ttl.String())
	}
	_, err = Issue(context.Background(), db, time.Minute, issuedAt)
	assert.NoError(t, err)
}

func TestIssueRefusesOnceAnAdministratorExists(t *testing.T) {
	db := openDB(t)
	claimable, err := Claimable(context.Background(), db)
	require.NoError(t, err)
	assert.True(t, claimable)

	require.NoError(t, db.Create(&model.User{Email: "owner@example.com", Password: "x", UUID: "u-1", Token: "t-1", IsAdmin: 1}).Error)
	_, err = Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.ErrorIs(t, err, ErrAdministratorExists)
	_, ok := metaValue(t, db)
	assert.False(t, ok, "nothing is stored")
	claimable, err = Claimable(context.Background(), db)
	require.NoError(t, err)
	assert.False(t, claimable)

	// A user who is not an administrator does not count.
	other := openDB(t)
	require.NoError(t, other.Create(&model.User{Email: "member@example.com", Password: "x", UUID: "u-2", Token: "t-2"}).Error)
	_, err = Issue(context.Background(), other, DefaultTTL, issuedAt)
	require.NoError(t, err)
}

func TestClaimCreatesTheAdministratorAndConsumesTheToken(t *testing.T) {
	logs := captureLogs(t)
	db := openDB(t)
	issued, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)

	claimer := &Claimer{DB: db, Now: fixedClock(issuedAt.Add(23 * time.Hour)), Cost: bcrypt.MinCost}
	password := "first-owner-Pa55"
	result, err := claimer.Claim(context.Background(), Request{Token: issued.Token, Email: "  owner@example.com ", Password: password, SourceIP: "203.0.113.7"})
	require.NoError(t, err)
	assert.Equal(t, "owner@example.com", result.Email)
	assert.False(t, result.Identity)

	created := admins(t, db)
	require.Len(t, created, 1)
	assert.Equal(t, result.UserID, created[0].ID)
	assert.Equal(t, "owner@example.com", created[0].Email)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(created[0].Password), []byte(password)))
	assert.NotEmpty(t, created[0].UUID)
	assert.NotEmpty(t, created[0].Token)
	_, ok := metaValue(t, db)
	assert.False(t, ok, "the token is consumed")

	// The audit row: the action, the email and the source address.
	var rows []model.AuditLog
	require.NoError(t, db.Find(&rows).Error)
	require.Len(t, rows, 1)
	assert.Equal(t, AuditAction, rows[0].Action)
	assert.Equal(t, "owner@example.com", rows[0].Email)
	assert.Equal(t, "203.0.113.7", rows[0].IP)
	assert.Equal(t, AuditPath, rows[0].Path)
	assert.Empty(t, rows[0].RequestBody)
	assert.Empty(t, rows[0].UserAgent)
	assert.Empty(t, rows[0].ErrorMessage)

	// Once claimed, the same link and any other answer "administrator exists".
	_, err = claimer.Claim(context.Background(), Request{Token: issued.Token, Email: "second@example.com", Password: password})
	assert.ErrorIs(t, err, ErrAdministratorExists)
	other, err := NewToken(cryptoReader{})
	require.NoError(t, err)
	_, err = claimer.Claim(context.Background(), Request{Token: other, Email: "second@example.com", Password: password})
	assert.ErrorIs(t, err, ErrAdministratorExists)
	assert.Len(t, admins(t, db), 1)

	stored := dump(t, db)
	assert.NotContains(t, stored, issued.Token)
	assert.NotContains(t, stored, password)
	assert.NotContains(t, logs.String(), issued.Token)
	assert.NotContains(t, logs.String(), password)
}

func TestClaimOnlyWithThePendingToken(t *testing.T) {
	db := openDB(t)
	claimer := &Claimer{DB: db, Now: fixedClock(issuedAt.Add(time.Hour)), Cost: bcrypt.MinCost}
	request := func(token string) Request {
		return Request{Token: token, Email: "owner@example.com", Password: "secret-1"}
	}
	// Nothing issued yet.
	unknown, err := NewToken(cryptoReader{})
	require.NoError(t, err)
	_, err = claimer.Claim(context.Background(), request(unknown))
	require.ErrorIs(t, err, ErrInvalidToken)

	first, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)
	// Rotation: a new link invalidates the previous one.
	second, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)
	require.NotEqual(t, first.Token, second.Token)
	_, err = claimer.Claim(context.Background(), request(first.Token))
	require.ErrorIs(t, err, ErrInvalidToken, "the replaced link")
	for _, wrong := range []string{"", "anixclaim_", unknown, second.Token + "x", strings.ToUpper(second.Token), second.Token[:len(second.Token)-1]} {
		_, err = claimer.Claim(context.Background(), request(wrong))
		require.ErrorIs(t, err, ErrInvalidToken, wrong)
	}
	assert.Empty(t, admins(t, db))
	_, err = claimer.Claim(context.Background(), request(second.Token))
	require.NoError(t, err)
}

func TestClaimExpires(t *testing.T) {
	db := openDB(t)
	issued, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)
	late := &Claimer{DB: db, Now: fixedClock(issuedAt.Add(24 * time.Hour)), Cost: bcrypt.MinCost}
	_, err = late.Claim(context.Background(), Request{Token: issued.Token, Email: "owner@example.com", Password: "secret-1"})
	require.ErrorIs(t, err, ErrExpired)
	// Without the token, an expired claim looks like any wrong token.
	other, err := NewToken(cryptoReader{})
	require.NoError(t, err)
	_, err = late.Claim(context.Background(), Request{Token: other, Email: "owner@example.com", Password: "secret-1"})
	require.ErrorIs(t, err, ErrInvalidToken)
	assert.Empty(t, admins(t, db))

	// A new link works again.
	renewed, err := Issue(context.Background(), db, DefaultTTL, issuedAt.Add(24*time.Hour))
	require.NoError(t, err)
	_, err = late.Claim(context.Background(), Request{Token: renewed.Token, Email: "owner@example.com", Password: "secret-1"})
	require.NoError(t, err)
}

func TestClaimChecksTheEmailAndThePasswordAfterTheToken(t *testing.T) {
	db := openDB(t)
	issued, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)
	claimer := &Claimer{DB: db, Now: fixedClock(issuedAt), Cost: bcrypt.MinCost}
	claim := func(token, email, password string) error {
		_, err := claimer.Claim(context.Background(), Request{Token: token, Email: email, Password: password})
		return err
	}
	other, err := NewToken(cryptoReader{})
	require.NoError(t, err)
	// A wrong token never learns the password rule.
	assert.ErrorIs(t, claim(other, "not-an-email", "x"), ErrInvalidToken)

	assert.ErrorIs(t, claim(issued.Token, "", "secret-1"), ErrInvalidEmail)
	assert.ErrorIs(t, claim(issued.Token, "not-an-email", "secret-1"), ErrInvalidEmail)
	assert.ErrorIs(t, claim(issued.Token, "admin@localhost", "secret-1"), ErrInvalidEmail, "the sign-in refuses it")
	assert.ErrorIs(t, claim(issued.Token, "owner@example.com", ""), ErrWeakPassword)
	assert.ErrorIs(t, claim(issued.Token, "owner@example.com", "12345"), ErrWeakPassword)
	assert.ErrorIs(t, claim(issued.Token, "owner@example.com", strings.Repeat("a", 73)), ErrPasswordTooLong)
	// Six characters, counted as characters: 密码密码密码 is 18 bytes.
	require.NoError(t, CheckPassword("密码密码密码"))
	require.NoError(t, CheckPassword(strings.Repeat("a", 72)))

	require.NoError(t, db.Create(&model.User{Email: "member@example.com", Password: "x", UUID: "u-1", Token: "t-1"}).Error)
	assert.ErrorIs(t, claim(issued.Token, "member@example.com", "secret-1"), ErrEmailTaken)

	// None of that consumed the token.
	_, ok := metaValue(t, db)
	require.True(t, ok)
	require.NoError(t, claim(issued.Token, "owner@example.com", "123456"))
}

func TestConcurrentClaimsWithOneTokenCreateOneAdministrator(t *testing.T) {
	db := openDB(t)
	issued, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)
	claimer := &Claimer{DB: db, Now: fixedClock(issuedAt), Cost: bcrypt.MinCost}
	const racers = 12
	var (
		wg     sync.WaitGroup
		start  = make(chan struct{})
		errs   = make([]error, racers)
		emails = make([]string, racers)
	)
	for i := range racers {
		emails[i] = fmt.Sprintf("owner%d@example.com", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, errs[i] = claimer.Claim(context.Background(), Request{Token: issued.Token, Email: emails[i], Password: "secret-1"})
		}()
	}
	close(start)
	wg.Wait()
	winners := 0
	for i, err := range errs {
		if err == nil {
			winners++
			continue
		}
		assert.True(t, errors.Is(err, ErrAdministratorExists) || errors.Is(err, ErrInvalidToken), "racer %d: %v", i, err)
	}
	assert.Equal(t, 1, winners)
	assert.Len(t, admins(t, db), 1)
	var audits int64
	require.NoError(t, db.Model(&model.AuditLog{}).Where("action = ?", AuditAction).Count(&audits).Error)
	assert.Equal(t, int64(1), audits)
}

// fakeIdentity records the accounts handed to identity.
type fakeIdentity struct {
	mu       sync.Mutex
	accounts []model.User
	uuids    []string
	err      error
}

func (f *fakeIdentity) ImportAccount(_ context.Context, user model.User, accountUUID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.accounts = append(f.accounts, user)
	f.uuids = append(f.uuids, accountUUID)
	return nil
}

func setAuthority(t *testing.T, db *gorm.DB, state string) {
	t.Helper()
	require.NoError(t, db.Save(&model.IdentityAuthority{ID: 1, State: state, UpdatedAt: issuedAt}).Error)
}

func TestClaimThroughIdentityOnceItHoldsTheCredentials(t *testing.T) {
	for _, state := range []string{model.IdentityAuthorityIdentity, model.IdentityAuthorityFinalized} {
		t.Run(state, func(t *testing.T) {
			db := openDB(t)
			setAuthority(t, db, state)
			issued, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
			require.NoError(t, err)
			request := Request{Token: issued.Token, Email: "owner@example.com", Password: "secret-1", SourceIP: "198.51.100.4"}

			// Without identity nothing changes and the link keeps working.
			_, err = (&Claimer{DB: db, Now: fixedClock(issuedAt), Cost: bcrypt.MinCost}).Claim(context.Background(), request)
			require.ErrorIs(t, err, ErrIdentityUnavailable)
			assert.Empty(t, admins(t, db))

			// Identity refuses: the claim is undone and the token restored.
			refusing := &fakeIdentity{err: errors.New("identity is restarting")}
			_, err = (&Claimer{DB: db, Identity: refusing, Now: fixedClock(issuedAt), Cost: bcrypt.MinCost}).Claim(context.Background(), request)
			require.ErrorIs(t, err, ErrIdentityUnavailable)
			assert.Empty(t, admins(t, db))
			var links, audits int64
			require.NoError(t, db.Model(&model.IdentityAccountLink{}).Count(&links).Error)
			require.NoError(t, db.Model(&model.AuditLog{}).Count(&audits).Error)
			assert.Zero(t, links)
			assert.Zero(t, audits)
			record, ok := metaValue(t, db)
			require.True(t, ok, "the token is restored")
			assert.Equal(t, Hash(issued.Token), record.SHA256)

			identity := &fakeIdentity{}
			result, err := (&Claimer{DB: db, Identity: identity, Now: fixedClock(issuedAt), Cost: bcrypt.MinCost}).Claim(context.Background(), request)
			require.NoError(t, err)
			assert.True(t, result.Identity)
			require.Len(t, identity.accounts, 1)
			pushed := identity.accounts[0]
			assert.Equal(t, result.UserID, pushed.ID)
			assert.Equal(t, 1, pushed.IsAdmin)
			assert.Equal(t, "owner@example.com", pushed.Email)
			assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(pushed.Password), []byte("secret-1")), "identity gets the hash")
			assert.Equal(t, identityimport.AccountUUID(result.UserID), identity.uuids[0])

			created := admins(t, db)
			require.Len(t, created, 1)
			if state == model.IdentityAuthorityFinalized {
				assert.Equal(t, model.UnusableLegacyPassword, created[0].Password, "no legacy credential after finalize")
			} else {
				assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(created[0].Password), []byte("secret-1")), "mirrored until finalize")
			}
			var link model.IdentityAccountLink
			require.NoError(t, db.Where("user_id = ?", result.UserID).Take(&link).Error)
			assert.Equal(t, identity.uuids[0], link.AccountUUID)
			_, ok = metaValue(t, db)
			assert.False(t, ok)
		})
	}
}

func TestClaimWhileImportingStaysWithTheKernel(t *testing.T) {
	db := openDB(t)
	setAuthority(t, db, model.IdentityAuthorityImporting)
	issued, err := Issue(context.Background(), db, DefaultTTL, issuedAt)
	require.NoError(t, err)
	identity := &fakeIdentity{}
	result, err := (&Claimer{DB: db, Identity: identity, Now: fixedClock(issuedAt), Cost: bcrypt.MinCost}).Claim(context.Background(),
		Request{Token: issued.Token, Email: "owner@example.com", Password: "secret-1"})
	require.NoError(t, err)
	assert.False(t, result.Identity)
	assert.Empty(t, identity.accounts, "the next import copies it")
}

// cryptoReader is crypto/rand for NewToken.
type cryptoReader struct{}

func (cryptoReader) Read(p []byte) (int, error) { return crand.Read(p) }
