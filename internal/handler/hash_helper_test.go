package handler

import (
	"crypto/sha256"
	"encoding/hex"
)

// hashString hashes a token with SHA-256, matching the api_key_hash contract
// used by the UniProxy node-auth middleware.
func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
