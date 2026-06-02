// RBAC roles, kept in sync with backend src/config/permissions.js.
export const ROLES = ["SUPERADMIN", "LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"];

// Roles with the user:manage permission (user management menu/page is open to these).
export const PRIVILEGED_ROLES = ["SUPERADMIN", "LAB_MANAGER"];

export const DEFAULT_ROLE = "RESEARCHER";
