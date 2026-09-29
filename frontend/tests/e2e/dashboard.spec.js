// The dashboard is the new landing page: what the lab's pipeline looks like
// right now, without opening an experiment. It reads one summary call.
import { expect, test } from "@playwright/test";

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

test("signing in lands on the dashboard and it shows the pipeline's shape", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await expect(page).toHaveURL(/\/dashboard$/);

  // Every status tile renders, whatever the count the shared fixture data
  // happens to add up to at this point in the suite.
  for (const key of ["planned", "inProgress", "completed", "cancelled", "queued", "running", "failed", "calibration"]) {
    await expect(page.locator(`[data-test="tile-${key}"]`)).toBeVisible();
  }

  // The seed's own published run is a fixed, one-time fixture, so it is
  // always somewhere in the recent activity feed.
  await expect(page.locator(`[data-test="recent-activity"] a[href="/tests/${data.testId}"]`).first()).toBeVisible({
    timeout: 10_000,
  });
});

test("a dashboard row opens its test", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  const row = page.locator(`[data-test="recent-activity"] a[href="/tests/${data.testId}"]`).first();
  await expect(row).toBeVisible({ timeout: 10_000 });
  await row.click();
  await expect(page).toHaveURL(new RegExp(`/tests/${data.testId}$`));
  await expect(page.getByTestId("test-status")).toBeVisible();
});
