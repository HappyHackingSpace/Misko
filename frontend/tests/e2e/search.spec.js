// One search from the header: Ctrl+K (Cmd+K on a Mac) or the button opens it,
// results update while typing, grouped by kind, and a result opens its page.
import { expect, test } from "@playwright/test";
import { signIn } from "./helpers.js";

test("one search finds studies, their reports, sessions and subjects", async ({ page, request }) => {
  const { data } = await signIn(page, request);
  const palette = page.getByTestId("search-palette");
  const input = page.getByTestId("search-input");
  const group = (key) => palette.getByTestId(`search-${key}`);

  await page.goto("/dashboard");
  await page.keyboard.press("ControlOrMeta+k");
  await expect(input).toBeFocused();
  await input.fill("Compound X");
  await expect(group("studies")).toContainText("E2E");
  await expect(group("reports").locator(`a[href="/experiments/${data.experimentId}?tab=results"]`)).toBeVisible();
  // The keyboard moves through the results; Enter opens the one marked.
  await input.press("ArrowDown");
  await input.press("ArrowUp");
  await input.press("Enter");
  await expect(page).toHaveURL(new RegExp(`/experiments/${data.experimentId}$`));
  await expect(palette).toBeHidden();

  // A subject code finds the animal and the sessions it is in.
  await page.getByTestId("search-open").click();
  await input.fill("M07");
  await expect(group("subjects")).toContainText("M07");
  await group("sessions").locator(`a[href="/tests/${data.day14TestId}"]`).click();
  await expect(page).toHaveURL(new RegExp(`/tests/${data.day14TestId}$`));

  await page.keyboard.press("ControlOrMeta+k");
  await input.fill("no-such-record");
  await expect(palette.getByTestId("search-empty")).toContainText("no-such-record");
  await input.press("Escape");
  await expect(palette).toBeHidden();

  // A search that could not run says so instead of "nothing matches".
  await page.route(/\/api\/(experiments|tests|subjects)\?/, (route) => route.abort());
  await page.getByTestId("search-open").click();
  await input.fill("M07");
  await expect(palette.getByTestId("search-failed")).toBeVisible();
  await palette.getByRole("button", { name: "Close search" }).click();
  await expect(palette).toBeHidden();
});
