// Package token issues random worker tokens and hashes them with SHA-256.
// Tokens have 256 bits of entropy, so a fast hash is sufficient for lookup.
package token

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

type Tokens struct{}

func New() Tokens { return Tokens{} }

func (Tokens) New() (string, []byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("generate worker token: %w", err)
	}
	token := "mw_" + base64.RawURLEncoding.EncodeToString(raw)
	return token, Tokens{}.Hash(token), nil
}

func (Tokens) Hash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}
