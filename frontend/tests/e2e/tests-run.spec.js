// Planning a test records what will be done; running it records what was done.
// A test starts, collects trials, and completes only once at least one trial
// exists, because a completed test with no trial is a claim with no evidence.
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

// A refused fixture call has to say what the API said, or the failure reads as
// "false" and the real reason stays in the server.
async function asAdmin(request, data) {
  const response = await request.post(`${API}/api/auth/login`, {
    data: { email: data.adminEmail, password: data.adminPassword },
  });
  expect(response.ok()).toBeTruthy();
  const { token } = await response.json();
  const headers = { Authorization: `Bearer ${token}` };
  const ok = async (r, method, path) => {
    if (!r.ok()) throw new Error(`${method} ${path} answered ${r.status()}: ${await r.text()}`);
    return r.json();
  };
  return {
    get: async (path) => ok(await request.get(`${API}/api${path}`, { headers }), "GET", path),
    post: async (path, body) => ok(await request.post(`${API}/api${path}`, { headers, data: body }), "POST", path),
  };
}

// The seeded test is already in progress and is shared by every run, so a test
// that needs a fresh PLANNED record plans one of its own.
async function planATest(api, data) {
  const enrollments = await api.get(`/experiments/${data.experimentId}/enrollments?pageSize=100`);
  const protocols = await api.get(`/experiments/${data.experimentId}/protocols`);
  const versions = await api.get(`/experiments/${data.experimentId}/protocols/${protocols.data[0].id}/versions`);
  const version = versions.data.reduce((newest, v) => (v.number > newest.number ? v : newest));
  const planned = await api.post(`/experiments/${data.experimentId}/tests`, {
    enrollmentId: enrollments.data[0].id,
    phaseId: "",
    protocolVersionId: version.id,
    stepPosition: version.steps[0].position,
    scheduledAt: new Date(Date.now() - 60_000).toISOString(),
    notes: "",
  });
  return planned;
}

// The trial fields carry seconds, which is what lets a trial be recorded at the
// moment it happened rather than at the next whole minute.
const SECONDS = /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}$/;

test("a planned test is started, collects a trial, and completes", async ({ page, request }) => {
  const data = await seed(request);
  const api = await asAdmin(request, data);
  const planned = await planATest(api, data);

  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${planned.id}`);

  // The raw status travels in an attribute, so this holds in any language.
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "PLANNED");
  // Nothing can be completed before it has started.
  await expect(page.getByTestId("complete-test")).toHaveCount(0);
  await expect(page.getByTestId("trial-form")).toHaveCount(0);

  await page.getByTestId("start-test").click();
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "IN_PROGRESS", { timeout: 10_000 });

  // A test in progress may not be completed until a trial is on the record.
  await expect(page.getByTestId("complete-test")).toBeDisabled();
  await expect(page.getByTestId("needs-trial")).toBeVisible();

  // The start the screen offers is the moment it last read the test, which is
  // after the start the API recorded; typing a time here would race that by
  // whole seconds. The end is left empty: an open trial is one that has started
  // and not yet ended, and a test completes over it.
  // Within the second the test started in, the offered start is that start
  // rounded up, which is a moment ahead. Letting the clock pass that second and
  // reading the screen again offers the current second instead, so the trial is
  // recorded in the past and the completion that follows is accepted.
  await page.waitForTimeout(1100);
  await page.reload();
  // It carries seconds, not just minutes: a whole-minute default would sit
  // before the test's own start and the API would refuse the trial.
  await expect(page.getByTestId("trial-start")).toHaveValue(SECONDS);
  await expect
    .poll(async () => new Date(await page.getByTestId("trial-start").inputValue()).getTime() <= Date.now())
    .toBe(true);
  await page.getByTestId("trial-repetition").fill("1");
  await page.getByTestId("trial-save").click();

  // A refused write leaves the screen unchanged, so the error is asserted too:
  // without this, a rejected trial reads as a slow one.
  await expect(page.getByTestId("error")).toHaveCount(0);
  await expect(page.getByTestId("trial")).toHaveCount(1, { timeout: 10_000 });
  await expect(page.getByTestId("needs-trial")).toHaveCount(0);

  await expect(page.getByTestId("complete-test")).toBeEnabled();
  await page.getByTestId("complete-test").click();
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "COMPLETED", { timeout: 10_000 });

  // A finished test offers no further transition.
  await expect(page.getByTestId("start-test")).toHaveCount(0);
  await expect(page.getByTestId("complete-test")).toHaveCount(0);
  await expect(page.getByTestId("trial-form")).toHaveCount(0);
});

test("the trial start offered is never before the test's own start", async ({ page, request }) => {
  const data = await seed(request);
  const api = await asAdmin(request, data);
  const planned = await planATest(api, data);

  // The API accepts a start up to two minutes ahead of its own clock, for a
  // browser whose clock runs fast. Starting a few seconds ahead makes the case
  // the screen has to handle plain: the current second is before the test's own
  // start, and a trial recorded there would be refused.
  const started = await api.post(`/tests/${planned.id}/start`, {
    startedAt: new Date(Date.now() + 5_000).toISOString(),
  });

  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${planned.id}`);

  const offered = await page.getByTestId("trial-start").inputValue();
  expect(offered).toMatch(SECONDS);
  expect(new Date(offered).getTime()).toBeGreaterThanOrEqual(new Date(started.startedAt).getTime());
});

test("cancelling asks for a reason and records it", async ({ page, request }) => {
  const data = await seed(request);
  const api = await asAdmin(request, data);
  const planned = await planATest(api, data);

  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${planned.id}`);

  // The API refuses an empty reason, so the screen does not offer to send one.
  await expect(page.getByTestId("cancel-test")).toBeDisabled();

  const reason = `Arena flooded ${Date.now().toString(36).toUpperCase()}`;
  await page.getByTestId("cancel-reason-input").fill(reason);
  await expect(page.getByTestId("cancel-test")).toBeEnabled();
  await page.getByTestId("cancel-test").click();

  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "CANCELLED", { timeout: 10_000 });
  // The reason is kept with the record, not only in the click that sent it.
  await expect(page.getByTestId("cancel-reason")).toContainText(reason);
  await expect(page.getByTestId("start-test")).toHaveCount(0);
});

test("a viewer reads the trials but runs nothing", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);

  // The seeded test is in progress, so every control would be on offer to a
  // role that had the permissions.
  await page.goto(`/tests/${data.testId}`);
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "IN_PROGRESS");
  await expect(page.getByTestId("trial-count")).toBeVisible();

  // Running needs test:run and cancelling needs test:write; a viewer has neither.
  await expect(page.getByTestId("complete-test")).toHaveCount(0);
  await expect(page.getByTestId("trial-form")).toHaveCount(0);
  await expect(page.getByTestId("cancel-test")).toHaveCount(0);
});
