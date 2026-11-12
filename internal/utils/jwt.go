package utils

import "github.com/golang-jwt/jwt/v5"

// Claims are a verified user access token's claims.
type Claims struct {
	UserID  uint   `json:"user_id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
	// SessionID (sid) identifies one login, so a single session can be
	// revoked.
	SessionID string `json:"sid,omitempty"`
	// TokenVersion (tv) is the account's token version when identity signed
	// the token.
	TokenVersion uint64 `json:"tv,omitempty"`
	jwt.RegisteredClaims
}
