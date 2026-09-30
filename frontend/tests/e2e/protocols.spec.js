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

// The dropdowns are Naive UI selects, not native ones: open one and pick an
// option by the hook every option carries (its value travels in data-value).
const optionsOf = (page, testId) => page.locator(`.n-base-select-option [data-test="${testId}-option"]`);

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
  // The revisions belong to an environment, so that dropdown stays shut until one
  // is chosen.
  await expect(page.locator('[data-test="step-revision-0"] .n-base-selection--disabled')).toHaveCount(1, { timeout: 10_000 });
  await page.getByTestId("step-environment-0").click();
  await expect(optionsOf(page, "step-environment")).not.toHaveCount(0, { timeout: 10_000 });
  await optionsOf(page, "step-environment").first().click();

  await expect(page.locator('[data-test="step-revision-0"] .n-base-selection--disabled')).toHaveCount(0);
  await page.getByTestId("step-revision-0").click();
  // The newest revision is marked and offered first.
  await expect(page.getByTestId("step-revision-badge").first()).toBeVisible();
  await optionsOf(page, "step-revision").first().click();

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
