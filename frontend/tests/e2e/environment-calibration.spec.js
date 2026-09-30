// A calibration belongs to the environment: it is entered once when the rig is
// set up and every video of it uses it. A single video whose camera or arena
// moved is calibrated by hand and only that video changes. Entering the same
// four corners for every video was the problem this replaces.
import { expect, test } from "@playwright/test";
import { openCalibration } from "./helpers.js";

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

// The fixture camera: a 320x240 view of a 50 cm square arena. The same
// correspondences the harness calibrates with.
const FIT = [
  { pixel: [52, 28], world: [0, 0] },
  { pixel: [268, 36], world: [50, 0] },
  { pixel: [292, 222], world: [50, 50] },
  { pixel: [30, 214], world: [0, 50] },
];
const CHECK = [
  { pixel: [160.9466, 116.0652], world: [25, 25] },
  { pixel: [85.7359, 172.3152], world: [10, 40] },
  { pixel: [227.6114, 66.2066], world: [40, 10] },
];

async function fillCalibration(form) {
  await form.getByTestId("camera-id").fill("e2e-camera");
  await form.getByTestId("frame-width").fill("320");
  await form.getByTestId("frame-height").fill("240");
  await form.getByTestId("crop-width").fill("320");
  await form.getByTestId("crop-height").fill("240");
  for (const [kind, points] of [["fit", FIT], ["check", CHECK]]) {
    for (const [index, p] of points.entries()) {
      await form.getByTestId(`${kind}-${index}-pixel-x`).fill(String(p.pixel[0]));
      await form.getByTestId(`${kind}-${index}-pixel-y`).fill(String(p.pixel[1]));
      await form.getByTestId(`${kind}-${index}-world-x`).fill(String(p.world[0]));
      await form.getByTestId(`${kind}-${index}-world-y`).fill(String(p.world[1]));
    }
  }
}

test("an environment is calibrated once when it is set up", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  await page.goto("/environments/new");
  await page.getByTestId("environment-name").fill(`E2E Rig ${Date.now().toString(36).toUpperCase()}`);
  await chooseOption(page, "environment-paradigm", "environment-paradigm-OPEN_FIELD");
  // The calibration points below lie inside a 50 cm square arena.
  await expect(page.getByTestId("apparatus-arena_width_cm")).toBeVisible({ timeout: 10_000 });
  await page.getByTestId("apparatus-arena_width_cm").fill("50");
  await page.getByTestId("apparatus-arena_height_cm").fill("50");
  await page.getByTestId("environment-save").click();
  await expect(page).toHaveURL(/\/environments\/[0-9a-f-]{36}$/);

  const section = page.getByTestId("environment-calibration");
  await expect(section).toHaveAttribute("data-status", "WAITING_FOR_CALIBRATION");
  await expect(section.getByTestId("calibration-waiting")).toBeVisible();

  await section.getByTestId("environment-calibrate").click();
  const form = section.getByTestId("calibration-form");
  await expect(form).toBeVisible();
  // An environment has no video, so there is no reference frame to name.
  await expect(form.getByTestId("reference-frame")).toHaveCount(0);
  await fillCalibration(form);
  await form.getByTestId("calibration-save").click();

  await expect(page.getByTestId("error")).toHaveCount(0);
  await expect(section).toHaveAttribute("data-status", "CALIBRATED", { timeout: 10_000 });
  await expect(section.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-status", "VALID");
  await expect(section.getByTestId("tolerance")).toContainText("2.00");

  // A correction replaces it and names it, so the chain stays one line.
  await section.getByTestId("environment-calibrate").click();
  await expect(section.getByTestId("calibration-correcting")).toBeVisible();
  await expect(section.getByTestId("camera-id")).toHaveValue("e2e-camera");
});

test("a recording without a calibration of its own uses the environment's", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  const card = await openCalibration(page, data.inheritedRecordingId);
  await expect(card.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-status", "VALID");
  await expect(card.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-source", "ENVIRONMENT");
  await expect(card.getByTestId("calibration-inherited")).toBeVisible();
  // Nothing checks camera movement yet, and the panel says so instead of
  // implying the calibration was verified for this video.
  await expect(card.getByTestId("calibration-drift")).toHaveAttribute("data-drift", "UNCHECKED");
  await expect(card.getByTestId("calibration-drift-hint")).toBeVisible();

  // The recording that was calibrated by hand keeps its own.
  const own = await openCalibration(page, data.recordingId);
  await expect(own.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-source", "RECORDING");
});

test("a video whose camera moved is calibrated by hand without touching the others", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  // Every upload of a test in the calibrated environment is calibrated from the
  // start. The new recording is the last one, since recordings come back in
  // creation order.
  await expect(page.getByTestId("recordings")).toBeVisible();
  const before = await page.getByTestId("recording").count();
  await page.getByTestId("upload-open").click();
  await page.getByTestId("upload-input").setInputFiles("tests/fixtures/analyzed.mp4");
  await expect(page.getByTestId("recording")).toHaveCount(before + 1, { timeout: 30_000 });

  // The page opens the recording that was just uploaded.
  const uploadedId = await page.getByTestId("recording").last().getAttribute("data-recording");
  const card = await openCalibration(page, uploadedId);
  await expect(card.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-source", "ENVIRONMENT");
  await card.getByTestId("calibrate").click();
  // The first manual calibration of a video replaces nothing, so it is not a
  // correction, and the form says what it is about to do.
  await expect(card.getByTestId("calibration-correcting")).toHaveCount(0);
  await expect(card.getByTestId("calibration-note")).toBeVisible();
  await fillCalibration(card.getByTestId("calibration-form"));
  await card.getByTestId("calibration-save").click();

  await expect(page.getByTestId("error")).toHaveCount(0);
  await expect(card.getByTestId("calibration-form")).toHaveCount(0, { timeout: 10_000 });
  await expect(card.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-source", "RECORDING");
  // Only that video changed.
  const inherited = await openCalibration(page, data.inheritedRecordingId);
  await expect(inherited.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-source", "ENVIRONMENT");
});

test("a viewer reads the environment's calibration but cannot enter one", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await page.goto(`/environments/${data.environmentId}`);

  const section = page.getByTestId("environment-calibration");
  await expect(section.getByTestId("calibration-summary")).toBeVisible();
  await expect(section.getByTestId("environment-calibrate")).toHaveCount(0);
});
