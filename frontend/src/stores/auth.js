import { defineStore } from "pinia";
import { auth as authApi } from "../api/endpoints.js";
import { getToken, setToken } from "../api/client.js";

// The API decides what a role may do and returns the permissions of the signed
// in user; the panel only hides what the API would refuse.
export const useAuth = defineStore("auth", {
  state: () => ({ user: null, permissions: [], ready: false }),
  getters: {
    isLoggedIn: (s) => !!s.user,
    can: (s) => (permission) => s.permissions.includes(permission),
    canRun: (s) => s.permissions.includes("test:run"),
    canWriteTests: (s) => s.permissions.includes("test:write"),
    canManageUsers: (s) => s.permissions.includes("user:manage"),
    canConfigureLab: (s) => s.permissions.includes("lab:configure"),
  },
  actions: {
    async init() {
      if (getToken()) {
        try {
          const { user, permissions } = await authApi.me();
          this.user = user;
          this.permissions = permissions || [];
        } catch {
          setToken(null);
        }
      }
      this.ready = true;
    },
    async login(email, password) {
      const { token, user } = await authApi.login(email, password);
      setToken(token);
      this.user = user;
      const me = await authApi.me();
      this.permissions = me.permissions || [];
    },
    logout() {
      setToken(null);
      this.user = null;
      this.permissions = [];
    },
  },
});
