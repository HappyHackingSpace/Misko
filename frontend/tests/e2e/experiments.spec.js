// Experiments can be created from the panel, and a group can be added to one.
// Until now both screens were read only, so a study could not be started at all.
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

// The seeded data is shared by every run, so a unique code keeps each run
// independent of what earlier runs created.
const uniqueCode = () => `E2E-X-${Date.now().toString(36).toUpperCase()}`;

test("an experiment can be created and given a group", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  const code = uniqueCode();
  await page.goto("/experiments");
  await page.getByTestId("experiment-new").click();
  await expect(page).toHaveURL(/\/experiments\/new$/);

  await page.getByTestId("experiment-code").fill(code);
  await page.getByTestId("experiment-title").fill("Browser test study");
  await page.getByTestId("experiment-description").fill("created by the browser test");
  await page.getByTestId("experiment-requires-control").check();
  await page.getByTestId("experiment-save").click();

  // Saving lands on the detail screen of the experiment that was just created,
  // which is where its groups live.
  await expect(page).toHaveURL(/\/experiments\/[0-9a-f-]{36}$/);
  await expect(page.getByTestId("error")).toHaveCount(0);
  await expect(page.getByText(code, { exact: false }).first()).toBeVisible();

  // A design that requires a control group starts without one, so adding it is
  // the next thing the laboratory has to do.
  await expect(page.getByTestId("group")).toHaveCount(0);
  await page.getByTestId("group-name").fill("Control A");
  await page.getByTestId("group-role").selectOption("CONTROL");
  await page.getByTestId("group-target-size").fill("8");
  await page.getByTestId("group-add").click();

  const group = page.getByTestId("group").filter({ hasText: "Control A" });
  await expect(group).toHaveCount(1, { timeout: 10_000 });
  await expect(group.locator("[data-role]")).toHaveAttribute("data-role", "CONTROL");
});

test("a viewer cannot create an experiment", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);

  await page.goto("/experiments");
  await expect(page.getByTestId("experiment-new")).toHaveCount(0);

  // The group form belongs to study:write as well.
  await page.goto(`/experiments/${data.experimentId}`);
  await expect(page.getByTestId("group-add")).toHaveCount(0);

  // And the form stays out of reach when the address is typed by hand.
  await page.goto("/experiments/new");
  await expect(page).toHaveURL(/\/dashboard$/);
});
