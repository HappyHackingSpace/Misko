// Shared by the specs that work inside the test page. The page lists the
// recordings of a test and shows one at a time, so anything a spec wants to read
// first has to be brought on screen: the recording, then the tab, then the run.
import { expect } from "@playwright/test";

export const API = `http://127.0.0.1:${process.env.E2E_API_PORT || "4010"}`;

const auth = async (page) => ({ Authorization: `Bearer ${await page.evaluate(() => localStorage.getItem("misko_token"))}` });

// Shows the recording in the pane and waits until the pane is about it.
export async function openRecording(page, recordingId) {
  await page.getByTestId("page-tab-videos").click();
  await page.locator(`[data-test="recording"][data-recording="${recordingId}"] button`).click();
  await expect(page.locator(`[data-test="recording-pane"][data-recording="${recordingId}"]`)).toBeVisible();
}

// The calibration card of a recording, brought on screen.
export async function openCalibration(page, recordingId) {
  await openRecording(page, recordingId);
  await page.getByTestId("tab-calibration").click();
  const card = page.locator(`[data-test="calibration"][data-recording="${recordingId}"]`);
  await expect(card).toBeVisible();
  return card;
}

// Picks a run of the recording in the pane from the run list.
export async function chooseRun(page, runId) {
  const result = page.getByTestId("result");
  if ((await result.getAttribute("data-run-id")) === runId) return;
  await page.getByTestId("run-select").click();
  await page.locator(`.n-base-select-option [data-test="run-select-option"][data-value="${runId}"]`).click();
  await expect(result).toHaveAttribute("data-run-id", runId);
}

// Opens the result of any run of the test: the recording it belongs to first,
// because the page shows the runs of one recording at a time.
export async function openRun(page, request, runId) {
  const response = await request.get(`${API}/api/analysis-runs/${runId}`, { headers: await auth(page) });
  expect(response.ok()).toBeTruthy();
  const { recordingId } = await response.json();
  await openRecording(page, recordingId);
  await page.getByTestId("tab-result").click();
  await chooseRun(page, runId);
}
