package utils

import (
	"errors"
	"time"

	"github.com/anixops/v2board/internal/config"
	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID  uint   `json:"user_id"`
	Email   string `json:"email"`
	IsAdmin bool   `json:"is_admin"`
	jwt.RegisteredClaims
}

// GenerateToken 生成JWT token
func GenerateToken(userID uint, email string, isAdmin bool, secret string, expireSeconds int) (string, error) {
	claims := Claims{
		UserID:  userID,
		Email:   email,
		IsAdmin: isAdmin,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expireSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "v2board",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ParseToken 解析JWT token (自动从配置获取secret)
func ParseToken(tokenString string) (*Claims, error) {
	cfg := config.Get()
	if cfg == nil {
		return nil, errors.New("config not loaded")
	}
	return ParseTokenWithSecret(tokenString, cfg.JWT.Secret)
}

// ParseTokenWithSecret 使用指定secret解析JWT token
func ParseTokenWithSecret(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}
