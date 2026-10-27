package access

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// NewToken generates 256 bits of random credential entropy with an explicit purpose prefix.
func NewToken(prefix string) string {
	var value [32]byte
	_, _ = rand.Read(value[:]) // crypto/rand.Read cannot fail on supported Go platforms.
	return prefix + base64.RawURLEncoding.EncodeToString(value[:])
}

// TokenHash is the stored verifier; plaintext credentials never enter persistence.
func TokenHash(value string) string {
	digest := sha256.Sum256([]byte("keldock-credential-v1\x00" + value))
	return hex.EncodeToString(digest[:])
}

// CSRF binds an anti-forgery token to exactly one opaque browser session.
func CSRF(session string) string {
	mac := hmac.New(sha256.New, []byte(session))
	_, _ = mac.Write([]byte("keldock-csrf-v1")) // hash.Hash.Write always succeeds.
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// ValidCSRF compares a submitted anti-forgery token without exposing the session.
func ValidCSRF(session, submitted string) bool {
	return len(session) >= 32 && subtle.ConstantTimeCompare([]byte(CSRF(session)), []byte(submitted)) == 1
}

// HashPassword uses salted Argon2id and records its bounded work factors in PHC format.
func HashPassword(password string) (string, error) {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || utf8.RuneCountInString(password) > 256 || len(password) > 1024 {
		return "", fmt.Errorf("password must contain 12–256 characters")
	}
	var salt [16]byte
	_, _ = rand.Read(salt[:]) // crypto/rand.Read cannot fail on supported Go platforms.
	digest := argon2.IDKey([]byte(password), salt[:], 3, 64*1024, 2, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=3,p=2$%s$%s", base64.RawStdEncoding.EncodeToString(salt[:]), base64.RawStdEncoding.EncodeToString(digest)), nil
}

// VerifyPassword rejects malformed or excessive hash parameters before allocating memory.
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" || len(password) > 1024 {
		return false
	}
	if parts[3] != "m=65536,t=3,p=2" {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) != 16 {
		return false
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expected) != 32 {
		return false
	}
	actual := argon2.IDKey([]byte(password), salt, 3, 65536, 2, 32)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}
