// The Tests page is the way back to a test without knowing which experiment it
// belongs to: the menu opens every test, and a row opens that test.
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

test("the menu lists every test and opens one", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);

  await page.getByTestId("nav-tests").click();
  await expect(page).toHaveURL(/\/tests$/);

  const row = page.locator(`[data-test="all-tests"] a[href="/tests/${data.testId}"]`);
  await expect(row).toHaveText(data.subjectCode);
  await expect(page.locator(`[data-test="all-tests"] a[href="/experiments/${data.experimentId}"]`).first()).toHaveText(
    data.experimentCode,
  );

  await row.click();
  await expect(page).toHaveURL(new RegExp(`/tests/${data.testId}$`));
  await expect(page.getByTestId("test-status")).toBeVisible();
});

test("the status filter narrows the list", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await page.goto("/tests");
  await expect(page.locator(`[data-test="all-tests"] a[href="/tests/${data.testId}"]`)).toBeVisible();

  await page.getByTestId("all-tests-status").click();
  await page.getByTestId("option-status-CANCELLED").click();
  await expect(page.locator(`[data-test="all-tests"] a[href="/tests/${data.testId}"]`)).toHaveCount(0);
});
