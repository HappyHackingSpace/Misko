import { defineStore } from "pinia";
import { api } from "../api/client.js";

// Public branding info. The backend /api/meta endpoint returns the
// Laboratory singleton's name (falls back to LAB_NAME if not set up).
export const useLab = defineStore("lab", {
  state: () => ({ appName: "Mişko", labName: "" }),
  getters: {
    // Browser tab / window title: product name + lab name.
    title: (s) => (s.labName ? `${s.appName} · ${s.labName}` : s.appName),
  },
  actions: {
    async load() {
      try {
        const meta = await api("/meta");
        this.appName = meta.appName || this.appName;
        this.labName = meta.labName || "";
      } catch {
        // Fail silently: branding is not critical, the default name is used.
      }
      document.title = this.title;
    },
  },
});
