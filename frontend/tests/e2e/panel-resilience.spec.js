// What the panel does when reality is not tidy: two published runs to switch
// between, events clicked faster than the player can settle, a slow object
// store, and a signed URL that expired while the laboratory was reading.
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

async function openSeededTest(page, data) {
  await page.goto(`/tests/${data.testId}`);
  await expect(page.getByTestId("runs")).toBeVisible();
  await expect(page.getByTestId("event-timeline")).toBeVisible();
  await expect(page.getByTestId("analyzed-video")).toBeVisible();
}

// The object an analyzed video is playing, read from its own source URL.
const playingObject = (video) => new URL(video.src).searchParams.get("object") || "";

test("switching runs replaces the video and the events of the previous run", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await openSeededTest(page, data);

  const analyzed = page.getByTestId("analyzed-video");
  // The panel opens the newest published run, which is the reanalysis, and the
  // reanalysis wrote its own analyzed video.
  await expect(page.locator('[data-test="run"][data-selected="true"]')).toHaveAttribute("data-run-id", data.secondRunId);
  await expect.poll(async () => analyzed.evaluate(playingObject)).toContain("reanalysis.mp4");

  // Open the older run. Its pair is a different object, and nothing of the run
  // that was on screen may survive the switch.
  await page.locator(`[data-run-id="${data.runId}"]`).click();
  await expect.poll(async () => analyzed.evaluate(playingObject), { timeout: 10_000 }).toContain("overlay.mp4");
  await expect(analyzed).toBeVisible();
  await expect(page.getByTestId("event")).toHaveCount(3);
  await expect(page.getByTestId("metric-distance_cm")).toBeVisible();

  // And back again: the older run stays inspectable, so both pairs are readable.
  await page.locator(`[data-run-id="${data.secondRunId}"]`).click();
  await expect.poll(async () => analyzed.evaluate(playingObject), { timeout: 10_000 }).toContain("reanalysis.mp4");

  // Play an event here, which arms a stop at that event's end. The stop is
  // computed in the open run's own video time, so its offset is the one that
  // matters.
  const offset = (
    await (
      await request.get(`${API}/api/analysis-runs/${data.secondRunId}/video-pair`, {
        headers: { Authorization: `Bearer ${await page.evaluate(() => localStorage.getItem("misko_token"))}` },
      })
    ).json()
  ).outputOffsetUs / 1_000_000;
  const interval = (
    await page.getByTestId("event").evaluateAll((nodes) =>
      nodes
        .map((node) => ({ startUs: Number(node.dataset.startUs), endUs: Number(node.dataset.endUs) }))
        .filter((event) => event.endUs > event.startUs)
        .sort((a, b) => a.startUs - b.startUs),
    )
  )[0];
  await page.locator(`[data-start-us="${interval.startUs}"][data-end-us="${interval.endUs}"]`).click();
  // Switch while it is still playing, so the stop it armed is still pending:
  // once playback reaches the event end the stop has already spent itself.
  await expect
    .poll(async () => analyzed.evaluate((video) => !video.paused && video.currentTime > 0), { timeout: 10_000 })
    .toBe(true);

  // Switch runs and play the other video from its start. The stop belonged to
  // the run that was open, so nothing may cut this one short.
  await page.locator(`[data-run-id="${data.runId}"]`).click();
  await expect.poll(async () => analyzed.evaluate(playingObject), { timeout: 10_000 }).toContain("overlay.mp4");
  // Playing a source that has only just been swapped in is a race: seeking
  // before the engine holds any data, or calling play() before that seek
  // settles, leaves WebKit stalled on the first frame. Wait for data, seek,
  // wait for the seek, then play. A refused play is reported rather than
  // swallowed, so a stall says why it happened.
  await expect
    .poll(async () => analyzed.evaluate((video) => video.readyState), { timeout: 15_000 })
    .toBeGreaterThanOrEqual(2);
  await analyzed.evaluate(
    (video) =>
      new Promise((resolve) => {
        if (video.currentTime === 0) {
          resolve();
          return;
        }
        video.addEventListener("seeked", resolve, { once: true });
        video.currentTime = 0;
      }),
  );
  await analyzed.evaluate((video) => video.play());

  // The stale stop sits at the end of the interval that was played, in this
  // video's own time. Playing past it is what proves it was cleared: a panel
  // that kept it pauses exactly there. Waiting for the video to end instead
  // would depend on a shared machine playing four seconds in real time, which
  // is not something a test should rely on.
  const staleStop = offset + interval.endUs / 1_000_000;
  await expect
    .poll(async () => analyzed.evaluate((video) => video.currentTime), { timeout: 20_000 })
    .toBeGreaterThan(staleStop + 0.15);
  expect(await analyzed.evaluate((video) => video.paused)).toBe(false);
});

