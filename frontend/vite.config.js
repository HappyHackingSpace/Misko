import { defineConfig } from "vite";
import vue from "@vitejs/plugin-vue";

// VITE_API_PROXY points the dev server at another API, for example the end to
// end server the browser tests start.
const api = process.env.VITE_API_PROXY || "http://localhost:4000";

export default defineConfig({
  plugins: [vue()],
  // Naive UI resolves its style/theme helpers lazily; prebundling them avoids
  // a dev-server reload storm the first time a page imports a component.
  optimizeDeps: {
    include: ["naive-ui", "vueuc", "date-fns-tz"],
  },
  server: {
    port: 5173,
    proxy: {
      "/api": api,
    },
  },
});
