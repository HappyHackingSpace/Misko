// Test sessions in the seeded study: the worklist and its filters in the
// address, planning a session with two animals in two arenas, changing the
// animals while it is planned, running it (trials per animal, the video and a
// calibration for one animal's arena on that video) and completing it, and
// cancelling another with a reason.
import { expect, test } from "@playwright/test";
import { chooseValue, confirm, fillCalibration, signIn } from "./helpers.js";

const VIDEO = "tests/fixtures/analyzed.mp4";

test("the worklist keeps its view and filters in the address", async ({ page, request }) => {
  const { data } = await signIn(page, request);
  const link = (id) => page.locator(`[data-test="worklist-link"][href="/tests/${id}"]`);

  await page.goto(`/tests?experimentId=${data.experimentId}`);
  // Today: the session in progress since this morning.
  await expect(link(data.runningTestId)).toBeVisible();
  await expect(link(data.plannedTestId)).toHaveCount(0);

  // Overdue: the baseline session planned on an earlier day.
  await page.getByTestId("worklist-view-overdue").click();
  await expect(page).toHaveURL(/view=overdue/);
  await expect(link(data.plannedTestId)).toBeVisible();
  await expect(link(data.runningTestId)).toHaveCount(0);

  // A status filter narrows any view and is kept in the address too.
  await page.goto(`/tests?view=all&experimentId=${data.experimentId}&status=IN_PROGRESS`);
  await expect(link(data.runningTestId)).toBeVisible();
  await expect(link(data.day14TestId)).toBeVisible();
  for (const status of await page.getByTestId("worklist-status").evaluateAll((tags) => tags.map((t) => t.dataset.status))) {
    expect(status).toBe("IN_PROGRESS");
  }
  await page.getByTestId("worklist-status-filter").hover();
  await page.locator('[data-test="worklist-status-filter"] .n-base-clear').click();
  await expect(page).not.toHaveURL(/status=/);
  await expect(page).toHaveURL(/view=all/);
});

