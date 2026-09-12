import { defineConfig, devices } from "@playwright/test";

// Browser tests run against the real Go API started by backend/tests/e2e, which
// serves signed upload and read URLs from local disk instead of Cloud Storage.
// E2E_DATABASE_URL must name an empty PostgreSQL database; the server installs
// the schema and seeds one analyzed test.
const API_PORT = process.env.E2E_API_PORT || "4010";
const APP_PORT = process.env.E2E_APP_PORT || "5273";

export default defineConfig({
  testDir: "tests/e2e",
  fullyParallel: false,
  workers: 1,
  forbidOnly: !!process.env.CI,
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [["github"], ["list"]] : [["list"]],
  use: {
    baseURL: `http://127.0.0.1:${APP_PORT}`,
    // The panel marks its test hooks with data-test.
    testIdAttribute: "data-test",
    trace: "retain-on-failure",
    video: "off",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
  webServer: [
    {
      command: `go run ./tests/e2e -addr 127.0.0.1:${API_PORT}`,
      cwd: "../backend",
      url: `http://127.0.0.1:${API_PORT}/api/ready`,
      reuseExistingServer: !process.env.CI,
      timeout: 180_000,
      stdout: "pipe",
      stderr: "pipe",
    },
    {
      // --host pins the dev server to the address the tests poll; without it
      // Vite binds "localhost", which can resolve to the IPv6 loopback only.
      command: `npm run dev -- --host 127.0.0.1 --port ${APP_PORT} --strictPort`,
      url: `http://127.0.0.1:${APP_PORT}`,
      reuseExistingServer: !process.env.CI,
      timeout: 120_000,
      env: { VITE_API_PROXY: `http://127.0.0.1:${API_PORT}` },
    },
  ],
});
