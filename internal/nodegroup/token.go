package nodegroup

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/AnixOps/anix-control/sdk/agentcontrol"
	"github.com/AnixOps/anix-control/v5/internal/config"
	"github.com/AnixOps/anix-control/v5/internal/modulepki"
)

// Join tokens: anixjoin_ and the base64url encoding (no padding) of 32
// random bytes. Control keeps the token's SHA-256, the only value it looks a
// token up by, and a copy sealed with AES-256-GCM under a key derived with
// HKDF-SHA256 from module_runtime.ca_kek, so that a super administrator can
// copy the install command again (owner decision D8). Resetting a group's
// token replaces both in one update.
const (
	// tokenSealInfo is the HKDF info of the sealing key: the key-encryption
	// key never seals a token itself, and the derived key seals nothing else.
	tokenSealInfo = "anixops node-group join token v1" // #nosec G101 -- the HKDF info label, not a credential.
	// tokenAdditionalData binds a sealed token to its hash, so a sealed value
	// copied to another group never opens there.
	tokenAdditionalData = "anixops-node-group-join-token:" // #nosec G101 -- the AEAD additional data prefix, not a credential.
	tokenBytes          = 32
)

var tokenPattern = regexp.MustCompile(`^` + agentcontrol.JoinTokenPrefix + `[A-Za-z0-9_-]{43}$`)

// newJoinToken returns a new join token.
func newJoinToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return agentcontrol.JoinTokenPrefix + base64.RawURLEncoding.EncodeToString(raw), nil
}

// ValidToken reports whether token has the form of a join token.
func ValidToken(token string) bool { return tokenPattern.MatchString(token) }

// HashToken is the stored form of a join token: its hex SHA-256.
func HashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

// tokenSealer seals and opens join tokens.
type tokenSealer struct {
	aead cipher.AEAD
}

// sealer derives the sealing key from the key-encryption key: KEK when set,
// else module_runtime.ca_kek of the loaded configuration. ErrNoKEK when
// neither is set.
func (s *Service) sealer() (*tokenSealer, error) {
	var raw string
	switch {
	case s.KEK != nil:
		raw = s.KEK()
	case config.Get() != nil:
		raw = config.Get().ModuleRuntime.CAKEK
	}
	if strings.TrimSpace(raw) == "" {
		return nil, ErrNoKEK
	}
	kek, err := modulepki.ParseKEK(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoKEK, err)
	}
	key, err := hkdf.Key(sha256.New, kek, nil, tokenSealInfo, 32)
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
	return &tokenSealer{aead: aead}, nil
}

// seal returns token sealed for the group whose token hash is hash: the
// base64 of the nonce and the ciphertext.
func (t *tokenSealer) seal(hash, token string) (string, error) {
	nonce := make([]byte, t.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(t.aead.Seal(nonce, nonce, []byte(token), []byte(tokenAdditionalData+hash))), nil
}

// ErrSealedToken: a sealed token is corrupt, belongs to another hash or was
// sealed under another key-encryption key. Resetting the token heals it.
var ErrSealedToken = errors.New("the node group's join token cannot be unsealed: module_runtime.ca_kek changed or the row was altered; reset the token")

// open returns the token sealed for hash. The opened token must hash to
// hash: a sealed value never stands for another token.
func (t *tokenSealer) open(hash, sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil || len(raw) < t.aead.NonceSize() {
		return "", ErrSealedToken
	}
	nonce, ciphertext := raw[:t.aead.NonceSize()], raw[t.aead.NonceSize():]
	plain, err := t.aead.Open(nil, nonce, ciphertext, []byte(tokenAdditionalData+hash))
	if err != nil || HashToken(string(plain)) != hash {
		return "", ErrSealedToken
	}
	return string(plain), nil
}

// newSealedToken returns a new token with its hash and sealed copy.
func newSealedToken(sealer *tokenSealer) (token, hash, sealed string, err error) {
	token, err = newJoinToken()
	if err != nil {
		return "", "", "", err
	}
	hash = HashToken(token)
	sealed, err = sealer.seal(hash, token)
	if err != nil {
		return "", "", "", err
	}
	return token, hash, sealed, nil
}
