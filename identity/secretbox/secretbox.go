// Package secretbox protects identity secrets at rest with the identity
// key-encryption key: AES-256-GCM for secrets identity must read back (TOTP
// seeds), and a keyed hash for secrets it only compares (backup codes), so a
// copy of the database alone reveals neither.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

// Box seals and hashes with one KEK. The associated data names the owner of
// a secret, so a sealed value cannot be moved to another row.
type Box struct {
	aead    cipher.AEAD
	hashKey []byte
}

// New returns a box for a 32-byte key-encryption key.
func New(kek []byte) (*Box, error) {
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
	mac := hmac.New(sha256.New, kek)
	_, _ = mac.Write([]byte("anixops-identity secret hash key v1"))
	return &Box{aead: aead, hashKey: mac.Sum(nil)}, nil
}

// Seal encrypts plaintext for owner.
func (b *Box) Seal(owner string, plaintext []byte) (string, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b.aead.Seal(nonce, nonce, plaintext, []byte(owner))), nil
}

// Open decrypts a value sealed for owner.
func (b *Box) Open(owner, sealed string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < b.aead.NonceSize() {
		return nil, errors.New("sealed secret is malformed")
	}
	plaintext, err := b.aead.Open(nil, raw[:b.aead.NonceSize()], raw[b.aead.NonceSize():], []byte(owner))
	if err != nil {
		return nil, errors.New("sealed secret does not open for its owner")
	}
	return plaintext, nil
}

// Hash returns the keyed hash of value for owner, hex encoded.
func (b *Box) Hash(owner, value string) string {
	mac := hmac.New(sha256.New, b.hashKey)
	_, _ = mac.Write([]byte(owner))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

// Equal compares two hashes in constant time.
func Equal(a, b string) bool {
	return hmac.Equal([]byte(a), []byte(b))
}
