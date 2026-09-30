// A calibration turns pixels into centimetres, so it is what makes a recording
// measurable at all. The panel showed only a badge saying whether one existed,
// and offered no way to enter one.
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

// The seeded test holds two recordings, one of which failed quality control, and
// a calibration belongs to one recording rather than to the test. Every
// assertion is therefore scoped to the recording the seed names.

// The arena is 50 cm square and the frame is 320x240. These four corners and
// three inner points are the correspondences the harness itself calibrates
// with, so they describe the real camera of the fixture.
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

async function fillPoints(card, kind, points) {
  for (const [index, p] of points.entries()) {
    await card.getByTestId(`${kind}-${index}-pixel-x`).fill(String(p.pixel[0]));
    await card.getByTestId(`${kind}-${index}-pixel-y`).fill(String(p.pixel[1]));
    await card.getByTestId(`${kind}-${index}-world-x`).fill(String(p.world[0]));
    await card.getByTestId(`${kind}-${index}-world-y`).fill(String(p.world[1]));
  }
}

async function openForm(card) {
  await card.getByTestId("calibrate").click();
  await expect(card.getByTestId("calibration-form")).toBeVisible();
  await card.getByTestId("camera-id").fill("e2e-camera");
  await card.getByTestId("frame-width").fill("320");
  await card.getByTestId("frame-height").fill("240");
  await card.getByTestId("crop-width").fill("320");
  await card.getByTestId("crop-height").fill("240");
}

test("a recording is calibrated from the panel and the errors are shown", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  const card = await openCalibration(page, data.recordingId);
  // The seeded recording is already calibrated, so this enters a correction,
  // which is the harder path: it has to name the calibration it replaces.
  await expect(card.getByTestId("fit-error")).toBeVisible();

  await openForm(card);
  await expect(card.getByTestId("calibration-correcting")).toBeVisible();
  await fillPoints(card, "fit", FIT);
  await fillPoints(card, "check", CHECK);
  await card.getByTestId("calibration-save").click();

  await expect(page.getByTestId("error")).toHaveCount(0);
  await expect(card.getByTestId("calibration-form")).toHaveCount(0, { timeout: 10_000 });
  // The points describe the fixture's camera exactly, so the calibration is
  // valid and its check error is well inside the two centimetre tolerance.
  await expect(card.getByTestId("calibration-summary")).toHaveAttribute("data-calibration-status", "VALID");
  await expect(card.getByTestId("tolerance")).toContainText("2.00");
});

test("a calibration whose points do not describe the camera is refused", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  const card = await openCalibration(page, data.recordingId);
  await openForm(card);

  // Four points on one line cannot define a plane, and the API says so rather
  // than computing a meaningless transform.
  await fillPoints(card, "fit", [
    { pixel: [10, 10], world: [0, 0] },
    { pixel: [20, 20], world: [10, 10] },
    { pixel: [30, 30], world: [20, 20] },
    { pixel: [40, 40], world: [30, 30] },
  ]);
  await fillPoints(card, "check", CHECK);
  await card.getByTestId("calibration-save").click();

  // The refusal is shown and the form stays open with the points still in it,
  // so the technician can correct them rather than type them again. The wording
  // is not asserted here: the panel is translated, and which language a session
  // is in is the reader's choice. That the code has a message in both languages
  // is a property of the message catalogue, not of this screen.
  await expect(page.getByTestId("error")).toBeVisible({ timeout: 10_000 });
  await expect(card.getByTestId("calibration-form")).toBeVisible();
  await expect(card.getByTestId("fit-0-pixel-x")).toHaveValue("10");
});

test("a recording whose video is not verified is not offered for calibration", async ({ page, request }) => {
  const data = await seed(request);

  // Declaring an upload creates the recording; the video stays PENDING until
  // the API has read the bytes back from storage. Calibrating one would pin
  // centimetres to a frame that may never arrive, so the screen must not offer
  // it. The upload is deliberately left unfinished.
  const login = await request.post(`${API}/api/auth/login`, {
    data: { email: data.adminEmail, password: data.adminPassword },
  });
  const { token } = await login.json();
  const started = await request.post(`${API}/api/tests/${data.testId}/recordings`, {
    headers: { Authorization: `Bearer ${token}` },
    // The checksum only has to be well formed here: four bytes, base64.
    data: { fileName: "never-arrives.mp4", contentType: "video/mp4", sizeBytes: 1024, crc32c: "AAAAAA==", clipStartUs: 0 },
  });
  expect(started.ok()).toBeTruthy();
  const pending = (await started.json()).recording.id;

  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  const card = await openCalibration(page, pending);
  await expect(card.getByTestId("calibration-none")).toBeVisible();
  await expect(card.getByTestId("calibrate")).toHaveCount(0);
  // The verified recording of the same test is still offered, so this is the
  // video's state talking and not a permission.
  const verified = await openCalibration(page, data.recordingId);
  await expect(verified.getByTestId("calibrate")).toHaveCount(1);
});

test("a viewer reads the calibration but cannot enter one", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await page.goto(`/tests/${data.testId}`);

  const card = await openCalibration(page, data.recordingId);
  // The measured errors stay readable.
  await expect(card.getByTestId("calibration-summary")).toBeVisible();
  // Entering one needs test:run, which a viewer does not have.
  await expect(card.getByTestId("calibrate")).toHaveCount(0);
  await expect(card.getByTestId("calibration-form")).toHaveCount(0);
});
