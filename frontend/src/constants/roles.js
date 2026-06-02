// RBAC rolleri — backend src/config/permissions.js ile senkron tutulur.
export const ROLES = ["SUPERADMIN", "LAB_MANAGER", "RESEARCHER", "TECHNICIAN", "VIEWER"];

// user:manage iznine sahip roller (kullanıcı yönetimi menüsü/sayfası bunlara açık).
export const PRIVILEGED_ROLES = ["SUPERADMIN", "LAB_MANAGER"];

export const DEFAULT_ROLE = "RESEARCHER";
