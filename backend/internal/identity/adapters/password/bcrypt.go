// Package password hashes, compares and generates passwords with bcrypt.
package password

import (
	"crypto/rand"
	"fmt"
	"github.com/HappyHackingSpace/Misko/backend/internal/identity/domain"
	"golang.org/x/crypto/bcrypt"
)

type Bcrypt struct {
	cost int
	// dummy is compared against when no account exists, to equalize timing.
	dummy []byte
}

func NewBcrypt(cost int) (*Bcrypt, error) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return nil, fmt.Errorf("bcrypt cost must be between %d and %d", bcrypt.MinCost, bcrypt.MaxCost)
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte(rand.Text()), cost)
	if err != nil {
		return nil, fmt.Errorf("prepare bcrypt: %w", err)
	}
	return &Bcrypt{cost: cost, dummy: dummy}, nil
}

func (b *Bcrypt) Hash(password string) (string, error) {
	if len(password) > domain.MaxPasswordBytes {
		return "", domain.ErrPasswordTooLong
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), b.cost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// Matches never accepts input longer than bcrypt reads, so a long password
// cannot match the hash of its 72-byte prefix.
func (b *Bcrypt) Matches(hash, password string) bool {
	if hash == "" || len(password) > domain.MaxPasswordBytes {
		_ = bcrypt.CompareHashAndPassword(b.dummy, []byte(password[:min(len(password), domain.MaxPasswordBytes)]))
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// Generate returns 26 random base32 characters (130 bits).
func (b *Bcrypt) Generate() (string, error) { return rand.Text(), nil }
