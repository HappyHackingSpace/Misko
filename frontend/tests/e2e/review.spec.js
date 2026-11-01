// Reviewing analysis results: every result counts only after a person accepts
// it. A result is accepted or rejected with a reason, a newer run can be put
// under review instead, clean results are accepted together, and in a blinded
// study the reviewer never sees the group.
import { expect, test } from "@playwright/test";
import { chooseValue, confirm, fill, signIn } from "./helpers.js";

// Both tests change the seeded results (reject, accept, choose a run), so a
// retry starts from changed data and can only fail for the wrong reason.
test.describe.configure({ retries: 0 });

// The review page plays the analysed video pair; Firefox and WebKit may hold
// the load event while the videos buffer. Every assertion waits for what it
// needs, so navigation only waits for the document.
const open = (page, url) => page.goto(url, { waitUntil: "domcontentloaded" });

test("results are rejected with a reason, accepted and switched to another run", async ({ page, request }) => {
  const { data, api } = await signIn(page, request);
  const row = (id) => page.locator(`[data-test="review-row"][data-test-subject="${id}"]`);
  const detail = page.getByTestId("review-detail");

  await open(page, `/review?experimentId=${data.experimentId}`);
  await expect(row(data.results.M03)).toHaveAttribute("data-status", "PENDING");
  await expect(row(data.results.M07)).toBeVisible();

  // Reject: the group is visible in an open study; a reason is required.
  await row(data.results.M03).click();
  await expect(detail).toHaveAttribute("data-test-subject", data.results.M03);
  await expect(detail.getByTestId("review-group")).toContainText("G2");
  await expect(detail.getByTestId("review-metric-path_length_cm")).toContainText(/127[.,]4 cm/);
  await expect(detail.getByTestId("review-reject")).toBeDisabled();
  await chooseValue(page, "review-reason", "TRACKING_LOST", { scope: detail });
  await fill(detail, "review-note", "Tail tracked instead of the body");
  await detail.getByTestId("review-reject").click();
  await expect(row(data.results.M03)).toHaveCount(0);
  expect((await api("GET", `/analysis/results/${data.results.M03}`)).status).toBe("REJECTED");

  // Accept: M02 was analysed twice and says a newer run exists.
  await open(page, `/review?experimentId=${data.experimentId}&open=${data.results.M02}`);
  await expect(detail).toHaveAttribute("data-test-subject", data.results.M02);
  await expect(detail.getByTestId("check-newer")).toBeVisible();
  await detail.getByTestId("review-accept").click();
  await expect(row(data.results.M02)).toHaveCount(0);

  // Choosing the newer run sends the animal back to review with that run.
  const before = await api("GET", `/analysis/results/${data.results.M02}`);
  expect(before.status).toBe("ACCEPTED");
  await open(page, `/review?experimentId=${data.experimentId}&status=ALL&open=${data.results.M02}`);
  await expect(detail.getByTestId("decision")).toBeVisible();
  await chooseValue(page, "review-run-select", before.latestRunId, { scope: detail });
  await detail.getByTestId("choose-run").click();
  await expect(row(data.results.M02)).toHaveAttribute("data-status", "PENDING");
  const after = await api("GET", `/analysis/results/${data.results.M02}`);
  expect(after.status).toBe("PENDING");
  expect(after.run.id).toBe(before.latestRunId);
  expect(after.newerRunAvailable).toBe(false);

  // The status filter lives in the address.
  await open(page, `/review?experimentId=${data.experimentId}&status=REJECTED`);
  await expect(row(data.results.M03)).toHaveAttribute("data-status", "REJECTED");
  await expect(page.getByTestId("review-row")).toHaveCount(1);
});

test("a blinded study hides the group and clean results are accepted together", async ({ page, request }) => {
  const { data } = await signIn(page, request);
  const { blind } = data;
  const detail = page.getByTestId("review-detail");

  await open(page, `/review?experimentId=${blind.experimentId}`);
  const rows = page.getByTestId("review-row");
  await expect(rows).toHaveCount(2);
  for (const text of await rows.allInnerTexts()) expect(text).toContain("Hidden (blinded)");
  await page.locator(`[data-test="review-row"][data-test-subject="${blind.results.B01}"]`).click();
  await expect(detail.getByTestId("blinded")).toBeVisible();
  await expect(detail.getByTestId("review-group")).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(detail).toBeHidden();

  await expect(page.getByTestId("select-clean")).toContainText("(2)");
  await page.getByTestId("select-clean").click();
  await expect(page.getByTestId("bulk-accept")).toContainText("(2)");
  await page.getByTestId("bulk-accept").click();
  await confirm(page);
  await expect(page.getByTestId("review-empty")).toBeVisible();

  await open(page, `/review?experimentId=${blind.experimentId}&status=ACCEPTED`);
  await expect(rows).toHaveCount(2);
  // The study report of a blinded study warns before it is shared.
  await open(page, `/experiments/${blind.experimentId}?tab=results`);
  await expect(page.getByTestId("report-blinded")).toBeVisible();
  await expect(page.getByTestId("report-interim")).toBeVisible();
});
