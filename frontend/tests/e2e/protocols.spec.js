// A protocol says what a test actually does: which apparatus revision, which
// trial type, how many trials. The panel listed none of this and could create
// none of it, so a study could be opened but never given a procedure.
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

// The seeded experiment is shared by every run, so a unique name keeps each run
// independent of the protocols earlier runs added.
const uniqueName = () => `E2E Protocol ${Date.now().toString(36).toUpperCase()}`;

test("a protocol can be written for an experiment", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  const name = uniqueName();
  await page.goto(`/experiments/${data.experimentId}`);
  await expect(page.getByTestId("protocol-form")).toBeVisible();

  await page.getByTestId("protocol-name").fill(name);

  // The apparatus revisions of the whole laboratory are the choices. The seed
  // created one, so the first real option is selectable.
  const revision = page.getByTestId("step-revision-0");
  const options = revision.locator("option");
  await expect(options).not.toHaveCount(1, { timeout: 10_000 });
  const value = await options.nth(1).getAttribute("value");
  await revision.selectOption(value);

  await page.getByTestId("step-trial-type-0").fill("STANDARD");
  await page.getByTestId("step-trials-0").fill("3");
  await page.getByTestId("protocol-save").click();

  // It appears in the list at version 1, which is what a test can then be
  // planned against.
  const created = page.getByTestId("protocol").filter({ hasText: name });
  await expect(created).toHaveCount(1, { timeout: 10_000 });
  await expect(created).toContainText("1");
  await expect(page.getByTestId("error")).toHaveCount(0);
});

test("a second step can be added and removed before saving", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/experiments/${data.experimentId}`);

  await expect(page.getByTestId("protocol-step")).toHaveCount(1);
  await page.getByTestId("protocol-add-step").click();
  await expect(page.getByTestId("protocol-step")).toHaveCount(2);

  // Removing is offered only while more than one step exists, because a
  // protocol without a step is not a protocol.
  await page.getByTestId("step-remove-1").click();
  await expect(page.getByTestId("protocol-step")).toHaveCount(1);
  await expect(page.getByTestId("step-remove-0")).toHaveCount(0);
});

test("a viewer cannot write a protocol", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);

  await page.goto(`/experiments/${data.experimentId}`);
  // The protocols of the experiment stay readable.
  await expect(page.getByTestId("protocol")).not.toHaveCount(0);
  // Writing one needs apparatus:write, which a viewer does not have.
  await expect(page.getByTestId("protocol-form")).toHaveCount(0);
});
