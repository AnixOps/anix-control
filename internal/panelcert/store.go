package panelcert

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/AnixOps/anix-control/v5/internal/model"
	"golang.org/x/crypto/acme/autocert"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Sealing of the entries that hold private keys: AES-256-GCM under a key
// derived with HKDF-SHA256 from the root key (module_runtime.ca_kek), the
// entry's key bound as additional data so that a sealed value copied to
// another key never opens.
const (
	sealInfo          = "anixops acme cache v1"
	sealAdditionalTag = "anixops acme cache v1:"
	kekSize           = 32
)

// Keys of Control's own entries. A DNS name never holds ':', so they never
// meet an autocert key.
const (
	statusKey          = "anixops:status"
	retryKey           = "anixops:retry"
	selfSignedKeyStart = "anixops:self-signed:"
)

// ErrSealed means an entry cannot be opened: the root key changed or the
// row was altered.
var ErrSealed = errors.New("panel certificate store: a sealed entry cannot be opened (module_runtime.ca_kek changed or the row was altered)")

// Store keeps the panel certificate entries in v5_acme_cache. With a root
// key every entry but the public ones (challenge answers, the status, a
// retry request) is sealed. Without one, the entries that hold a key stay in
// memory and do not survive a restart; the public ones still reach the
// table, so the command line sees the status.
type Store struct {
	db   *gorm.DB
	aead cipher.AEAD
	now  func() time.Time

	mu     sync.Mutex
	memory map[string][]byte
}

// NewStore returns the store on db sealed with kek (32 bytes). A nil db
// keeps every entry in memory, a nil kek the secret ones.
func NewStore(db *gorm.DB, kek []byte, now func() time.Time) (*Store, error) {
	if now == nil {
		now = time.Now
	}
	store := &Store{db: db, now: now, memory: map[string][]byte{}}
	if db == nil || kek == nil {
		return store, nil
	}
	if len(kek) != kekSize {
		return nil, errors.New("panel certificate store: the root key must be 32 bytes")
	}
	key, err := hkdf.Key(sha256.New, kek, nil, sealInfo, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	store.aead = aead
	return store, nil
}

// Persistent tells whether the certificates and keys survive a restart.
func (s *Store) Persistent() bool { return s.db != nil && s.aead != nil }

// public tells which entries are stored in the clear: the HTTP-01 answers
// (the CA fetches them over plain http anyway) and Control's own status
// rows.
func public(key string) bool {
	return strings.HasSuffix(key, "+http-01") || key == statusKey || key == retryKey
}

// inMemory tells whether key stays in memory.
func (s *Store) inMemory(key string) bool {
	return s.db == nil || (s.aead == nil && !public(key))
}

// Get returns an entry, autocert.ErrCacheMiss when there is none.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	if s.inMemory(key) {
		s.mu.Lock()
		defer s.mu.Unlock()
		data, ok := s.memory[key]
		if !ok {
			return nil, autocert.ErrCacheMiss
		}
		return append([]byte(nil), data...), nil
	}
	var rows []model.ACMECacheEntry
	if err := s.db.WithContext(ctx).Where(&model.ACMECacheEntry{Key: key}).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, autocert.ErrCacheMiss
	}
	if !rows[0].Sealed {
		return rows[0].Data, nil
	}
	if s.aead == nil {
		return nil, ErrSealed
	}
	return s.open(key, rows[0].Data)
}

// Put stores an entry, sealed unless it is public.
func (s *Store) Put(ctx context.Context, key string, data []byte) error {
	if s.inMemory(key) {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.memory[key] = append([]byte(nil), data...)
		return nil
	}
	row := model.ACMECacheEntry{Key: key, Data: data, UpdatedAt: s.now().UTC()}
	if !public(key) {
		sealed, err := s.seal(key, data)
		if err != nil {
			return err
		}
		row.Data, row.Sealed = sealed, true
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "cache_key"}},
		DoUpdates: clause.AssignmentColumns([]string{"data", "sealed", "updated_at"}),
	}).Create(&row).Error
}

// Delete removes an entry; a missing one is not an error.
func (s *Store) Delete(ctx context.Context, key string) error {
	if s.inMemory(key) {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.memory, key)
		return nil
	}
	return s.db.WithContext(ctx).Where(&model.ACMECacheEntry{Key: key}).Delete(&model.ACMECacheEntry{}).Error
}

func (s *Store) seal(key string, data []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, data, []byte(sealAdditionalTag+key)), nil
}

func (s *Store) open(key string, sealed []byte) ([]byte, error) {
	if len(sealed) < s.aead.NonceSize() {
		return nil, ErrSealed
	}
	nonce, ciphertext := sealed[:s.aead.NonceSize()], sealed[s.aead.NonceSize():]
	plain, err := s.aead.Open(nil, nonce, ciphertext, []byte(sealAdditionalTag+key))
	if err != nil {
		return nil, ErrSealed
	}
	return plain, nil
}

// directoryPrefix is the namespace of one ACME directory's entries: an
// account, certificates and challenge answers of the staging directory
// never stand in for the production directory's.
func directoryPrefix(directoryURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(directoryURL)))
	return "acme-" + hex.EncodeToString(sum[:6]) + "/"
}

// directoryCache is the autocert.Cache of one ACME directory on the store.
// While an issuance forces a name, the name's certificate entries miss, so
// that a fresh autocert.Manager requests a new certificate instead of
// handing back the one it would find.
type directoryCache struct {
	store  *Store
	prefix string

	mu     sync.Mutex
	forced map[string]bool
}

func newDirectoryCache(store *Store, directoryURL string) *directoryCache {
	return &directoryCache{store: store, prefix: directoryPrefix(directoryURL)}
}

// force makes the certificate entries of name miss until release.
func (c *directoryCache) force(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forced = map[string]bool{name: true, name + "+rsa": true}
}

func (c *directoryCache) release() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.forced = nil
}

func (c *directoryCache) missing(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.forced[key]
}

func (c *directoryCache) Get(ctx context.Context, key string) ([]byte, error) {
	if c.missing(key) {
		return nil, autocert.ErrCacheMiss
	}
	return c.store.Get(ctx, c.prefix+key)
}

func (c *directoryCache) Put(ctx context.Context, key string, data []byte) error {
	return c.store.Put(ctx, c.prefix+key, data)
}

func (c *directoryCache) Delete(ctx context.Context, key string) error {
	return c.store.Delete(ctx, c.prefix+key)
}

// certificateKey is the store key of name's certificate under directoryURL
// (the key autocert uses for an ECDSA certificate).
func certificateKey(directoryURL, name string) string {
	return directoryPrefix(directoryURL) + name
}
