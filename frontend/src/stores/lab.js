import { defineStore } from "pinia";
import { api } from "../api.js";

// Genel (public) markalama bilgisi. Backend /api/meta uçtan gelir; Phase 1'de
// Laboratory singleton'ı gelene kadar tek kiracı adını burada tutarız.
export const useLab = defineStore("lab", {
  state: () => ({ appName: "Mişko", labName: "" }),
  actions: {
    async load() {
      try {
        const meta = await api("/meta");
        this.appName = meta.appName || this.appName;
        this.labName = meta.labName || "";
      } catch {
        // Sessizce geç: markalama kritik değil, varsayılan ad kullanılır.
      }
    },
  },
});
