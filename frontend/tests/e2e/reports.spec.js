// Published results across an experiment. Every number the analysis produced
// lived only on the page of the test it came from, so nobody could read a whole
// experiment at once, compare its groups, or take the numbers away as a file.
import { readFile } from "node:fs/promises";
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

async function openReports(page, data) {
  await page.getByTestId("nav-reports").click();
  await expect(page.getByTestId("report-experiment")).toBeVisible();
  await chooseOption(page, "report-experiment", `option-experiment-${data.experimentId}`);
}

test("an experiment's metrics are listed and its groups summarised", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await openReports(page, data);

  // The seeded run published several metrics for one subject.
  await expect(page.getByTestId("metric-value").first()).toBeVisible({ timeout: 10_000 });
  const values = page.getByTestId("metric-value");
  expect(await values.count()).toBeGreaterThan(1);
  await expect(page.getByTestId("report-error")).toHaveCount(0);

  // Choosing one metric compares the groups of the experiment on it. The
  // seeded test belongs to no group, so the comparison has one row.
  await chooseOption(page, "report-metric", "option-metric-duration_s");
  const summary = page.getByTestId("report-summary");
  await expect(summary).toBeVisible({ timeout: 10_000 });
  await expect(page.getByTestId("summary-group")).toHaveCount(1);
  // The analysed clip is just under three seconds, and the summary reports it
  // as the mean of the one test it found.
  const mean = Number(await page.getByTestId("summary-mean").first().textContent());
  expect(mean).toBeGreaterThan(2);
  expect(mean).toBeLessThan(4);

  // The table now holds only that metric.
  for (const value of await page.getByTestId("metric-value").all()) {
    await expect(value).toHaveAttribute("data-metric", "duration_s");
  }

  // A summary compares the latest run of each test. Asking it to mix every run
  // of a test is a question the API refuses, so the screen stops offering one
  // rather than showing a refusal. It has to stay gone: a summary that merely
  // blinks out while a request is in flight would pass a one-off check.
  await chooseOption(page, "report-selection", "option-selection-all");
  await expect
    .poll(
      async () => {
        const seen = [];
        for (let i = 0; i < 6; i++) {
          seen.push(await page.getByTestId("report-summary").count());
          await page.waitForTimeout(250);
        }
        return Math.max(...seen);
      },
      { timeout: 15_000 },
    )
    .toBe(0);
  await expect(page.getByTestId("report-error")).toHaveCount(0);
});

test("the rows belong to the experiment that was chosen", async ({ page, request }) => {
  const data = await seed(request);

  // A second experiment of this run's own, with no results at all. The seeded
  // experiment is the only one with published metrics, so choosing the empty
  // one has to empty the table: without the filter its rows would still show.
  const login = await request.post(`${API}/api/auth/login`, {
    data: { email: data.adminEmail, password: data.adminPassword },
  });
  const { token } = await login.json();
  const code = `EMPTY${Date.now().toString(36).toUpperCase().slice(-5)}`;
  const created = await request.post(`${API}/api/experiments`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { code, title: "No results yet" },
  });
  expect(created.ok()).toBeTruthy();
  const empty = (await created.json()).id;

  await signIn(page, data.adminEmail, data.adminPassword);
  await openReports(page, data);
  await expect(page.getByTestId("metric-value").first()).toBeVisible({ timeout: 10_000 });

  await chooseOption(page, "report-experiment", `option-experiment-${empty}`);
  await expect(page.getByTestId("metric-value")).toHaveCount(0, { timeout: 10_000 });
  await expect(page.getByTestId("report-error")).toHaveCount(0);

  // And back again, so the emptiness is the filter talking rather than a
  // screen that stopped loading.
  await chooseOption(page, "report-experiment", `option-experiment-${data.experimentId}`);
  await expect(page.getByTestId("metric-value").first()).toBeVisible({ timeout: 10_000 });
});

test("the events of an experiment are listed with their kind", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await openReports(page, data);

  await page.getByTestId("tab-events").click();
  await expect(page.getByTestId("event-type").first()).toBeVisible({ timeout: 10_000 });

  // The seeded Open Field run published an immobility interval, a centre entry
  // and a centre interval.
  const types = await page.getByTestId("event-type").evaluateAll((nodes) => nodes.map((n) => n.dataset.event));
  expect(types).toContain("immobile");
  expect(types).toContain("center_entry");
  expect(types).toContain("in_center");
  await expect(page.getByTestId("report-error")).toHaveCount(0);
});

test("the rows are taken away as a file the API wrote", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await openReports(page, data);
  await expect(page.getByTestId("metric-value").first()).toBeVisible({ timeout: 10_000 });

  // The export is the API's own CSV over every matching row, not the page on
  // screen, so the file is asked for and saved rather than built in the browser.
  const [download] = await Promise.all([
    page.waitForEvent("download", { timeout: 20_000 }),
    page.getByTestId("report-export").click(),
  ]);
  expect(download.suggestedFilename()).toContain("metrics");

  // The export is UTF-16 LE with a BOM and tab separators so Excel opens it in any locale.
  const text = await readFile(await download.path(), "utf16le");
  const [header, ...rows] = text.trim().split("\n");
  // The header is the provenance a result has to carry to be traceable.
  expect(header).toContain("runId");
  expect(header).toContain("calibrationId");
  expect(header).toContain("subjectCode");
  expect(header).toContain("metricKey");
  expect(rows.length).toBeGreaterThan(1);

  // The filters on screen travel with the export: narrowing to one metric
  // narrows the file too, rather than downloading everything regardless.
  await chooseOption(page, "report-metric", "option-metric-duration_s");
  await expect(page.getByTestId("metric-value")).toHaveCount(1, { timeout: 10_000 });
  const [narrowed] = await Promise.all([
    page.waitForEvent("download", { timeout: 20_000 }),
    page.getByTestId("report-export").click(),
  ]);
  const narrowedRows = (await readFile(await narrowed.path(), "utf16le")).trim().split("\n").slice(1);
  expect(narrowedRows.length).toBeGreaterThan(0);
  expect(narrowedRows.length).toBeLessThan(rows.length);
  for (const row of narrowedRows) expect(row).toContain("duration_s");
});

test("reports offer no free-text search, because the API refuses one", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await openReports(page, data);
  await expect(page.getByTestId("metric-value").first()).toBeVisible({ timeout: 10_000 });

  // Every other list screen has a search box; these endpoints filter by ids and
  // keys instead, and a box that is always refused is worse than no box.
  await expect(page.getByTestId("list-search")).toHaveCount(0);
  await page.goto("/subjects");
  await expect(page.getByTestId("list-search")).toHaveCount(1);
});
