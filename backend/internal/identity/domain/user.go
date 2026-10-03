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
	// ErrSuperAdminProtected guards the single SUPERADMIN account bootstrap
	// creates: nobody, including that account acting on itself through these
	// routes, may assign the role, or rename, reassign, reset the password of,
	// or delete that account. It exists only once and answers to no one.
	ErrSuperAdminProtected = errors.New("the superadmin account cannot be created, changed or removed through this API")
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

// CheckDeletion enforces that users cannot delete themselves, that the
// SUPERADMIN account can never be deleted, and that the last privileged user
// remains. privilegedCount must be read in the same serializable transaction
// as the deletion.
func CheckDeletion(actorID string, target User, privilegedCount int) error {
	if actorID == target.ID {
		return ErrSelfDeletion
	}
	if target.Role == access.SuperAdmin {
		return ErrSuperAdminProtected
	}
	if target.Role.Privileged() && privilegedCount <= 1 {
		return ErrLastPrivileged
	}
	return nil
}

// CheckRoleChange rejects any change to or from SUPERADMIN — that role exists
// only for the single account the setup command creates — and prevents
// demoting the last privileged user to an unprivileged role.
func CheckRoleChange(target User, next access.Role, privilegedCount int) error {
	if target.Role == access.SuperAdmin || next == access.SuperAdmin {
		return ErrSuperAdminProtected
	}
	if target.Role.Privileged() && !next.Privileged() && privilegedCount <= 1 {
		return ErrLastPrivileged
	}
	return nil
}

// CheckNotSuperAdminRole rejects SUPERADMIN as the role for a newly created
// user: the account exists only once, created by the setup command.
func CheckNotSuperAdminRole(role access.Role) error {
	if role == access.SuperAdmin {
		return ErrSuperAdminProtected
	}
	return nil
}

// CheckNotSuperAdmin rejects any action against the SUPERADMIN account other
// than role change or deletion, which CheckRoleChange and CheckDeletion
// already cover — currently just resetting its password.
func CheckNotSuperAdmin(target User) error {
	if target.Role == access.SuperAdmin {
		return ErrSuperAdminProtected
	}
	return nil
}

func blank(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }
