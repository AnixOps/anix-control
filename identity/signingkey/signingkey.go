// Package signingkey manages the Ed25519 keys that sign identity tokens:
// generation, encryption at rest under a key-encryption key, and rotation.
//
// A key is published (as NEXT) before it signs, so verifiers that refresh
// their key set periodically know it in time; it then signs (ACTIVE); after
// the next rotation it stops signing but still verifies (RETIRED) until every
// token it signed has expired; then it is dropped. A compromised key is
// REVOKED: it verifies nothing, and its users must log in again.
package signingkey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

// State is a key's place in its lifecycle.
type State string

const (
	StateNext    State = "next"
	StateActive  State = "active"
	StateRetired State = "retired"
	StateRevoked State = "revoked"
)

// Key is one signing key. PrivateKey is nil when only the public half is
// known (for example after loading published keys).
type Key struct {
	ID         string
	State      State
	PublicKey  ed25519.PublicKey
	PrivateKey ed25519.PrivateKey
	CreatedAt  time.Time
	// ActivatedAt is when the key started signing; zero for NEXT keys.
	ActivatedAt time.Time
	// RetiredAt is when the key stopped signing; zero unless RETIRED.
	RetiredAt time.Time
}

// Generate creates a NEXT key.
func Generate(now time.Time) (Key, error) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return Key{}, err
	}
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return Key{}, err
	}
	return Key{
		ID:    "idk-" + now.UTC().Format("2006-01-02") + "-" + hex.EncodeToString(suffix),
		State: StateNext, PublicKey: public, PrivateKey: private, CreatedAt: now,
	}, nil
}

// Policy decides when keys change state.
type Policy struct {
	// RotateAfter is how long a key signs before the next one takes over.
	RotateAfter time.Duration
	// PublishAhead is how long a key is published before it signs; it must
	// exceed the verifiers' key refresh interval.
	PublishAhead time.Duration
	// VerifyAfterRetire is how long a retired key keeps verifying: the
	// longest token lifetime plus the verifiers' leeway.
	VerifyAfterRetire time.Duration
}

// Validate refuses policies that would let a token outlive its key.
func (p Policy) Validate() error {
	if p.RotateAfter <= 0 || p.PublishAhead <= 0 || p.VerifyAfterRetire <= 0 {
		return errors.New("signing key policy durations must be positive")
	}
	if p.PublishAhead >= p.RotateAfter {
		return errors.New("signing keys must be published for less time than they sign")
	}
	return nil
}

// NotAfter is when a key stops verifying; zero while it has no end yet.
func (p Policy) NotAfter(key Key) time.Time {
	if key.State != StateRetired {
		return time.Time{}
	}
	return key.RetiredAt.Add(p.VerifyAfterRetire)
}

// Advance applies the transitions due at now and returns the new key set and
// whether it changed. generate creates a NEXT key when one is needed.
//   - With no usable key, a new key is activated at once (first start, or
//     every key revoked); verifiers learn it on their next refresh or when
//     they meet its kid.
//   - A NEXT key is created PublishAhead before the active key's term ends.
//   - The NEXT key is activated once the active key's term ended and the
//     NEXT key was published for PublishAhead; the old key is retired.
//   - Retired keys past VerifyAfterRetire are dropped; revoked keys are kept.
func (p Policy) Advance(keys []Key, now time.Time, generate func(time.Time) (Key, error)) ([]Key, bool, error) {
	if err := p.Validate(); err != nil {
		return nil, false, err
	}
	next := append([]Key(nil), keys...)
	changed := false

	kept := next[:0]
	for _, key := range next {
		if key.State == StateRetired && !now.Before(p.NotAfter(key)) {
			changed = true
			continue
		}
		kept = append(kept, key)
	}
	next = kept

	active, pending := -1, -1
	for index, key := range next {
		switch key.State {
		case StateActive:
			if active < 0 || key.ActivatedAt.After(next[active].ActivatedAt) {
				active = index
			}
		case StateNext:
			if pending < 0 || key.CreatedAt.Before(next[pending].CreatedAt) {
				pending = index
			}
		}
	}

	if active < 0 {
		if pending < 0 {
			key, err := generate(now)
			if err != nil {
				return nil, false, err
			}
			next = append(next, key)
			pending = len(next) - 1
		}
		next[pending].State, next[pending].ActivatedAt = StateActive, now
		return sortKeys(next), true, nil
	}

	termEnds := next[active].ActivatedAt.Add(p.RotateAfter)
	if pending < 0 && !now.Before(termEnds.Add(-p.PublishAhead)) {
		key, err := generate(now)
		if err != nil {
			return nil, false, err
		}
		next = append(next, key)
		pending = len(next) - 1
		changed = true
	}
	if pending >= 0 && !now.Before(termEnds) && !now.Before(next[pending].CreatedAt.Add(p.PublishAhead)) {
		next[active].State, next[active].RetiredAt = StateRetired, now
		next[pending].State, next[pending].ActivatedAt = StateActive, now
		changed = true
	}
	return sortKeys(next), changed, nil
}