test("the last of several quick clicks is the interval that plays", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await openSeededTest(page, data);

  const analyzed = page.getByTestId("analyzed-video");
  const events = page.getByTestId("event");
  const intervals = await events.evaluateAll((nodes) =>
    nodes
      .map((node) => ({ startUs: Number(node.dataset.startUs), endUs: Number(node.dataset.endUs) }))
      .filter((event) => event.endUs > event.startUs)
      .sort((a, b) => a.startUs - b.startUs),
  );
  expect(intervals.length).toBeGreaterThan(1);
  // The earliest interval is clicked last, so the two stop times are far apart:
  // stopping at the later interval's end would mean the first click still ruled.
  const last = intervals[0];
  const first = intervals[intervals.length - 1];
  expect(first.endUs - last.endUs).toBeGreaterThan(300_000);

  const offset = (await (await request.get(`${API}/api/analysis-runs/${data.secondRunId}/video-pair`, {
    headers: { Authorization: `Bearer ${await page.evaluate(() => localStorage.getItem("misko_token"))}` },
  })).json()).outputOffsetUs / 1_000_000;

  // A point event can share its start with an interval, so both ends address it.
  const button = (event) => page.locator(`[data-start-us="${event.startUs}"][data-end-us="${event.endUs}"]`);
  await button(first).click();
  await button(last).click();

  // The clicked event is the one highlighted, and playback stops at its end.
  await expect(button(last)).toHaveAttribute("aria-pressed", "true");
  await expect
    .poll(async () => analyzed.evaluate((video) => video.paused && video.played.length > 0), { timeout: 10_000 })
    .toBe(true);
  const stopped = await analyzed.evaluate((video) => video.currentTime);
  expect(stopped).toBeLessThanOrEqual(offset + last.endUs / 1_000_000 + 0.3);
});

test("a slow object store delays playback but does not misplace it", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);

  // Every read of the video is answered late, so the player has to buffer.
  await page.route(/\/e2e\/storage\/object/, async (route) => {
    await new Promise((resolve) => setTimeout(resolve, 350));
    await route.continue();
  });

  await openSeededTest(page, data);
  const analyzed = page.getByTestId("analyzed-video");
  const events = page.getByTestId("event");
  const chosen = (
    await events.evaluateAll((nodes) =>
      nodes
        .map((node) => ({ startUs: Number(node.dataset.startUs), endUs: Number(node.dataset.endUs) }))
        .filter((event) => event.endUs > event.startUs)
        .sort((a, b) => b.endUs - b.startUs - (a.endUs - a.startUs)),
    )
  )[0];

  const offset = (await (await request.get(`${API}/api/analysis-runs/${data.secondRunId}/video-pair`, {
    headers: { Authorization: `Bearer ${await page.evaluate(() => localStorage.getItem("misko_token"))}` },
  })).json()).outputOffsetUs / 1_000_000;
  const startS = offset + chosen.startUs / 1_000_000;
  const endS = offset + chosen.endUs / 1_000_000;

  await page.locator(`[data-start-us="${chosen.startUs}"][data-end-us="${chosen.endUs}"]`).click();

  // A player that is still fetching can stutter at zero before the seek takes,
  // so what matters here is where it ends up: inside the interval that was
  // asked for, and stopped at its end rather than running on to the clip end.
  await expect
    .poll(async () => analyzed.evaluate((video) => video.currentTime), { timeout: 30_000 })
    .toBeGreaterThanOrEqual(startS - 0.3);
  await expect
    .poll(async () => analyzed.evaluate((video) => video.paused && video.played.length > 0), { timeout: 30_000 })
    .toBe(true);
  const stopped = await analyzed.evaluate((video) => video.currentTime);
  expect(stopped).toBeGreaterThanOrEqual(startS - 0.3);
  expect(stopped).toBeLessThanOrEqual(endS + 0.4);
});

test("an expired signed URL is replaced without losing the position", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await openSeededTest(page, data);

  const analyzed = page.getByTestId("analyzed-video");
  await expect.poll(async () => analyzed.evaluate((video) => video.duration || 0), { timeout: 10_000 }).toBeGreaterThan(0);

  // Park the player somewhere the laboratory would notice losing, and wait for
  // the seek to finish: until it does, the panel has not been told where the
  // reader is, and losing the URL in that millisecond is not what is tested here.
  await analyzed.evaluate(
    (video) =>
      new Promise((resolve) => {
        video.pause();
        video.addEventListener("seeked", resolve, { once: true });
        video.currentTime = 2.4;
      }),
  );
  await expect.poll(async () => analyzed.evaluate((v) => v.currentTime)).toBeGreaterThan(2);

  // The signed read expires. The store refuses it, exactly as a bucket does.
  const expired = await analyzed.evaluate((video) => {
    const url = new URL(video.src);
    url.searchParams.set("expires", String(Date.now() - 60_000));
    video.src = url.toString();
    video.load();
    return url.searchParams.get("expires");
  });

  // The panel asks for a fresh pair, and the new URL is valid again.
  await expect
    .poll(
      async () =>
        analyzed.evaluate((video) => Number(new URL(video.src).searchParams.get("expires") || 0)),
      { timeout: 15_000 },
    )
    .toBeGreaterThan(Number(expired));
  await expect.poll(async () => analyzed.evaluate((video) => video.error === null), { timeout: 15_000 }).toBe(true);

  // The reader is left where they were, not sent back to the start.
  await expect.poll(async () => analyzed.evaluate((v) => v.currentTime), { timeout: 15_000 }).toBeGreaterThan(2);
});
