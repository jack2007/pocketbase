package crypto

import (
	"crypto/rand"
	"encoding/base64"

	"golang.org/x/crypto/bcrypt"
)

const secretBytes = 32

// GenerateEnrollSecret creates a URL-safe enrollment secret and its bcrypt hash.
func GenerateEnrollSecret() (plaintext string, hash string, err error) {
	raw := make([]byte, secretBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}

	plaintext = base64.RawURLEncoding.EncodeToString(raw)
	hash, err = HashToken(plaintext)
	if err != nil {
		return "", "", err
	}
	return plaintext, hash, nil
}

// VerifySecret reports whether plaintext matches a bcrypt hash.
func VerifySecret(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}

// HashToken hashes a session token with bcrypt's default cost.
func HashToken(plaintext string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}