test("a session with two animals is planned, run and completed", async ({ page, request }) => {
  test.slow();
  const { data } = await signIn(page, request);
  await page.goto(`/experiments/${data.experimentId}?tab=tests`);

  // Plan: Day 7, two animals, each in its own arena.
  await page.getByTestId("plan-session").click();
  const drawer = page.getByTestId("plan-drawer");
  await chooseValue(page, "plan-timepoint", data.timepoints.day7);
  await chooseValue(page, "plan-animal-0", data.allocations.M05, { search: "M05" });
  await chooseValue(page, "plan-arena-0", data.arenas.poolA);
  await drawer.getByTestId("plan-add-animal").click();
  await chooseValue(page, "plan-animal-1", data.allocations.M08, { search: "M08" });
  await chooseValue(page, "plan-arena-1", data.arenas.poolB);
  await drawer.getByTestId("plan-save").click();
  await expect(drawer).toBeHidden();

  const section = page.locator('[data-test="timepoint-section"][data-timepoint="Day 7"]');
  const row = section.locator("tbody tr").filter({ hasText: "M05" });
  await expect(row.getByTestId("session-animal-chip")).toHaveCount(2);
  await expect(row.getByTestId("session-status")).toHaveAttribute("data-status", "PLANNED");
  await row.getByTestId("session-link").click();
  await expect(page).toHaveURL(/\/tests\/[0-9a-f-]{36}$/);
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "PLANNED");
  await expect(page.getByTestId("test-animal")).toHaveCount(2);

  // While planned, animals can be added and removed.
  await page.getByTestId("edit-animals").click();
  const editor = page.getByTestId("session-animals-editor");
  await chooseValue(page, "add-animal", data.allocations.M01, { search: "M01" });
  await chooseValue(page, "add-arena", data.arenas.poolC);
  await editor.getByTestId("add-animal-save").click();
  await expect(editor.getByTestId("session-animal")).toHaveCount(3);
  await editor.locator('[data-test="session-animal"][data-animal="M01"]').getByTestId("remove-animal").click();
  await confirm(page);
  await expect(editor.getByTestId("session-animal")).toHaveCount(2);
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("test-animal")).toHaveCount(2);

  // Start; the video panel waits for the session to run.
  await expect(page.getByTestId("video-not-started")).toBeVisible();
  await page.getByTestId("start-test").click();
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "IN_PROGRESS");
  await expect(page.getByTestId("edit-animals")).toHaveCount(0);

  // Completing waits for a trial of every animal.
  await expect(page.getByTestId("needs-trials")).toBeVisible();
  await expect(page.getByTestId("complete-test")).toBeDisabled();
  const m05 = page.locator('[data-test="test-animal"][data-animal="M05"]');
  const m08 = page.locator('[data-test="test-animal"][data-animal="M08"]');
  await m05.getByTestId("trial-open").click();
  await m05.getByTestId("trial-save").click();
  await expect(m05.getByTestId("trial")).toHaveCount(1);
  await expect(m05.getByTestId("trial-count")).toContainText("1");
  await expect(page.getByTestId("complete-test")).toBeDisabled();

  // The session's one video, then a replacement; the first stays listed.
  await page.getByTestId("video-input").setInputFiles(VIDEO);
  const current = page.getByTestId("video-current");
  await expect(current).toBeVisible({ timeout: 30_000 });
  const first = await current.getAttribute("data-recording");
  const chooser = page.waitForEvent("filechooser");
  await page.getByTestId("video-replace").click();
  await confirm(page);
  await (await chooser).setFiles(VIDEO);
  await expect(current).not.toHaveAttribute("data-recording", first, { timeout: 30_000 });
  await page.getByTestId("video-panel").locator(".n-collapse-item__header").click();
  await expect(page.getByTestId("video-earlier")).toHaveCount(1);

  // Each animal's part of the frame uses its arena's calibration; the camera
  // moved for M05, so it is calibrated on this video only.
  const calibration = m05.getByTestId("animal-calibration");
  await expect(calibration.getByTestId("calibration-source")).toHaveAttribute("data-source", "ENVIRONMENT");
  await calibration.getByTestId("calibrate").click();
  const dialog = page.getByTestId("calibration-dialog");
  await expect(dialog.getByTestId("calibration-reference-warning")).toBeVisible();
  await fillCalibration(dialog);
  await dialog.getByTestId("reference-frame").fill("0");
  await dialog.getByTestId("calibration-save").click();
  // The dialog stays open to show how the calibration came out.
  await expect(dialog.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-status", "VALID", { timeout: 10_000 });
  await dialog.getByTestId("calibration-cancel").click();
  await expect(dialog).toBeHidden();
  await expect(calibration.getByTestId("calibration-source")).toHaveAttribute("data-source", "OVERRIDE");
  await expect(m08.getByTestId("calibration-source")).toHaveAttribute("data-source", "ENVIRONMENT");

  await m08.getByTestId("trial-open").click();
  await m08.getByTestId("trial-save").click();
  await expect(m08.getByTestId("trial")).toHaveCount(1);
  await expect(page.getByTestId("needs-trials")).toHaveCount(0);
  await page.getByTestId("complete-test").click();
  await confirm(page);
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "COMPLETED");
});

test("a planned session is cancelled with a reason", async ({ page, request }) => {
  const { data, api } = await signIn(page, request);
  const planned = await api("POST", `/experiments/${data.experimentId}/tests`, {
    timepointId: data.timepoints.day7,
    protocolVersionId: data.protocolVersionId,
    stepPosition: 1,
    subjects: [{ allocationId: data.allocations.M03, environmentRevisionId: data.arenas.poolC }],
  });
  await page.goto(`/tests/${planned.id}`);
  await page.getByTestId("cancel-open").click();
  await expect(page.getByTestId("cancel-test")).toBeDisabled();
  await page.getByTestId("cancel-reason-input").fill("Arena lamp broken");
  await page.getByTestId("cancel-test").click();
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "CANCELLED");
  await expect(page.getByTestId("cancel-reason")).toContainText("Arena lamp broken");
  await expect(page.getByTestId("start-test")).toHaveCount(0);
});
