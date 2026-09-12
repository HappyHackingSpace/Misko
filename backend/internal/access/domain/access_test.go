package domain

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

// The matrix is written with literal strings, independent of the implementation
// constants, so drift on either side fails loudly.
var expectedMatrix = map[string]string{
	"SUPERADMIN":  "user:manage lab:configure study:write subject:write weight:write apparatus:write test:write test:run *:read",
	"LAB_MANAGER": "user:manage lab:configure study:write subject:write weight:write apparatus:write test:write test:run *:read",
	"RESEARCHER":  "study:write subject:write weight:write apparatus:write test:write test:run *:read",
	"TECHNICIAN":  "weight:write test:run *:read",
	"VIEWER":      "*:read",
}

func TestPermissionMatrixForEveryRoleAndPermission(t *testing.T) {
	all := strings.Fields(expectedMatrix["SUPERADMIN"])
	if len(Roles()) != len(expectedMatrix) || len(Permissions()) != len(all) {
		t.Fatalf("roles=%v permissions=%v", Roles(), Permissions())
	}
	for rawRole, granted := range expectedMatrix {
		role, err := ParseRole(rawRole)
		if err != nil {
			t.Fatalf("%s: %v", rawRole, err)
		}
		for _, rawPermission := range all {
			permission, err := ParsePermission(rawPermission)
			if err != nil {
				t.Fatalf("%s: %v", rawPermission, err)
			}
			want := slices.Contains(strings.Fields(granted), rawPermission)
			if got := role.Allows(permission); got != want {
				t.Errorf("%s %s: allowed=%v want %v", rawRole, rawPermission, got, want)
			}
		}
	}
}

func TestUnknownRolesAndPermissionsAreDenied(t *testing.T) {
	for _, raw := range []string{"", "ADMIN", "superadmin", " SUPERADMIN", "SUPERADMIN\x00"} {
		if _, err := ParseRole(raw); !errors.Is(err, ErrUnknownRole) {
			t.Errorf("ParseRole(%q) err=%v", raw, err)
		}
		if Role(raw).Allows(Read) {
			t.Errorf("unknown role %q can read", raw)
		}
	}
	for _, raw := range []string{"", "user:delete", "*", "*:write", "USER:MANAGE"} {
		if _, err := ParsePermission(raw); !errors.Is(err, ErrUnknownPermission) {
			t.Errorf("ParsePermission(%q) err=%v", raw, err)
		}
		if SuperAdmin.Allows(Permission(raw)) {
			t.Errorf("unknown permission %q granted", raw)
		}
	}
}

func TestActorRequire(t *testing.T) {
	for _, tc := range []struct {
		actor      Actor
		permission Permission
		want       error
	}{
		{Actor{}, Read, ErrUnauthenticated},
		{Actor{Role: SuperAdmin}, Read, ErrUnauthenticated},
		{Actor{UserID: "u", Role: Viewer}, Read, nil},
		{Actor{UserID: "u", Role: Viewer}, TestRun, ErrForbidden},
		{Actor{UserID: "u", Role: Technician}, TestRun, nil},
		{Actor{UserID: "u", Role: Technician}, TestWrite, ErrForbidden},
		{Actor{UserID: "u", Role: Role("ROOT")}, Read, ErrForbidden},
	} {
		if err := tc.actor.Require(tc.permission); !errors.Is(err, tc.want) || (tc.want == nil && err != nil) {
			t.Errorf("%+v %s: err=%v want %v", tc.actor, tc.permission, err, tc.want)
		}
	}
}

func TestOnlyManagingRolesArePrivileged(t *testing.T) {
	for _, role := range Roles() {
		want := role == SuperAdmin || role == LabManager
		if role.Privileged() != want || role.Privileged() != role.Allows(UserManage) {
			t.Errorf("%s privileged=%v", role, role.Privileged())
		}
	}
	if Role("ADMIN").Privileged() {
		t.Error("unknown role is privileged")
	}
}

func TestCatalogCannotBeMutatedByCallers(t *testing.T) {
	roles, permissions := Roles(), Permissions()
	roles[0], permissions[0] = "ROOT", "root:all"
	if Roles()[0] != SuperAdmin || Permissions()[0] != UserManage {
		t.Fatal("caller mutated the RBAC catalog")
	}
}
