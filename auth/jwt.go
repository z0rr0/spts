package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	// ErrInvalidToken is an error for invalid token.
	ErrInvalidToken = errors.New("invalid token")
	// ErrTokenExpired is an error for expired token.
	ErrTokenExpired = errors.New("token expired")
)

// JWTManager is a JWT token manager.
type JWTManager struct {
	config Config
}

// Claims is a JWT claims.
type Claims struct {
	Username string
	jwt.RegisteredClaims
}

// NewJWTManager creates a new JWT token manager.
func NewJWTManager(config Config) *JWTManager {
	return &JWTManager{config: config}
}

// Create creates a new JWT token.
func (jm *JWTManager) Create(username string) (string, error) {
	var now = time.Now()

	claims := Claims{
		username,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(jm.config.TokenDuration)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	return token.SignedString(jm.config.JWTSecret)
}

// Validate checks if the token is valid.
func (jm *JWTManager) Validate(tokenString string) (*Claims, error) {
	var claims = new(Claims)

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return jm.config.JWTSecret, nil
	})

	if err != nil {
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrTokenExpired
	}

	return claims, nil
}
