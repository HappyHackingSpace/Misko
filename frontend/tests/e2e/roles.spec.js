// What each role may do is decided by the API; the panel hides what it would
// refuse. A viewer reads everything and changes nothing. A technician runs
// the lab day: doses, weighs, runs sessions and uploads their videos, but does
// not plan sessions or change the study.
import { expect, test } from "@playwright/test";
import { seed, signIn } from "./helpers.js";

test("a viewer signs in and sees no write controls", async ({ page, request }) => {
  const data = await seed(request);
  await page.addInitScript(() => localStorage.setItem("misko.locale", "en"));
  await page.goto("/login");
  await page.locator("#login-email").fill(data.accounts.VIEWER.email);
  await page.locator("#login-password").fill(data.accounts.VIEWER.password);
  await page.getByTestId("sign-in").click();
  await expect(page.getByTestId("nav-experiments")).toBeVisible();

  const none = async (...ids) => {
    for (const id of ids) await expect(page.getByTestId(id), id).toHaveCount(0);
  };
  await page.goto("/substances");
  await expect(page.getByTestId("substance-row").first()).toBeVisible();
  await none("substance-new", "substance-edit-MPTP", "substance-delete-MPTP");
  await page.goto("/disease-models");
  await expect(page.getByTestId("disease-model-row").first()).toBeVisible();
  await none("disease-model-new", "disease-model-edit-MPTP Parkinson", "disease-model-delete-MPTP Parkinson");
  await page.goto("/subjects");
  await expect(page.getByTestId("subject-row").first()).toBeVisible();
  await none("subject-new", "subject-edit-M01");
  await page.goto(`/subjects/${data.subjects.M01}`);
  await expect(page.getByTestId("subject-weights")).toBeVisible();
  await none("subject-edit", "weight-form");
  await page.goto("/environments");
  await expect(page.getByTestId("environment").first()).toBeVisible();
  await none("environment-new");
  await page.goto(`/environments/${data.environments.poolA}`);
  await expect(page.getByTestId("calibration-summary")).toBeVisible();
  await none("environment-calibrate", "environment-new-revision");

  await page.goto("/experiments");
  await expect(page.getByTestId("experiment-link").first()).toBeVisible();
  await none("experiment-new");
  const study = `/experiments/${data.experimentId}`;
  await page.goto(`${study}?tab=overview`);
  await expect(page.getByTestId("overview-hypothesis")).toBeVisible();
  await none("experiment-edit", "experiment-complete");
  await page.goto(`${study}?tab=animals`);
  await expect(page.getByTestId("allocations-table")).toBeVisible();
  await none("allocate-open", "allocation-exclude");
  await page.goto(`${study}?tab=dosing`);
  await expect(page.getByTestId("due-dose").first()).toBeVisible();
  await none("record-dose", "unplanned-dose", "record-induction");
  await page.goto(`${study}?tab=tests`);
  await expect(page.getByTestId("session-link").first()).toBeVisible();
  await none("plan-session", "plan-at-timepoint", "edit-animals");

  await page.goto(`/tests/${data.runningTestId}`);
  await expect(page.getByTestId("test-animal")).toHaveCount(2);
  await expect(page.getByTestId("video-current")).toBeVisible();
  await none("complete-test", "cancel-open", "trial-open", "video-replace", "video-upload", "calibrate", "reanalyze");

  await page.goto(`/review?experimentId=${data.experimentId}&open=${data.results.M07}`);
  await expect(page.getByTestId("review-detail")).toBeVisible();
  await none("bulk-bar", "review-actions", "choose-run");

  // Pages that only write are out of reach.
  for (const path of ["/users", "/experiments/new", "/environments/new"]) {
    await page.goto(path);
    await expect(page).toHaveURL(/\/dashboard$/);
  }
});

test("a technician doses, runs a session and uploads its video, but cannot plan", async ({ page, request }) => {
  const { data } = await signIn(page, request, "TECHNICIAN");
  const study = `/experiments/${data.experimentId}`;

  await page.goto(`${study}?tab=dosing`);
  const m02 = page.locator('[data-test="due-dose"][data-animal="M02"]');
  await m02.getByTestId("record-dose").click();
  await page.getByTestId("dose-dialog").getByTestId("dose-save").click();
  await expect(m02).toHaveAttribute("data-state", "given");

  await page.goto(`${study}?tab=tests`);
  await expect(page.getByTestId("session-link").first()).toBeVisible();
  await expect(page.getByTestId("plan-session")).toHaveCount(0);
  await page.goto(`${study}?tab=overview`);
  await expect(page.getByTestId("experiment-edit")).toHaveCount(0);

  // The overdue baseline session of M01.
  await page.goto(`/tests/${data.plannedTestId}`);
  await expect(page.getByTestId("cancel-open")).toHaveCount(0);
  await expect(page.getByTestId("edit-animals")).toHaveCount(0);
  await page.getByTestId("start-test").click();
  await expect(page.getByTestId("test-status")).toHaveAttribute("data-status", "IN_PROGRESS");
  const m01 = page.locator('[data-test="test-animal"][data-animal="M01"]');
  await m01.getByTestId("trial-open").click();
  await m01.getByTestId("trial-save").click();
  await expect(m01.getByTestId("trial")).toHaveCount(1);
  await page.getByTestId("video-input").setInputFiles("tests/fixtures/analyzed.mp4");
  await expect(page.getByTestId("video-current")).toBeVisible({ timeout: 30_000 });
  await expect(page.getByTestId("complete-test")).toBeEnabled();
});
