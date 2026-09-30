// An environment can be corrected after it was created. The name and notes are
// edited in place; the measurements never are, because tests that already ran
// keep the values they were run with, so changing them adds a revision.
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

async function createEnvironment(page, name) {
  await page.goto("/environments/new");
  await page.getByTestId("environment-name").fill(name);
  await page.getByTestId("environment-paradigm").click();
  await page.getByTestId("environment-paradigm-OPEN_FIELD").click();
  await expect(page.getByTestId("apparatus-arena_width_cm")).toBeVisible({ timeout: 10_000 });
  await page.getByTestId("apparatus-arena_width_cm").fill("50");
  await page.getByTestId("apparatus-arena_height_cm").fill("50");
  await page.getByTestId("environment-save").click();
  await expect(page).toHaveURL(/\/environments\/[0-9a-f-]{36}$/);
}

const unique = (label) => `${label} ${Date.now().toString(36).toUpperCase()}`;

test("the name and notes of an environment can be changed", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await createEnvironment(page, unique("E2E Old"));

  const renamed = unique("E2E Renamed");
  await page.getByTestId("environment-edit").click();
  await page.getByTestId("environment-edit-name").fill(renamed);
  await page.getByTestId("environment-edit-notes").fill("Camera above the arena");
  await page.getByTestId("environment-edit-save").click();

  await expect(page.getByTestId("error")).toHaveCount(0);
  await expect(page.getByTestId("environment-edit-form")).toHaveCount(0);
  await expect(page.getByRole("heading", { name: renamed })).toBeVisible({ timeout: 10_000 });
  await expect(page.getByText("Camera above the arena")).toBeVisible();

  await page.goto("/environments");
  await expect(page.getByTestId("environment").filter({ hasText: renamed })).toHaveCount(1);
});

test("changing the measurements adds a revision that has to be calibrated", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await createEnvironment(page, unique("E2E Rev"));
  await expect(page.getByTestId("environment-revision")).toHaveCount(1);

  await page.getByTestId("environment-new-revision").click();
  const form = page.getByTestId("revision-form");
  await expect(form).toBeVisible();
  // The form starts from the measurements in use.
  await expect(form.getByTestId("revision-arena_width_cm")).toHaveValue("50");
  await form.getByTestId("revision-arena_width_cm").fill("60");
  await form.getByTestId("revision-notes").fill("Width was entered wrong");
  await form.getByTestId("revision-save").click();

  await expect(page.getByTestId("error")).toHaveCount(0);
  await expect(page.getByTestId("environment-revision")).toHaveCount(2, { timeout: 10_000 });
  // The newest revision comes first and is the one to calibrate.
  const newest = page.getByTestId("environment-revision").first();
  await expect(newest).toHaveAttribute("data-revision", "2");
  await expect(newest.getByTestId("environment-calibration")).toHaveAttribute("data-status", "WAITING_FOR_CALIBRATION");
});

test("a viewer cannot edit an environment", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await page.goto(`/environments/${data.environmentId}`);

  await expect(page.getByTestId("environment-revision")).toBeVisible();
  await expect(page.getByTestId("environment-edit")).toHaveCount(0);
  await expect(page.getByTestId("environment-new-revision")).toHaveCount(0);
});

test("a test says which apparatus and revision it was run with", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  await page.goto(`/tests/${data.testId}`);
  const line = page.getByTestId("test-environment");
  await expect(line).toContainText("E2E arena");
  // Revisions never change, so a test always names the one it was planned with.
  await expect(line.getByTestId("test-revision")).toHaveAttribute("data-revision", "1");
  await line.getByRole("link", { name: "E2E arena" }).click();
  await expect(page).toHaveURL(new RegExp(`/environments/${data.environmentId}$`));

  await page.goto("/tests");
  const cell = page.getByTestId("all-tests-environment").first();
  await expect(cell).toContainText("E2E arena");
});
