// A test is the unit of work of the whole laboratory, and until now the panel
// could only open one that the seed had already planned. Planning pins a
// subject's enrollment to one step of one protocol version at a scheduled time.
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

// The seeded experiment is shared by every run, so each run plans its test at a
// minute of its own and finds that row rather than counting rows.
function uniqueFutureMinute() {
  const minutes = 60 + Math.floor(Math.random() * 100_000);
  const when = new Date(Date.now() + minutes * 60_000);
  when.setSeconds(0, 0);
  return when;
}

const localValue = (when) => {
  const local = new Date(when.getTime() - when.getTimezoneOffset() * 60_000);
  return local.toISOString().slice(0, 16);
};

// Some fixtures are cheaper to build through the API than by driving screens
// that do not exist yet, such as adding a second version to a protocol.
async function asAdmin(request, data) {
  const response = await request.post(`${API}/api/auth/login`, {
    data: { email: data.adminEmail, password: data.adminPassword },
  });
  expect(response.ok()).toBeTruthy();
  const { token } = await response.json();
  const headers = { Authorization: `Bearer ${token}` };
  // A refused fixture call has to say what the API said, or the failure reads as
  // "false" and the real reason stays in the server.
  const ok = async (response, method, path) => {
    if (!response.ok()) throw new Error(`${method} ${path} answered ${response.status()}: ${await response.text()}`);
    return response.json();
  };
  const get = async (path) => ok(await request.get(`${API}/api${path}`, { headers }), "GET", path);
  const post = async (path, body) => ok(await request.post(`${API}/api${path}`, { headers, data: body }), "POST", path);
  return { get, post };
}

test("a test can be planned for an enrolled subject", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/experiments/${data.experimentId}`);

  await expect(page.getByTestId("plan-form")).toBeVisible();
  // The subject is chosen by its code, not by an identifier.
  await page.getByTestId("plan-enrollment").selectOption({ label: data.subjectCode });

  const protocols = page.getByTestId("plan-protocol").locator("option");
  await expect(protocols).not.toHaveCount(1, { timeout: 10_000 });
  await page.getByTestId("plan-protocol").selectOption(await protocols.nth(1).getAttribute("value"));

  // Choosing a protocol brings its versions, and one of them with one of its
  // steps is what the plan carries.
  await expect(page.getByTestId("plan-version").locator("option")).not.toHaveCount(0);
  await expect(page.getByTestId("plan-step").locator("option")).not.toHaveCount(0);

  const when = uniqueFutureMinute();
  await page.getByTestId("plan-scheduled").fill(localValue(when));
  await page.getByTestId("plan-save").click();

  // The instant travels in an attribute, so this holds in any language and any
  // locale's rendering of a date.
  await expect
    .poll(
      async () => {
        const scheduled = await page.getByTestId("test-row").evaluateAll((rows) => rows.map((row) => row.dataset.scheduled));
        return scheduled.some((value) => new Date(value).getTime() === when.getTime());
      },
      { timeout: 10_000 },
    )
    .toBe(true);
});

test("the plan follows the newest version of the chosen protocol", async ({ page, request }) => {
  const data = await seed(request);
  const api = await asAdmin(request, data);

  // A protocol of this run's own, revised once. The paradigm of the seeded
  // apparatus defines one trial type, so the two versions differ in how many
  // trials they run; that number is on the step the screen offers.
  const environments = await api.get("/environments");
  const revisions = await api.get(`/environments/${environments.data[0].id}/revisions`);
  const revision = revisions.data[0];
  const step = (trials) => ({
    position: 1,
    paradigmKey: revision.paradigmKey,
    paradigmVersion: revision.paradigmVersion,
    environmentRevisionId: revision.id,
    trialType: "STANDARD",
    trials,
    interTrialIntervalS: 0,
    session: {},
    notes: "",
  });

  const name = `E2E Revised ${Date.now().toString(36).toUpperCase()}`;
  const created = await api.post(`/experiments/${data.experimentId}/protocols`, {
    name,
    description: "",
    version: { notes: "", steps: [step(1)] },
  });
  await api.post(`/experiments/${data.experimentId}/protocols/${created.protocol.id}/versions`, {
    notes: "",
    steps: [step(7)],
  });

  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/experiments/${data.experimentId}`);

  // Nothing is offered before a protocol is chosen, because a step belongs to a
  // version and a version belongs to a protocol.
  await expect(page.getByTestId("plan-version").locator("option")).toHaveCount(0);
  await expect(page.getByTestId("plan-step").locator("option")).toHaveCount(0);

  await page.getByTestId("plan-protocol").selectOption({ label: name });

  // Both revisions are offered, and the plan defaults to the newest: the study
  // runs the procedure it revised to, not the one it left behind. The second
  // version runs seven trials where the first ran one.
  await expect(page.getByTestId("plan-version").locator("option")).toHaveCount(2);
  await expect(page.getByTestId("plan-step").locator("option")).toHaveCount(1, { timeout: 10_000 });
  await expect(page.getByTestId("plan-step").locator("option")).toContainText("7");
});

test("a viewer cannot plan a test", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await page.goto(`/experiments/${data.experimentId}`);

  // The tests of the experiment stay readable.
  await expect(page.getByTestId("test-row")).not.toHaveCount(0);
  // Planning one needs test:write, which a viewer does not have.
  await expect(page.getByTestId("plan-form")).toHaveCount(0);
});
