// Package auth provides password hashing and opaque session credentials.
package auth

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
)

const iterations = 600000

func Token() string { return rand.Text() }

func Digest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func Hash(password string) (string, error) {
	if len(password) < 12 || len(password) > 256 {
		return "", fmt.Errorf("password must be 12 to 256 bytes")
	}
	salt := Token()
	key, err := pbkdf2.Key(sha256.New, password, []byte(salt), iterations, 32)
	if err != nil {
		return "", err
	}
	return "pbkdf2-sha256$600000$" + salt + "$" + hex.EncodeToString(key), nil
}

func Verify(hash, password string) bool {
	parts := strings.Split(hash, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || parts[1] != "600000" || len(password) > 256 {
		return false
	}
	expected, err := hex.DecodeString(parts[3])
	if err != nil {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, []byte(parts[2]), iterations, 32)
	return err == nil && subtle.ConstantTimeCompare(key, expected) == 1
}
