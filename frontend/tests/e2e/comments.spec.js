// A test is discussed by the people who ran it: an unusual observation, a
// question for whoever analyses the data later. The thread existed as a
// component written against the replaced API and was mounted on no screen, so
// the laboratory had no way to leave a note on a test.
import { expect, test } from "@playwright/test";

const API = `http://127.0.0.1:${process.env.E2E_API_PORT || "4010"}`;

async function seed(request) {
  const response = await request.get(`${API}/e2e/seed`);
  expect(response.ok()).toBeTruthy();
  return response.json();
}

// Signing in again is not enough to change who is reading: the router sends a
// signed-in visitor away from the login screen, so the session is ended first.
async function signOut(page) {
  await page.getByTestId("user-menu").click();
  await page.getByTestId("logout").click();
  // Logging out routes to the login screen. Waiting for that to land matters:
  // a goto issued while it is still in flight interrupts it.
  await page.waitForURL(/\/login$/);
  await expect(page.locator("#login-email")).toBeVisible();
}

async function signIn(page, email, password) {
  if (!/\/login$/.test(page.url())) await page.goto("/login");
  await page.locator("#login-email").fill(email);
  await page.locator("#login-password").fill(password);
  await page.getByTestId("sign-in").click();
  await expect(page.getByTestId("nav-experiments")).toBeVisible();
}

// The seeded test is shared by every run, so each comment carries a mark of its
// own and every assertion looks for that one rather than counting the thread.
const unique = (what) => `${what} ${Date.now().toString(36).toUpperCase()}`;

const commentWith = (page, text) => page.getByTestId("comment").filter({ hasText: text });

// A comment being edited shows a textarea instead of its body, so its text no
// longer finds it. Rows are pinned by id once they exist.
const commentById = (page, id) => page.locator(`[data-test="comment"][data-comment="${id}"]`);

async function post(page, text) {
  // The thread is a tab of the test page.
  await page.getByTestId("page-tab-comments").click();
  await page.getByTestId("comment-draft").fill(text);
  await page.getByTestId("comment-submit").click();
  const row = commentWith(page, text);
  await expect(row).toHaveCount(1, { timeout: 10_000 });
  return row.getAttribute("data-comment");
}

test("a comment is posted, corrected and marked as edited", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  const body = unique("Mouse hesitated at the entry");
  const id = await post(page, body);
  const posted = commentById(page, id);
  // A fresh comment is not marked as edited; the API decides that, not the panel.
  await expect(posted.getByTestId("comment-edited")).toHaveCount(0);

  const corrected = `${body} (second trial)`;
  await posted.getByTestId("comment-edit").click();
  await posted.getByTestId("comment-edit-draft").fill(corrected);
  await posted.getByTestId("comment-edit-save").click();

  await expect(page.getByTestId("comment-error")).toHaveCount(0);
  // The same row now carries the corrected text and says it was edited.
  await expect(posted.getByTestId("comment-body")).toHaveText(corrected, { timeout: 10_000 });
  await expect(posted.getByTestId("comment-edited")).toBeVisible();
});

test("a viewer may write a comment but cannot touch someone else's", async ({ page, request }) => {
  const data = await seed(request);

  // The administrator leaves a note first, so the viewer has someone else's
  // comment in front of them.
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);
  const theirs = unique("Checked the arena light");
  await post(page, theirs);

  await signOut(page);
  await signIn(page, data.viewerEmail, data.viewerPassword);
  await page.goto(`/tests/${data.testId}`);

  // Commenting needs only read access, so a viewer takes part in the discussion.
  const mine = unique("Second observer agrees");
  await post(page, mine);
  await expect(commentWith(page, mine).getByTestId("comment-edit")).toHaveCount(1);

  // Editing belongs to the author, and a viewer moderates nothing.
  const other = commentWith(page, theirs);
  await expect(other).toHaveCount(1);
  await expect(other.getByTestId("comment-edit")).toHaveCount(0);
  await expect(other.getByTestId("comment-delete")).toHaveCount(0);
});

test("deleting asks first and then removes the comment", async ({ page, request }) => {
  const data = await seed(request);
  await signIn(page, data.adminEmail, data.adminPassword);
  await page.goto(`/tests/${data.testId}`);

  const body = unique("Wrong subject, ignore");
  const id = await post(page, body);
  const posted = commentById(page, id);

  // Deleting cannot be undone, so it is confirmed in the thread itself.
  await posted.getByTestId("comment-delete").click();
  await expect(posted.getByTestId("comment-confirm")).toBeVisible();
  await posted.getByTestId("comment-delete-cancel").click();
  await expect(posted).toHaveCount(1);

  await posted.getByTestId("comment-delete").click();
  await posted.getByTestId("comment-delete-confirm").click();
  await expect(commentById(page, id)).toHaveCount(0, { timeout: 10_000 });
  await expect(page.getByTestId("comment-error")).toHaveCount(0);
});
