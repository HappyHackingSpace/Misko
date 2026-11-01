// What a study's numbers look like before they are final: the Results tab
// counts only accepted results and says when some are still pending, the
// report exports as CSV, the reports page shows the same report, and the
// dashboard's tiles open the lists they count.
import { readFile } from "node:fs/promises";
import { expect, test } from "@playwright/test";
import { chooseValue, signIn, unique } from "./helpers.js";

test("the Results tab shows provisional numbers and exports them", async ({ page, request }) => {
  const { data } = await signIn(page, request);
  await page.goto(`/experiments/${data.experimentId}?tab=results`);

  // M04 (G2) and M06 (G3) are accepted at Day 14; M07 (G3) is still pending.
  const banner = page.getByTestId("report-provisional");
  await expect(banner).toBeVisible();
  await expect(page.getByTestId("report-review-link")).toHaveAttribute("href", new RegExp(`experimentId=${data.experimentId}.*status=PENDING`));
  const section = page.getByTestId("report-section-PRIMARY");
  const group = (code) => section.locator(`[data-test="report-group"][data-group="${code}"]`);
  // A distance shows to the millimetre; the export keeps full precision.
  await expect(group("G2")).toContainText(/127[.,]4 cm/);
  await expect(group("G3")).toContainText(/127[.,]4 cm/);
  // n counts accepted results only: one per group, the pending one waits.
  await expect(group("G2").locator("td").nth(1)).toHaveText("1");
  await expect(group("G3").locator("td").nth(1)).toHaveText("1");
  await expect(group("G1").locator("td").nth(1)).toHaveText("0");
  await expect(page.getByTestId("group-chart")).toBeVisible();

  // One row per animal as CSV.
  const download = page.waitForEvent("download");
  await page.getByTestId("report-export").click();
  await page.locator(".n-dropdown-option").filter({ hasText: "CSV, one row per subject" }).click();
  const file = await download;
  expect(file.suggestedFilename()).toMatch(/^E2E-.*\.csv$/);
  const csv = await readFile(await file.path(), "utf8");
  for (const code of ["M04", "M06", "M07"]) expect(csv).toContain(code);
  expect(csv).toContain("127.36");

  await page.getByTestId("report-review-link").click();
  await expect(page).toHaveURL(/\/review\?/);
  await expect(page.locator(`[data-test="review-row"][data-test-subject="${data.results.M07}"]`)).toBeVisible();
});

test("the reports page opens a study's report; a draft has none", async ({ page, request }) => {
  const { data, api } = await signIn(page, request);
  await page.goto("/reports");
  await chooseValue(page, "report-experiment", data.experimentId, { search: "E2E" });
  await expect(page).toHaveURL(new RegExp(`experimentId=${data.experimentId}`));
  await expect(page.getByTestId("report-section-PRIMARY")).toBeVisible();
  await expect(page.getByTestId("report-open-experiment")).toHaveAttribute("href", `/experiments/${data.experimentId}?tab=results`);

  const draft = await api("POST", "/experiments", { code: unique("DRAFT"), title: "Not started" });
  await page.goto(`/experiments/${draft.id}?tab=results`);
  await expect(page.getByTestId("results-draft")).toBeVisible();
});

test("dashboard tiles open the lists they count", async ({ page, request }) => {
  const { api } = await signIn(page, request);
  const draft = await api("POST", "/experiments", { code: unique("DRAFT"), title: "Not started" });
  const value = async (key) => Number((await page.getByTestId(`tile-${key}`).locator(".tile-value").innerText()).replace(/\D/g, ""));

  await page.goto("/dashboard");
  expect(await value("pending")).toBeGreaterThan(0);
  expect(await value("accepted")).toBeGreaterThan(0);
  await page.getByTestId("tile-pending").click();
  await expect(page).toHaveURL(/\/review\?status=PENDING$/);
  await expect(page.getByTestId("review-row").first()).toHaveAttribute("data-status", "PENDING");

  await page.goto("/dashboard");
  expect(await value("inProgress")).toBeGreaterThan(0);
  await page.getByTestId("tile-inProgress").click();
  await expect(page).toHaveURL(/\/tests\?status=IN_PROGRESS$/);
  await page.getByTestId("worklist-view-all").click();
  await expect(page.getByTestId("worklist-status").first()).toBeVisible();
  for (const status of await page.getByTestId("worklist-status").evaluateAll((tags) => tags.map((t) => t.dataset.status))) {
    expect(status).toBe("IN_PROGRESS");
  }

  await page.goto("/dashboard");
  await page.getByTestId("tile-experimentsRunning").click();
  await expect(page).toHaveURL(/\/experiments\?status=RUNNING$/);
  await expect(page.getByTestId("experiment-link").filter({ hasText: /^E2E$/ })).toBeVisible();
  await expect(page.getByTestId("experiment-status-filter")).toContainText("Running");
  await expect(page.getByTestId("experiment-link").filter({ hasText: draft.code })).toHaveCount(0);
});

test("an empty upcoming list says where sessions are planned", async ({ page, request }) => {
  await signIn(page, request);
  // The seeded lab always has planned sessions, so the summary is emptied here.
  await page.route("**/dashboard/summary", async (route) => {
    const summary = await (await route.fetch()).json();
    await route.fulfill({ json: { ...summary, upcomingTests: [] } });
  });
  await page.goto("/dashboard");
  const empty = page.getByTestId("upcoming-empty");
  await expect(empty).toContainText("Sessions are planned on the Tests tab of a running study.");
  await expect(empty.getByRole("link", { name: "How to plan a session" })).toHaveAttribute("href", /\/usage\/#planning-and-running-test-sessions$/);
  await empty.getByTestId("upcoming-running").click();
  await expect(page).toHaveURL(/\/experiments\?status=RUNNING$/);
});
