package domain

import (
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

var (
	ErrInvalidEmail     = errors.New("invalid email address")
	ErrInvalidName      = errors.New("name must contain 1 to 120 printable characters")
	ErrPasswordTooShort = errors.New("password must contain at least 8 characters")
	ErrPasswordTooLong  = errors.New("password must not exceed 72 bytes")
	ErrPasswordEncoding = errors.New("password must be valid UTF-8")
	ErrSelfDeletion     = errors.New("users cannot delete their own account")
	ErrLastPrivileged   = errors.New("the last privileged user cannot be deleted or demoted")
)

const (
	MinPasswordChars = 8
	// MaxPasswordBytes is the bcrypt input limit. Longer passwords are rejected
	// instead of being silently truncated.
	MaxPasswordBytes = 72
	maxEmailBytes    = 254
	maxNameChars     = 120
)

type User struct {
	ID        string
	Email     string
	Name      string
	Role      access.Role
	CreatedAt time.Time
}

// NormalizeEmail trims and lowercases an address so uniqueness is case-insensitive.
func NormalizeEmail(raw string) (string, error) {
	// Validate before lowercasing: ToLower replaces invalid bytes with U+FFFD.
	if !utf8.ValidString(raw) {
		return "", ErrInvalidEmail
	}
	email := strings.ToLower(strings.TrimSpace(raw))
	if email == "" || len(email) > maxEmailBytes || strings.IndexFunc(email, blank) >= 0 {
		return "", ErrInvalidEmail
	}
	local, host, _ := strings.Cut(email, "@")
	if local == "" || host == "" || strings.Contains(host, "@") || !strings.Contains(host, ".") ||
		strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
		return "", ErrInvalidEmail
	}
	return email, nil
}

func NormalizeName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > maxNameChars || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", ErrInvalidName
	}
	return name, nil
}

// ValidatePassword applies the policy for newly chosen passwords. Passwords are
// compared byte for byte; no trimming or Unicode normalization is applied.
func ValidatePassword(password string) error {
	switch {
	case !utf8.ValidString(password):
		return ErrPasswordEncoding
	case utf8.RuneCountInString(password) < MinPasswordChars:
		return ErrPasswordTooShort
	case len(password) > MaxPasswordBytes:
		return ErrPasswordTooLong
	}
	return nil
}

// CheckDeletion enforces that users cannot delete themselves and that the last
// privileged user remains. privilegedCount must be read in the same serializable
// transaction as the deletion.
func CheckDeletion(actorID string, target User, privilegedCount int) error {
	if actorID == target.ID {
		return ErrSelfDeletion
	}
	if target.Role.Privileged() && privilegedCount <= 1 {
		return ErrLastPrivileged
	}
	return nil
}

// CheckRoleChange prevents demoting the last privileged user to an unprivileged role.
func CheckRoleChange(target User, next access.Role, privilegedCount int) error {
	if target.Role.Privileged() && !next.Privileged() && privilegedCount <= 1 {
		return ErrLastPrivileged
	}
	return nil
}

func blank(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }
