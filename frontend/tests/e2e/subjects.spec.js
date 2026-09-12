// Subjects can be created and edited from the panel. Until now the screen was
// read only, so a laboratory had no way to register an animal at all.
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

// The seeded test is shared by every run, so the subject list keeps whatever
// earlier runs created. A unique code keeps each run independent of that.
const uniqueCode = () => `E2E-S-${Date.now().toString(36).toUpperCase()}`;

test("a subject can be registered and then corrected", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  const code = uniqueCode();
  await page.goto("/subjects");
  await page.getByTestId("subject-new").click();
  await expect(page).toHaveURL(/\/subjects\/new$/);

  await page.getByTestId("subject-code").fill(code);
  await page.getByTestId("subject-species").selectOption("RAT");
  await page.getByTestId("subject-sex").selectOption("FEMALE");
  await page.getByTestId("subject-strain").fill("C57BL/6");
  await page.getByTestId("subject-notes").fill("registered by the browser test");
  await page.getByTestId("subject-save").click();

  // Saving returns to the list, and the API has the subject: it comes back
  // through the same listing endpoint the table reads.
  await expect(page).toHaveURL(/\/subjects$/);
  await expect(page.getByTestId("error")).toHaveCount(0);
  // The search box carries no test hook and its placeholder is translated, so
  // it is addressed by its class.
  await page.locator(".dt-search").fill(code);
  await expect(page.getByText(code, { exact: true })).toBeVisible({ timeout: 10_000 });

  // Correcting it keeps the same record rather than creating a second one.
  await page.getByTestId(`subject-edit-${code}`).click();
  await expect(page).toHaveURL(/\/subjects\/[0-9a-f-]{36}$/);
  await expect(page.getByTestId("subject-code")).toHaveValue(code);
  await page.getByTestId("subject-sex").selectOption("MALE");
  await page.getByTestId("subject-save").click();

  await expect(page).toHaveURL(/\/subjects$/);
  await page.locator(".dt-search").fill(code);
  await expect(page.getByText(code, { exact: true })).toBeVisible({ timeout: 10_000 });
  // The panel runs in Turkish by default, so the sex is read from the attribute
  // rather than the label: the edit has to have reached the API, not just the page.
  const row = page.locator("tr", { hasText: code });
  await expect(row.locator("[data-sex]")).toHaveAttribute("data-sex", "MALE");
});

test("a viewer cannot register a subject", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);

  // The list is readable, but the write controls are not offered.
  await page.goto("/subjects");
  await expect(page.getByTestId("subject-new")).toHaveCount(0);

  // And the form stays out of reach even when the address is typed by hand:
  // the route requires subject:write.
  await page.goto("/subjects/new");
  await expect(page).toHaveURL(/\/experiments$/);
});
