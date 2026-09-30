// What the panel shows around the analysis: a failed run explains itself, a
// read-only role cannot start work, and switching runs swaps the result.
import { expect, test } from "@playwright/test";
import { openRun } from "./helpers.js";

const API = `http://127.0.0.1:${process.env.E2E_API_PORT || "4010"}`;

async function seed(request) {
  const response = await request.get(`${API}/e2e/seed`);
  expect(response.ok()).toBeTruthy();
  return response.json();
}

async function signIn(page, email, password) {
  await page.goto("/login");
  await page.locator("#login-email").fill(email);
  await page.locator("#login-password").fill(password);
  await page.getByTestId("sign-in").click();
  await expect(page.getByTestId("nav-experiments")).toBeVisible();
}

test("a failed run says why and publishes no result", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  // The failed run belongs to the second recording, whose analysis failed
  // quality control.
  await openRun(page, request, data.failedRunId);

  const failure = page.getByTestId("run-failure");
  await expect(failure).toBeVisible();
  await expect(failure).toContainText("QC_FAILED");
  // Nothing is published for it: no metrics, no events, no video pair.
  await expect(page.getByTestId("metrics")).toHaveCount(0);
  await expect(page.getByTestId("event-timeline")).toHaveCount(0);
  await expect(page.getByTestId("video-pair")).toHaveCount(0);
});

test("switching back to the successful run restores its result", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  await openRun(page, request, data.failedRunId);
  await expect(page.getByTestId("run-failure")).toBeVisible();

  await openRun(page, request, data.runId);
  await expect(page.getByTestId("metric-distance_cm")).toBeVisible();
  await expect(page.getByTestId("event")).toHaveCount(3);
  // The panel plays the run that is selected, not the one that failed.
  await expect(page.getByTestId("analyzed-video")).toBeVisible();
});

test("a viewer reads the result but cannot start work", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await page.goto(`/tests/${data.testId}`);

  // The published result is readable.
  await expect(page.getByTestId("metric-distance_cm")).toBeVisible();
  await expect(page.getByTestId("event")).toHaveCount(3);

  // Uploading and reanalysis need test:run, which a viewer does not have.
  await expect(page.getByTestId("upload-input")).toHaveCount(0);
  await expect(page.getByTestId("reanalyze")).toHaveCount(0);
  // The user administration page stays out of reach as well.
  await page.goto("/users");
  await expect(page).toHaveURL(/\/dashboard$/);
});
