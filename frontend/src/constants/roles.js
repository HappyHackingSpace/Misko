// RBAC roles, kept in sync with backend src/config/permissions.js.
export const ROLES = ["SUPERADMIN", "LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"];

// Roles a user can be created or changed into through the API. SUPERADMIN is
// excluded: that account is created once, by the setup command, and the API
// rejects it everywhere else (user.superAdminProtected).
export const ASSIGNABLE_ROLES = ROLES.filter((r) => r !== "SUPERADMIN");

// Roles with the user:manage permission (user management menu/page is open to these).
export const PRIVILEGED_ROLES = ["SUPERADMIN", "LAB_MANAGER"];

export const DEFAULT_ROLE = "RESEARCHER";
