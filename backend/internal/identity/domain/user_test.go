package domain

import (
	"errors"
	access "github.com/HappyHackingSpace/Misko/backend/internal/access/domain"
	"strings"
	"testing"
)

func TestNormalizeEmail(t *testing.T) {
	for raw, want := range map[string]string{
		" Admin@Lab.Example ": "admin@lab.example",
		"a.b+c@x.io":          "a.b+c@x.io",
	} {
		if got, err := NormalizeEmail(raw); err != nil || got != want {
			t.Errorf("NormalizeEmail(%q)=%q,%v want %q", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"", "admin", "@lab.io", "admin@", "admin@lab", "a@b@c.io", "ad min@lab.io",
		"admin@.io", "admin@lab.", "a\x00@lab.io", "\xff@lab.io", strings.Repeat("a", 250) + "@x.io",
	} {
		if _, err := NormalizeEmail(raw); !errors.Is(err, ErrInvalidEmail) {
			t.Errorf("NormalizeEmail(%q) err=%v", raw, err)
		}
	}
}

func TestNormalizeName(t *testing.T) {
	if got, err := NormalizeName("  Ada Lovelace "); err != nil || got != "Ada Lovelace" {
		t.Fatalf("got %q %v", got, err)
	}
	if _, err := NormalizeName(strings.Repeat("ş", 120)); err != nil {
		t.Fatalf("120 multi-byte runes rejected: %v", err)
	}
	for _, raw := range []string{"", "   ", strings.Repeat("a", 121), "bad\nname", "bad\xffname"} {
		if _, err := NormalizeName(raw); !errors.Is(err, ErrInvalidName) {
			t.Errorf("NormalizeName(%q) err=%v", raw, err)
		}
	}
}

// Length is counted in characters for the minimum and in bytes for the bcrypt
// maximum; oversized input is rejected rather than silently truncated.
func TestPasswordByteAndUnicodeBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, password string
		want           error
	}{
		{"seven ascii", "abcdefg", ErrPasswordTooShort},
		{"eight ascii", "abcdefgh", nil},
		{"seven runes in fourteen bytes", strings.Repeat("ş", 7), ErrPasswordTooShort},
		{"eight two-byte runes", strings.Repeat("ş", 8), nil},
		{"72 bytes", strings.Repeat("a", 72), nil},
		{"73 bytes", strings.Repeat("a", 73), ErrPasswordTooLong},
		{"36 two-byte runes", strings.Repeat("ş", 36), nil},
		{"37 two-byte runes", strings.Repeat("ş", 37), ErrPasswordTooLong},
		{"18 four-byte runes", strings.Repeat("🐭", 18), nil},
		{"19 four-byte runes", strings.Repeat("🐭", 19), ErrPasswordTooLong},
		{"invalid UTF-8", "abcdefgh\xff", ErrPasswordEncoding},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidatePassword(tc.password); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestDeletionRules(t *testing.T) {
	admin := User{ID: "admin", Role: access.SuperAdmin}
	manager := User{ID: "manager", Role: access.LabManager}
	researcher := User{ID: "researcher", Role: access.Researcher}
	for _, tc := range []struct {
		name       string
		actorID    string
		target     User
		privileged int
		want       error
	}{
		{"self", "admin", admin, 2, ErrSelfDeletion},
		// The SUPERADMIN account can never be deleted, by anyone, regardless of
		// how many other privileged users exist.
		{"superadmin, sole privileged", "manager", admin, 1, ErrSuperAdminProtected},
		{"superadmin, one of two privileged", "manager", admin, 2, ErrSuperAdminProtected},
		{"last privileged non-superadmin", "someone", manager, 1, ErrLastPrivileged},
		{"one of two privileged non-superadmin", "someone", manager, 2, nil},
		{"unprivileged while one admin remains", "admin", researcher, 1, nil},
	} {
		if err := CheckDeletion(tc.actorID, tc.target, tc.privileged); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: err=%v want %v", tc.name, err, tc.want)
		}
	}
}

func TestRoleChangeRules(t *testing.T) {
	for _, tc := range []struct {
		name       string
		from, to   access.Role
		privileged int
		want       error
	}{
		// The SUPERADMIN account can never be reassigned away from that role,
		// and no one else can ever be promoted into it: it exists only once,
		// created by the setup command.
		{"superadmin cannot be demoted, sole privileged", access.SuperAdmin, access.Researcher, 1, ErrSuperAdminProtected},
		{"superadmin cannot be demoted, one of two privileged", access.SuperAdmin, access.LabManager, 2, ErrSuperAdminProtected},
		{"no one can be promoted to superadmin", access.Researcher, access.SuperAdmin, 1, ErrSuperAdminProtected},
		{"last manager to viewer", access.LabManager, access.Viewer, 1, ErrLastPrivileged},
		{"demote one of two", access.LabManager, access.Technician, 2, nil},
		{"unprivileged change", access.Technician, access.Viewer, 1, nil},
	} {
		if err := CheckRoleChange(User{ID: "u", Role: tc.from}, tc.to, tc.privileged); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%s: err=%v want %v", tc.name, err, tc.want)
		}
	}
}

func TestNotSuperAdminRules(t *testing.T) {
	if err := CheckNotSuperAdminRole(access.SuperAdmin); !errors.Is(err, ErrSuperAdminProtected) {
		t.Errorf("role=SUPERADMIN: err=%v want %v", err, ErrSuperAdminProtected)
	}
	if err := CheckNotSuperAdminRole(access.LabManager); err != nil {
		t.Errorf("role=LAB_MANAGER: err=%v want nil", err)
	}
	if err := CheckNotSuperAdmin(User{Role: access.SuperAdmin}); !errors.Is(err, ErrSuperAdminProtected) {
		t.Errorf("target=SUPERADMIN: err=%v want %v", err, ErrSuperAdminProtected)
	}
	if err := CheckNotSuperAdmin(User{Role: access.LabManager}); err != nil {
		t.Errorf("target=LAB_MANAGER: err=%v want nil", err)
	}
}
