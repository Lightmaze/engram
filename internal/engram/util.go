package engram

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

func validateID(value string) error {
	if value == "" || len(value) > 200 {
		return errors.New("id must be 1-200 characters")
	}
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("-_.:", r)) {
			return errors.New("id contains an unsupported character")
		}
	}
	return nil
}
func required(name, value string, max int) error {
	if strings.TrimSpace(value) == "" {
		return errors.New(name + " is required")
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max {
		return errors.New(name + " is invalid or too long")
	}
	return nil
}
func randomID(prefix string) string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return prefix + "-" + hex.EncodeToString(raw)
}
func sha256Bytes(value []byte) []byte { sum := sha256.Sum256(value); return sum[:] }
func nowUTC() time.Time               { return time.Now().UTC() }
