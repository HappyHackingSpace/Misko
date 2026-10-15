// Environments: an arena is defined from a paradigm's parameters, calibrated
// once for its fixed camera (the default for every video filmed in it), and
// changed through a new revision that keeps the earlier one.
import { expect, test } from "@playwright/test";
import { fill, fillCalibration, signIn, unique } from "./helpers.js";

test("an arena is created with its parameters, calibrated and revised", async ({ page, request }) => {
  await signIn(page, request);
  const name = unique("Arena");

  await page.goto("/environments");
  await page.getByTestId("environment-new").click();
  await expect(page).toHaveURL(/\/environments\/new$/);
  await fill(page, "environment-name", name);
  await page.getByTestId("environment-paradigm").click();
  await page.getByTestId("environment-paradigm-MWM").click();
  // The paradigm decides which physical values exist and fills in defaults.
  await expect(page.getByTestId("apparatus-tank_diameter_cm")).toBeVisible({ timeout: 10_000 });
  await fill(page, "apparatus-tank_diameter_cm", "120");
  await fill(page, "apparatus-platform_diameter_cm", "10");
  await fill(page, "apparatus-platform_x_cm", "30");
  await fill(page, "apparatus-platform_y_cm", "30");
  await page.getByTestId("environment-save").click();
  await expect(page).toHaveURL(/\/environments\/[0-9a-f-]{36}$/);
  await expect(page.getByTestId("environment-revision")).toHaveCount(1);

  // The default calibration of the arena, entered once.
  const section = page.getByTestId("environment-calibration");
  await expect(section).toHaveAttribute("data-status", "WAITING_FOR_CALIBRATION");
  await expect(section.getByTestId("calibration-waiting")).toBeVisible();
  await section.getByTestId("environment-calibrate").click();
  const form = section.getByTestId("calibration-form");
  // An arena has no video, so there is no reference frame to name.
  await expect(form.getByTestId("reference-frame")).toHaveCount(0);
  await fillCalibration(form);
  await form.getByTestId("calibration-save").click();
  await expect(section).toHaveAttribute("data-status", "CALIBRATED", { timeout: 10_000 });
  await expect(section.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-status", "VALID");
  await expect(section.getByTestId("tolerance")).toContainText("2 cm");

  // A new revision changes the arena and keeps the first one.
  await page.getByTestId("environment-new-revision").click();
  const revision = page.getByTestId("revision-form");
  await fill(revision, "revision-tank_diameter_cm", "150");
  await revision.getByTestId("revision-notes").fill("Wider pool");
  await revision.getByTestId("revision-save").click();
  await expect(page.getByTestId("environment-revision")).toHaveCount(2);
  const second = page.locator('[data-test="environment-revision"][data-revision="2"]');
  await expect(second).toContainText("150");
  // The new revision is a new physical setup, so it waits for its own calibration.
  await expect(second.getByTestId("environment-calibration")).toHaveAttribute("data-status", "WAITING_FOR_CALIBRATION");
  await expect(page.locator('[data-test="environment-revision"][data-revision="1"]').getByTestId("environment-calibration")).toHaveAttribute("data-status", "CALIBRATED");

  await page.goto("/environments");
  const row = page.getByTestId("environment").filter({ hasText: name });
  await expect(row).toHaveCount(1);
  await expect(row.getByTestId("environment-paradigm")).toHaveAttribute("data-key", "MWM");
});
