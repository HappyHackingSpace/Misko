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

// Detection proposes the fit points on the frame shown; each stays proposed
// until the researcher confirms it, and check points are never proposed. The
// worker's answer is stubbed: it reports pixels of a frame half the video's size.
test("detecting the apparatus proposes fit points to confirm", async ({ page, request }) => {
  await signIn(page, request);
  await page.goto("/environments/new");
  await fill(page, "environment-name", unique("Box"));
  await page.getByTestId("environment-paradigm").click();
  await page.getByTestId("environment-paradigm-NOVEL_OBJECT").click();
  await expect(page.getByTestId("apparatus-arena_width_cm")).toBeVisible({ timeout: 10_000 });
  await fill(page, "apparatus-arena_width_cm", "40");
  await fill(page, "apparatus-arena_height_cm", "40");
  await page.getByTestId("environment-save").click();
  await expect(page).toHaveURL(/\/environments\/[0-9a-f-]{36}$/);

  let sent = null;
  await page.route("**/api/calibration-proposals", async (route) => {
    sent = route.request().postDataJSON();
    await route.fulfill({ status: 202, json: { id: "p-1", status: "QUEUED", result: null } });
  });
  const corners = [[20, 10], [140, 12], [138, 110], [22, 108]];
  const world = [[0, 0], [40, 0], [40, 40], [0, 40]];
  await page.route("**/api/calibration-proposals/p-1", (route) =>
    route.fulfill({
      json: {
        id: "p-1", status: "SUCCEEDED",
        result: {
          detected: true, frameWidth: 160, frameHeight: 120, confidence: 0.93,
          proposal: {
            measurementPlane: "ARENA_FLOOR",
            fitPoints: corners.map(([x, y], i) => ({ pixelX: x, pixelY: y, worldX: world[i][0], worldY: world[i][1] })),
            derivedCheckPoints: [{ pixelX: 80, pixelY: 11, worldX: 20, worldY: 0 }],
          },
        },
      },
    }),
  );

  const section = page.getByTestId("environment-calibration");
  await section.getByTestId("environment-calibrate").click();
  const form = section.getByTestId("calibration-form");
  await form.getByTestId("calibration-video-file").setInputFiles("tests/fixtures/analyzed.mp4");
  await form.getByTestId("calibration-detect").click();
  await expect(form.getByTestId("calibration-detect-message")).toContainText("93");
  expect(sent.paradigmKey).toBe("NOVEL_OBJECT");
  expect(sent.parameters.arena_width_cm).toBe(40);
  expect(sent.frame.length).toBeGreaterThan(100);

  // Scaled back to the 320 x 240 video.
  await expect(form.getByTestId("fit-1-pixel-x").locator("input")).toHaveValue("280");
  await expect(form.getByTestId("fit-2-world-y").locator("input")).toHaveValue("40");
  await expect(form.getByTestId("point-proposed")).toHaveCount(4);
  await expect(form.getByTestId("calibration-save")).toBeDisabled();
  for (const index of [0, 1, 2, 3]) await form.getByTestId(`fit-${index}-confirm`).click();
  await expect(form.getByTestId("point-proposed")).toHaveCount(0);
  await form.getByTestId("mode-check").click();
  await expect(form.getByTestId("check-0-pixel-x").locator("input")).toHaveValue("");
});
