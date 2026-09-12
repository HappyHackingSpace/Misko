// The workflow the issue asks for: open an analyzed test, click an event in the
// timeline and watch that interval play in the side panel, for the run that
// published it. The data comes from backend/tests/e2e, which seeds one analyzed
// Open Field test and serves signed URLs from local storage.
import { expect, test } from "@playwright/test";

const API = `http://127.0.0.1:${process.env.E2E_API_PORT || "4010"}`;

async function seed(request) {
  const response = await request.get(`${API}/e2e/seed`);
  expect(response.ok()).toBeTruthy();
  return response.json();
}

async function signIn(page, credentials) {
  // The panel starts in Turkish, so fields are addressed by their input id and
  // the submit button by its test hook rather than by translated text.
  await page.goto("/login");
  await page.locator("#login-email").fill(credentials.email);
  await page.locator("#login-password").fill(credentials.password);
  await page.getByTestId("sign-in").click();
  await expect(page.getByTestId("nav-experiments")).toBeVisible();
}

// The seeded run's events, in the order the timeline shows them.
async function openSeededTest(page, data) {
  await page.goto(`/tests/${data.testId}`);
  await expect(page.getByTestId("runs")).toBeVisible();
  await expect(page.getByTestId("event-timeline")).toBeVisible();
}

// The run the panel actually opened. The test has more than one published run,
// so reading the pair of a run the panel is not showing would prove nothing.
async function openRunId(page) {
  const selected = page.locator('[data-test="run"][data-selected="true"]');
  await expect(selected).toHaveCount(1);
  return selected.getAttribute("data-run-id");
}

// The pair of a run, read through the API with the panel's own token.
async function videoPair(page, request, runId) {
  const token = await page.evaluate(() => localStorage.getItem("misko_token"));
  const response = await request.get(`${API}/api/analysis-runs/${runId}/video-pair`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  expect(response.ok()).toBeTruthy();
  return response.json();
}

test.describe("analysis panel", () => {
  test("clicking an event plays that interval of the analyzed video", async ({ page, request }) => {
    const data = await seed(request);
    await signIn(page, { email: data.adminEmail, password: data.adminPassword });
    await openSeededTest(page, data);

    // Event times are measured from the clip start, the videos from their own
    // start: the seeded clip begins a second into the recording, so every
    // expected position below carries the pair's output offset.
    const pair = await videoPair(page, request, await openRunId(page));
    expect(pair.outputOffsetUs).toBeGreaterThan(0);
    const offset = pair.outputOffsetUs / 1_000_000;

    // Metrics come from the Go engine, so the panel shows what was published.
    await expect(page.getByTestId("metric-distance_cm")).toBeVisible();

    const events = page.getByTestId("event");
    await expect(events).toHaveCount(3);

    // Pick the interval that starts furthest from both ends of the clip, so
    // "seeked to the start" cannot be confused with "sat where it already was"
    // and "stopped at the end" cannot be confused with "ran to the end of the video".
    const analyzed = page.getByTestId("analyzed-video");
    await expect(analyzed).toBeVisible();
    const duration = await expect
      .poll(async () => analyzed.evaluate((video) => video.duration || 0), { timeout: 5000 })
      .toBeGreaterThan(0)
      .then(() => analyzed.evaluate((video) => video.duration));

    const intervals = await events.evaluateAll((nodes) =>
      nodes
        .map((node) => ({
          startUs: Number(node.dataset.startUs),
          endUs: Number(node.dataset.endUs),
          type: node.dataset.eventType,
        }))
        .filter((event) => event.endUs > event.startUs),
    );
    expect(intervals.length).toBeGreaterThan(0);
    const chosen = intervals.sort((a, b) => b.endUs - b.startUs - (a.endUs - a.startUs))[0];
    // Where the event lands in the analyzed video, which is what the player shows.
    const startS = offset + chosen.startUs / 1_000_000;
    const endS = offset + chosen.endUs / 1_000_000;
    expect(startS).toBeGreaterThan(0.5);
    expect(endS).toBeLessThan(duration - 0.2);

    // Park the video at zero first: any later position must come from the click.
    await analyzed.evaluate((video) => {
      video.pause();
      video.currentTime = 0;
    });
    await expect.poll(async () => analyzed.evaluate((v) => v.currentTime), { timeout: 5000 }).toBeLessThan(0.2);

    const target = events.filter({ has: page.locator(`[data-start-us="${chosen.startUs}"]`) }).first();
    const clickable = (await target.count()) ? target : events.nth(intervals.indexOf(chosen));
    await clickable.click();
    await expect(clickable).toHaveAttribute("aria-pressed", "true");

    // It plays and stops on its own, before the video runs out.
    await expect
      .poll(async () => analyzed.evaluate((video) => video.paused && video.played.length > 0), { timeout: 10_000 })
      .toBe(true);

    // What was played says where it began: seeking does not add to `played`, so
    // the first played second is the event start unless the panel played its way
    // there from somewhere earlier.
    const playback = await analyzed.evaluate((video) => ({
      current: video.currentTime,
      from: video.played.start(0),
    }));
    expect(playback.from).toBeGreaterThanOrEqual(startS - 0.25);
    expect(playback.from).toBeLessThanOrEqual(startS + 0.5);
    expect(playback.current).toBeGreaterThanOrEqual(startS - 0.25);
    // The stop is sampled on timeupdate, and engines sample at different rates:
    // Firefox overshoots the event end by more than Chromium does. The margin
    // has to cover that and still stay clear of the clip end, which is what
    // separates "stopped at the event" from "ran to the end of the video".
    expect(playback.current).toBeLessThanOrEqual(endS + 0.4);
    expect(playback.current).toBeLessThan(duration - 0.05);
  });

  test("the original and the analyzed video share the recording timeline", async ({ page, request }) => {
    const data = await seed(request);
    await signIn(page, { email: data.adminEmail, password: data.adminPassword });
    await openSeededTest(page, data);

    const body = await videoPair(page, request, await openRunId(page));
    expect(body.timeMappingVersion).toBe("identity-v1");
    // The analyzed clip starts inside the recording, so both offsets are the
    // clip start and neither player may ignore them.
    expect(body.sourceOffsetUs).toBeGreaterThan(0);
    expect(body.outputOffsetUs).toBeGreaterThan(0);

    const first = page.getByTestId("event").first();
    const startUs = Number(await first.getAttribute("data-start-us"));
    await first.click();

    // Both players land on the same recording time, each in its own timeline.
    await expect
      .poll(async () => page.getByTestId("analyzed-video").evaluate((v) => v.currentTime), { timeout: 5000 })
      .toBeGreaterThanOrEqual(startUs / 1_000_000 + body.outputOffsetUs / 1_000_000 - 0.2);
    const original = await page.getByTestId("original-video").evaluate((v) => v.currentTime);
    expect(original).toBeGreaterThanOrEqual(startUs / 1_000_000 + body.sourceOffsetUs / 1_000_000 - 0.2);
  });

  test("every event is reachable and startable with the keyboard", async ({ page, request }) => {
    const data = await seed(request);
    await signIn(page, { email: data.adminEmail, password: data.adminPassword });
    await openSeededTest(page, data);

    const first = page.getByTestId("event").first();
    await first.focus();
    await expect(first).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(first).toHaveAttribute("aria-pressed", "true");
  });
});
