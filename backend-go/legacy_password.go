package main

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/scrypt"
)

// verifyPasswordHash checks a password against either a new bcrypt hash
// (format: "$2a$...", "$2b$...") or a legacy werkzeug hash migrated as-is
// from the old Flask/SQLite database. werkzeug produces two formats:
//   scrypt:N:r:p$salt$hexdigest
//   pbkdf2:sha256:iterations$salt$hexdigest
// Returns (matched, wasLegacyFormat).
func verifyPasswordHash(stored, password string) (bool, bool) {
	if strings.HasPrefix(stored, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(stored), []byte(password)) == nil, false
	}
	parts := strings.SplitN(stored, "$", 3)
	if len(parts) != 3 {
		return false, true
	}
	method, salt, hexDigest := parts[0], parts[1], parts[2]
	want, err := hex.DecodeString(hexDigest)
	if err != nil {
		return false, true
	}
	methodParts := strings.Split(method, ":")
	switch methodParts[0] {
	case "scrypt":
		if len(methodParts) != 4 {
			return false, true
		}
		n, _ := strconv.Atoi(methodParts[1])
		r, _ := strconv.Atoi(methodParts[2])
		p, _ := strconv.Atoi(methodParts[3])
		got, err := scrypt.Key([]byte(password), []byte(salt), n, r, p, len(want))
		if err != nil {
			return false, true
		}
		return subtle.ConstantTimeCompare(got, want) == 1, true
	case "pbkdf2":
		if len(methodParts) != 3 {
			return false, true
		}
		iterations, _ := strconv.Atoi(methodParts[2])
		got := pbkdf2.Key([]byte(password), []byte(salt), iterations, len(want), sha256.New)
		return subtle.ConstantTimeCompare(got, want) == 1, true
	default:
		return false, true
	}
}
