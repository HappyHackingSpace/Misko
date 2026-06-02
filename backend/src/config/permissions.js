/**
 * RBAC - role and permission matrix (in-code, not editable in the DB).
 *
 * Design: permissions are defined in code; simple, SOLID, testable, and
 * changes ship via deploy (see docs/DOMAIN.md section 5b).
 *
 * Permission format: `resource:action`. Read permission exists in all roles (`*:read`).
 */

export const ROLES = {
  SUPERADMIN: "SUPERADMIN",
  LAB_MANAGER: "LAB_MANAGER",
  RESEARCHER: "RESEARCHER",
  TECHNICIAN: "TECHNICIAN",
  VIEWER: "VIEWER",
};

export const ROLE_LIST = Object.values(ROLES);

export const PERMISSIONS = {
  USER_MANAGE: "user:manage",
  LAB_CONFIGURE: "lab:configure",
  STUDY_WRITE: "study:write",
  SUBJECT_WRITE: "subject:write",
  WEIGHT_WRITE: "weight:write",
  APPARATUS_WRITE: "apparatus:write",
  TEST_WRITE: "test:write",
  TEST_RUN: "test:run",
  READ: "*:read",
};

const P = PERMISSIONS;

// RESEARCHER permissions (manages studies/subjects/apparatuses, runs tests).
const RESEARCHER_PERMS = [
  P.STUDY_WRITE,
  P.SUBJECT_WRITE,
  P.WEIGHT_WRITE,
  P.APPARATUS_WRITE,
  P.TEST_WRITE,
  P.TEST_RUN,
  P.READ,
];

// SUPERADMIN and LAB_MANAGER have all permissions (same matrix).
const ALL_PERMS = Object.values(P);

/** ROLE -> Set<permission> */
export const ROLE_PERMISSIONS = {
  [ROLES.SUPERADMIN]: new Set(ALL_PERMS),
  [ROLES.LAB_MANAGER]: new Set(ALL_PERMS),
  [ROLES.RESEARCHER]: new Set(RESEARCHER_PERMS),
  [ROLES.TECHNICIAN]: new Set([P.WEIGHT_WRITE, P.TEST_RUN, P.READ]),
  [ROLES.VIEWER]: new Set([P.READ]),
};

/**
 * Returns whether a role has a specific permission.
 * @param {string} role
 * @param {string} permission
 * @returns {boolean}
 */
export function hasPermission(role, permission) {
  const perms = ROLE_PERMISSIONS[role];
  return perms ? perms.has(permission) : false;
}

/**
 * "Privileged" roles that can manage users, such as SUPERADMIN/LAB_MANAGER.
 * The "last admin cannot be removed" rule operates over this set.
 */
export const PRIVILEGED_ROLES = [ROLES.SUPERADMIN, ROLES.LAB_MANAGER];

export function isPrivilegedRole(role) {
  return PRIVILEGED_ROLES.includes(role);
}
