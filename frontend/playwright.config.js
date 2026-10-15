import { defineConfig, devices } from "@playwright/test";

// Browser tests run against the real Go API started by backend/tests/e2e, which
// keeps uploads in memory instead of Cloud Storage. E2E_DATABASE_URL must name a
// PostgreSQL database whose name contains "e2e"; the server reinstalls the
// schema and seeds two running studies with results to review. The specs
// change that seed, so run one project per server start:
//   E2E_DATABASE_URL=... npx playwright test --project=chromium
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
    // Dates on screen and the lab (UTC in the seed) agree on what today is.
    timezoneId: "UTC",
    locale: "en-US",
    video: "off",
  },
  // The panel leans on <video> and upload behaviour that differs between
  // engines. A laboratory on macOS or an iPad gets WebKit, so CI runs the same
  // suite on all three, each against its own freshly seeded server.
  projects: [
    { name: "chromium", use: { ...devices["Desktop Chrome"] } },
    { name: "firefox", use: { ...devices["Desktop Firefox"] } },
    { name: "webkit", use: { ...devices["Desktop Safari"] } },
  ],
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
