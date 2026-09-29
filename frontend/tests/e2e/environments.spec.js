// An apparatus can be defined from the panel. The screen was read only, and its
// own comment claimed a revision is created with the protocol that uses it,
// which left no way to register a physical setup at all.
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

// Naive UI's select is not a native <select>: open it, then click the option
// by its (locale-independent) test hook.
async function chooseOption(page, triggerTestId, optionTestId) {
  await page.getByTestId(triggerTestId).click();
  await page.getByTestId(optionTestId).click();
}

// The seeded data is shared by every run, so a unique name keeps each run
// independent of what earlier runs created.
const uniqueName = () => `E2E Arena ${Date.now().toString(36).toUpperCase()}`;

test("an apparatus can be defined from a paradigm template", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  const name = uniqueName();
  await page.goto("/environments");
  await page.getByTestId("environment-new").click();
  await expect(page).toHaveURL(/\/environments\/new$/);

  await page.getByTestId("environment-name").fill(name);
  await chooseOption(page, "environment-paradigm", "environment-paradigm-OPEN_FIELD");

  // The paradigm decides which physical values exist, and the form fills them
  // with what the published version considers normal.
  const width = page.getByTestId("apparatus-arena_width_cm");
  await expect(width).toBeVisible({ timeout: 10_000 });
  await expect(width).not.toHaveValue("");
  await width.fill("60");

  await page.getByTestId("environment-save").click();
  await expect(page).toHaveURL(/\/environments$/);
  await expect(page.getByTestId("error")).toHaveCount(0);

  const row = page.getByTestId("environment").filter({ hasText: name });
  await expect(row).toHaveCount(1, { timeout: 10_000 });
  await expect(row).toContainText("OPEN_FIELD");
});

test("a paradigm page offers to create an environment from it", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  await page.goto("/paradigms/OPEN_FIELD");
  await page.getByTestId("create-environment").click();

  // Arriving this way preselects the paradigm, so the apparatus fields are
  // already the ones that paradigm declares.
  await expect(page).toHaveURL(/\/environments\/new\?paradigm=OPEN_FIELD$/);
  await expect(page.getByTestId("environment-paradigm-OPEN_FIELD")).toBeVisible();
  await expect(page.getByTestId("apparatus-arena_width_cm")).toBeVisible({ timeout: 10_000 });
});

test("a viewer cannot define an apparatus", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);

  await page.goto("/environments");
  await expect(page.getByTestId("environment-new")).toHaveCount(0);

  await page.goto("/paradigms/OPEN_FIELD");
  await expect(page.getByTestId("create-environment")).toHaveCount(0);

  await page.goto("/environments/new");
  await expect(page).toHaveURL(/\/experiments$/);
});
