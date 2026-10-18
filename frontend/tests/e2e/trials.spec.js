// Paradigm trials: a paradigm tried on one video in one arena, outside any
// experiment. The seeded trial has a succeeded automatic analysis; a new
// trial is created, filmed, calibrated on its own video and deleted; a
// viewer reads trials but changes nothing.
import { expect, test } from "@playwright/test";
import { chooseValue, confirm, fill, fillCalibration, signIn, unique } from "./helpers.js";

const VIDEO = "tests/fixtures/analyzed.mp4";

test("a seeded trial shows its analysis and is analysed again with a changed threshold", async ({ page, request }) => {
  const { data } = await signIn(page, request);
  await page.goto("/trials");
  await page.locator(`[data-test="trial-row"][data-name="${data.trial.name}"]`).getByTestId("trial-open").click();
  await expect(page).toHaveURL(new RegExp(`/trials/${data.trial.id}$`));

  await expect(page.getByTestId("trial-arena")).toContainText("Pool A");
  await expect(page.getByTestId("trial-calibration-source")).toHaveAttribute("data-source", "ENVIRONMENT");
  await expect(page.getByTestId("trial-video-player")).toBeVisible();

  const automatic = page.locator('[data-test="trial-analysis"][data-trigger="AUTOMATIC"]');
  await expect(automatic).toHaveAttribute("data-status", "SUCCEEDED");
  await expect(automatic.getByTestId("trial-metric-path_length_cm")).toContainText("cm");
  await expect(automatic.getByTestId("qc")).toHaveAttribute("data-passed", "true");
  // The overlay video and the events are read from the job's routes when opened.
  await automatic.getByTestId("trial-analysis-outputs").locator(".n-collapse-item__header").click();
  await expect(automatic.getByTestId("overlay-video")).toBeVisible();
  await expect(automatic.getByTestId("event").first()).toBeVisible();

  // Analyse again with a narrower wall annulus; only that value is sent.
  await page.getByTestId("trial-analyse-again").click();
  const form = page.getByTestId("analyse-again-form");
  await expect(form.locator('[data-test="again-wall_annulus_width_cm"]')).toHaveValue("12");
  await form.locator('[data-test="again-wall_annulus_width_cm"]').fill("8");
  await expect(page.getByTestId("analyse-again-changed")).toContainText("1");
  await page.getByTestId("analyse-again-submit").click();
  const manual = page.locator('[data-test="trial-analysis"][data-trigger="MANUAL"]');
  await expect(manual).toHaveAttribute("data-status", "QUEUED");
  await expect(manual.getByTestId("trial-analysis-overrides")).toContainText("8");
  // The newest analysis is listed first.
  await expect(page.getByTestId("trial-analysis").first()).toHaveAttribute("data-trigger", "MANUAL");
});

test("a trial is created, filmed, calibrated on its own video and deleted", async ({ page, request }) => {
  const { data } = await signIn(page, request);
  const name = unique("TRIAL");
  await page.goto("/trials");
  await page.getByTestId("trial-new").click();
  await fill(page, "trial-form-name", name);
  await chooseValue(page, "trial-form-paradigm", "MWM");
  await chooseValue(page, "trial-form-arena", data.arenas.poolB);
  await page.getByTestId("trial-form-save").click();
  await expect(page).toHaveURL(/\/trials\/[0-9a-f-]+$/);
  await expect(page.getByTestId("trial-arena")).toContainText("Pool B");
  await expect(page.getByTestId("trial-calibration")).toHaveAttribute("data-status", "WAITING_FOR_VIDEO");
  await expect(page.getByTestId("trial-analyses-none")).toBeVisible();

  // The video goes straight to storage and is verified; the arena's
  // calibration applies, so the automatic analysis is queued.
  await page.getByTestId("trial-video-input").setInputFiles(VIDEO);
  await expect(page.getByTestId("trial-video-current")).toHaveAttribute("data-status", "VERIFIED", { timeout: 20_000 });
  await expect(page.getByTestId("trial-calibration-source")).toHaveAttribute("data-source", "ENVIRONMENT");
  await expect(page.locator('[data-test="trial-analysis"][data-trigger="AUTOMATIC"]')).toHaveAttribute("data-status", "QUEUED");

  // A calibration on the trial's own video replaces the arena's for this trial only.
  await page.getByTestId("trial-calibrate").click();
  const dialog = page.getByTestId("calibration-dialog");
  await fillCalibration(dialog);
  await dialog.getByTestId("calibration-save").click();
  await expect(dialog.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-status", "VALID", { timeout: 10_000 });
  await dialog.getByTestId("calibration-cancel").click();
  await expect(page.getByTestId("trial-calibration-source")).toHaveAttribute("data-source", "TRIAL");

  // Deleting asks first and returns to the list, where the trial is gone.
  await page.getByTestId("trial-delete").click();
  await confirm(page);
  await expect(page).toHaveURL(/\/trials$/);
  await expect(page.getByTestId("trial-row").first()).toBeVisible();
  await expect(page.locator(`[data-test="trial-row"][data-name="${name}"]`)).toHaveCount(0);
});

test("a viewer reads trials but cannot start or change one", async ({ page, request }) => {
  const { data } = await signIn(page, request, "VIEWER");
  await page.goto("/trials");
  await expect(page.locator(`[data-test="trial-row"][data-name="${data.trial.name}"]`)).toBeVisible();
  await expect(page.getByTestId("trial-new")).toHaveCount(0);
  await page.goto(`/trials/${data.trial.id}`);
  await expect(page.getByTestId("trial-analysis").first()).toBeVisible();
  for (const id of ["trial-delete", "trial-analyse-again", "trial-video-upload", "trial-video-replace", "trial-calibrate"]) {
    await expect(page.getByTestId(id), id).toHaveCount(0);
  }
});
