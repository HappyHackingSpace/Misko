// Uploading a recording from the browser: the panel checksums the file, sends
// it straight to storage with a resumable upload and then asks the API to
// verify what arrived. Nothing here goes through the API but the metadata.
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

test("uploading a video adds a recording the API has verified", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  // Wait for the list before counting: count() does not wait on its own.
  await expect(page.getByTestId("recordings")).toBeVisible();
  const rows = page.getByTestId("recording");
  const before = await rows.count();
  expect(before).toBeGreaterThan(0);

  // The same fixture the harness seeds: real bytes, so the checksum the browser
  // computes is the one the API checks against storage.
  await page.getByTestId("upload-input").setInputFiles("tests/fixtures/analyzed.mp4");

  // The upload finishes and the reloaded list shows the new recording. A wrong
  // checksum or a file that never reached storage fails the finalize call, and
  // the panel would show the error instead of a new row.
  await expect(rows).toHaveCount(before + 1, { timeout: 30_000 });
  await expect(page.getByTestId("error")).toHaveCount(0);

  // VERIFIED is the status the API records only after it has read the object
  // back from storage and matched its size, checksum and content type, so the
  // row proves the bytes arrived rather than that a request was sent. The status
  // is read from the attribute, not the label, which is translated.
  //
  // The row to look at is the last one: recordings come back ordered by
  // creation. Counting rows that carry the file name would not work, because
  // every project runs against the same seeded test, so the second and third
  // engine see the uploads the earlier ones made.
  const uploaded = rows.last();
  await expect(uploaded).toContainText("analyzed.mp4");
  await expect(uploaded).toHaveAttribute("data-video-status", "VERIFIED");
});
