// Package domain is the shared RBAC kernel: roles, permissions and the
// authenticated actor. It is the only domain package other domains may import,
// so every use case checks authorization against the same static matrix.
package domain

import "errors"

var (
	ErrUnauthenticated   = errors.New("authentication required")
	ErrForbidden         = errors.New("permission denied")
	ErrUnknownRole       = errors.New("unknown role")
	ErrUnknownPermission = errors.New("unknown permission")
)

type Role string

const (
	SuperAdmin Role = "SUPERADMIN"
	LabManager Role = "LAB_MANAGER"
	Researcher Role = "RESEARCHER"
	Technician Role = "TECHNICIAN"
	Viewer     Role = "VIEWER"
)

// Roles returns every role, most privileged first, as a fresh slice.
func Roles() []Role { return []Role{SuperAdmin, LabManager, Researcher, Technician, Viewer} }

func ParseRole(raw string) (Role, error) {
	for _, role := range Roles() {
		if string(role) == raw {
			return role, nil
		}
	}
	return "", ErrUnknownRole
}

// Privileged roles manage users; at least one privileged user must always exist.
func (r Role) Privileged() bool { return r == SuperAdmin || r == LabManager }

type Permission string

const (
	UserManage     Permission = "user:manage"
	LabConfigure   Permission = "lab:configure"
	StudyWrite     Permission = "study:write"
	SubjectWrite   Permission = "subject:write"
	WeightWrite    Permission = "weight:write"
	ApparatusWrite Permission = "apparatus:write"
	TestWrite      Permission = "test:write"
	TestRun        Permission = "test:run"
	Read           Permission = "*:read"
)

// Permissions returns every permission as a fresh slice.
func Permissions() []Permission {
	return []Permission{UserManage, LabConfigure, StudyWrite, SubjectWrite, WeightWrite, ApparatusWrite, TestWrite, TestRun, Read}
}

func ParsePermission(raw string) (Permission, error) {
	for _, permission := range Permissions() {
		if string(permission) == raw {
			return permission, nil
		}
	}
	return "", ErrUnknownPermission
}

// Allows is the static permission matrix. Unknown roles and permissions are denied.
func (r Role) Allows(p Permission) bool {
	if _, err := ParsePermission(string(p)); err != nil {
		return false
	}
	switch r {
	case SuperAdmin, LabManager:
		return true
	case Researcher:
		return p != UserManage && p != LabConfigure
	case Technician:
		return p == WeightWrite || p == TestRun || p == Read
	case Viewer:
		return p == Read
	default:
		return false
	}
}

// Actor is the authenticated caller. Its role is loaded from the database on
// every request, never taken from a token claim.
type Actor struct {
	UserID string
	Role   Role
}

func (a Actor) Require(p Permission) error {
	if a.UserID == "" {
		return ErrUnauthenticated
	}
	if !a.Role.Allows(p) {
		return ErrForbidden
	}
	return nil
}
