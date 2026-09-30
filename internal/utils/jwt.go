package utils

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// LegacyTokenIssuer is the iss claim of the kernel's HS256 tokens.
const LegacyTokenIssuer = "v2board"

type Claims struct {
	UserID  uint   `json:"user_id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
	// SessionID (sid) identifies one login, so a single session can be
	// revoked. Tokens issued before it was added have none.
	SessionID string `json:"sid,omitempty"`
	// TokenVersion (tv) is set by the identity module's tokens; HS256 tokens
	// carry none, which counts as 0.
	TokenVersion uint64 `json:"tv,omitempty"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT token
func GenerateToken(userID uint, email string, isAdmin bool, secret string, expireSeconds int) (string, error) {
	sessionID, err := newSessionID()
	if err != nil {
		return "", err
	}
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Email:     email,
		IsAdmin:   isAdmin,
		SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expireSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			Issuer:    LegacyTokenIssuer,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseTokenWithSecret verifies an HS256 token signed with secret. The
// algorithm is pinned, so a token signed with another HMAC variant or any
// other algorithm is rejected, and iss, exp and iat are required. Revocation
// is checked by internal/authn, which every request path uses.
func ParseTokenWithSecret(tokenString string, secret string) (*Claims, error) {
	if secret == "" {
		return nil, errors.New("token secret is not configured")
	}
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(*jwt.Token) (any, error) {
		return []byte(secret), nil
	},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(LegacyTokenIssuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.IssuedAt == nil {
		return nil, errors.New("token has no issued-at time")
	}
	return claims, nil
}

func newSessionID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
