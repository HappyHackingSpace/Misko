/**
 * RBAC — rol ve izin matrisi (koda gömülü, DB'de düzenlenmez).
 *
 * Tasarım: izinler kodda tanımlıdır; basit, SOLID, test edilebilir ve
 * değişiklikler deploy ile gelir (bkz. docs/DOMAIN.md §5b).
 *
 * İzin biçimi: `resource:action`. Okuma izni tüm rollerde vardır (`*:read`).
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
  PARADIGM_TOGGLE: "paradigm:toggle",
  STUDY_WRITE: "study:write",
  SUBJECT_WRITE: "subject:write",
  WEIGHT_WRITE: "weight:write",
  APPARATUS_WRITE: "apparatus:write",
  TEST_WRITE: "test:write",
  TEST_RUN: "test:run",
  READ: "*:read",
};

const P = PERMISSIONS;

// RESEARCHER'ın izinleri (çalışmaları/denekleri/aparatları yönetir, test koşar).
const RESEARCHER_PERMS = [
  P.STUDY_WRITE,
  P.SUBJECT_WRITE,
  P.WEIGHT_WRITE,
  P.APPARATUS_WRITE,
  P.TEST_WRITE,
  P.TEST_RUN,
  P.READ,
];

// SUPERADMIN ve LAB_MANAGER tüm izinlere sahiptir (matris aynı).
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
 * Bir rolün belirli bir izne sahip olup olmadığını döner.
 * @param {string} role
 * @param {string} permission
 * @returns {boolean}
 */
export function hasPermission(role, permission) {
  const perms = ROLE_PERMISSIONS[role];
  return perms ? perms.has(permission) : false;
}

/**
 * SUPERADMIN/LAB_MANAGER gibi kullanıcı yönetimi yapabilen "ayrıcalıklı" roller.
 * "Son yönetici kaldırılamaz" kuralı bu küme üzerinden işler.
 */
export const PRIVILEGED_ROLES = [ROLES.SUPERADMIN, ROLES.LAB_MANAGER];

export function isPrivilegedRole(role) {
  return PRIVILEGED_ROLES.includes(role);
}
