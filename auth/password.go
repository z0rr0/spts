package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha3"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"log/slog"
)

const (
	saltLength = 64
	iterations = 600_000
)

// PasswordManager is a password manager.
type PasswordManager struct {
	config Config
}

// NewPasswordManager creates a new password manager.
func NewPasswordManager(config Config) *PasswordManager {
	return &PasswordManager{config: config}
}

// generateSalt generates a new user salt.
func (pm *PasswordManager) generateSalt() (string, error) {
	salt := make([]byte, saltLength)

	_, err := rand.Read(salt)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(salt), nil
}

// Hash returns a hash of the password.
func (pm *PasswordManager) Hash(password string, userSalt string) (string, error) {
	saltBytes, err := base64.StdEncoding.DecodeString(userSalt)
	if err != nil {
		return "", err
	}

	// mix global salt with user one
	combinedSalt := append(pm.config.GlobalSalt, saltBytes...)
	hash, err := pbkdf2.Key(sha3.New512, password, combinedSalt, iterations, sha512.Size)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}

	return base64.StdEncoding.EncodeToString(hash), nil
}

// Verify checks if the password is correct.
func (pm *PasswordManager) Verify(password, storedHash, userSalt string) bool {
	newHash, err := pm.Hash(password, userSalt)
	if err != nil {
		slog.Warn("failed to hash password", "error", err)
		return false
	}

	isEqual := subtle.ConstantTimeCompare([]byte(newHash), []byte(storedHash))
	return isEqual == 1
}