// Signing returns the key that signs now.
func Signing(keys []Key) (Key, error) {
	var signing *Key
	for index := range keys {
		key := &keys[index]
		if key.State == StateActive && key.PrivateKey != nil && (signing == nil || key.ActivatedAt.After(signing.ActivatedAt)) {
			signing = key
		}
	}
	if signing == nil {
		return Key{}, errors.New("no active signing key")
	}
	return *signing, nil
}

// Revoke marks a key revoked; tokens it signed stop verifying.
func Revoke(keys []Key, id string) ([]Key, error) {
	next := append([]Key(nil), keys...)
	for index := range next {
		if next[index].ID == id {
			next[index].State = StateRevoked
			return next, nil
		}
	}
	return nil, fmt.Errorf("signing key %q not found", id)
}

func sortKeys(keys []Key) []Key {
	sort.SliceStable(keys, func(i, j int) bool { return keys[i].CreatedAt.Before(keys[j].CreatedAt) })
	return keys
}

// Sealer encrypts private keys at rest with AES-256-GCM under a 32-byte
// key-encryption key, so replicas sharing the database share the keys but a
// database copy alone does not reveal them.
type Sealer struct {
	aead cipher.AEAD
}

// ParseKEK decodes a 32-byte key given as base64 or hex.
func ParseKEK(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	for _, decode := range []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString, base64.RawStdEncoding.DecodeString, base64.RawURLEncoding.DecodeString, hex.DecodeString,
	} {
		if raw, err := decode(value); err == nil && len(raw) == 32 {
			return raw, nil
		}
	}
	return nil, errors.New("the key-encryption key must be 32 bytes, base64 or hex")
}

// NewSealer returns a sealer for a 32-byte key-encryption key.
func NewSealer(kek []byte) (*Sealer, error) {
	if len(kek) != 32 {
		return nil, errors.New("the key-encryption key must be 32 bytes")
	}
	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{aead: aead}, nil
}

// Seal encrypts a private key; the key id is bound as associated data, so a
// sealed key cannot be moved to another key's row.
func (s *Sealer) Seal(id string, private ed25519.PrivateKey) (string, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := s.aead.Seal(nonce, nonce, private.Seed(), []byte(id))
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open decrypts a private key sealed for id.
func (s *Sealer) Open(id, sealed string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < s.aead.NonceSize() {
		return nil, errors.New("sealed signing key is malformed")
	}
	nonce, ciphertext := raw[:s.aead.NonceSize()], raw[s.aead.NonceSize():]
	seed, err := s.aead.Open(nil, nonce, ciphertext, []byte(id))
	if err != nil {
		return nil, fmt.Errorf("open signing key %q: %w", id, err)
	}
	if len(seed) != ed25519.SeedSize {
		return nil, errors.New("sealed signing key has the wrong size")
	}
	return ed25519.NewKeyFromSeed(seed), nil
}
