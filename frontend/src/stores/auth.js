import { defineStore } from "pinia";
import { api, setToken, getToken } from "../api.js";
import { PRIVILEGED_ROLES } from "../constants/roles.js";

export const useAuth = defineStore("auth", {
  state: () => ({ user: null, ready: false }),
  getters: {
    isLoggedIn: (s) => !!s.user,
    // Roles with the user:manage permission (SUPERADMIN / LAB_MANAGER).
    canManageUsers: (s) => PRIVILEGED_ROLES.includes(s.user?.role),
    // Backward compatibility: admin pages/menus use this getter.
    isAdmin: (s) => PRIVILEGED_ROLES.includes(s.user?.role),
  },
  actions: {
    async init() {
      if (getToken()) {
        try {
          const { user } = await api("/auth/me");
          this.user = user;
        } catch {
          setToken(null);
        }
      }
      this.ready = true;
    },
    async login(email, password) {
      const { token, user } = await api("/auth/login", { method: "POST", body: { email, password } });
      setToken(token);
      this.user = user;
    },
    logout() {
      setToken(null);
      this.user = null;
    },
  },
});
