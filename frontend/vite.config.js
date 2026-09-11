import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// VITE_API_PROXY points the dev server at another API, for example the end to
// end server the browser tests start.
const api = process.env.VITE_API_PROXY || "http://localhost:4000";

export default defineConfig({
  plugins: [vue()],
  server: {
    port: 5173,
    proxy: {
      "/api": api,
    },
  },
});
