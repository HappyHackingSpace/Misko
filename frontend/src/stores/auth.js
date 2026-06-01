import { defineStore } from "pinia";
import { api, setToken, getToken } from "../api.js";

export const useAuth = defineStore("auth", {
  state: () => ({ user: null, ready: false }),
  getters: {
    isLoggedIn: (s) => !!s.user,
    isAdmin: (s) => s.user?.role === "ADMIN",
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
