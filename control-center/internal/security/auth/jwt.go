package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// TokenType represents the type of token
type TokenType string

const (
	AccessToken  TokenType = "access"
	RefreshToken TokenType = "refresh"
	APIToken     TokenType = "api"
)

// Claims represents JWT claims
type Claims struct {
	jwt.RegisteredClaims
	Role         string   `json:"role"`
	Scopes       []string `json:"scopes,omitempty"`
	TenantID     string   `json:"tenant_id,omitempty"`
	AuthProvider string   `json:"auth_provider"` // local, oauth, ldap, saml
}

// JWTManager handles JWT operations
type JWTManager struct {
	secret        []byte
	accessExpire  time.Duration
	refreshExpire time.Duration
	issuer        string
}

// NewJWTManager creates a new JWT manager
func NewJWTManager(secret string, accessExpire, refreshExpire int, issuer string) *JWTManager {
	return &JWTManager{
		secret:        []byte(secret),
		accessExpire:  time.Duration(accessExpire) * time.Second,
		refreshExpire: time.Duration(refreshExpire) * time.Second,
		issuer:        issuer,
	}
}

// GenerateAccessToken generates an access token
func (j *JWTManager) GenerateAccessToken(userID, role string, scopes []string, authProvider string) (string, error) {
	return j.generateToken(userID, role, scopes, "", authProvider, j.accessExpire, AccessToken)
}

// GenerateRefreshToken generates a refresh token
func (j *JWTManager) GenerateRefreshToken(userID, role string, authProvider string) (string, error) {
	return j.generateToken(userID, role, nil, "", authProvider, j.refreshExpire, RefreshToken)
}

// GenerateAPIToken generates an API token (long-lived)
func (j *JWTManager) GenerateAPIToken(userID, name string) (string, error) {
	return j.generateToken(userID, "api", nil, "", "local", time.Hour*24*365, APIToken)
}

func (j *JWTManager) generateToken(userID, role string, scopes []string, tenantID, authProvider string, expiry time.Duration, tokenType TokenType) (string, error) {
	now := time.Now()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    j.issuer,
		},
		Role:         role,
		Scopes:       scopes,
		TenantID:     tenantID,
		AuthProvider: authProvider,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

// ValidateToken validates a token and returns claims
func (j *JWTManager) ValidateToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return j.secret, nil
	})

	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// RefreshAccessToken refreshes an access token using a refresh token
func (j *JWTManager) RefreshAccessToken(refreshToken string) (string, error) {
	claims, err := j.ValidateToken(refreshToken)
	if err != nil {
		return "", err
	}

	return j.GenerateAccessToken(claims.Subject, claims.Role, claims.Scopes, claims.AuthProvider)
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword checks if a password matches a hash
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// Errors
var (
	ErrInvalidToken  = errors.New("invalid token")
	ErrTokenExpired  = errors.New("token expired")
	ErrInvalidClaims = errors.New("invalid claims")
)
