// Substances have their own page: they are created, edited and deleted from a
// drawer; the API refuses a second entry with the same name and the deletion of
// a substance a study uses, and the panel says why.
import { expect, test } from "@playwright/test";
import { chooseText, confirm, fill, signIn, unique } from "./helpers.js";

test("substances: create, edit, duplicate name, in use, delete", async ({ page, request }) => {
  await signIn(page, request);
  await page.goto("/substances");
  const name = unique("Compound");

  await page.getByTestId("substance-new").click();
  await fill(page, "substance-form-name", name);
  await chooseText(page, "substance-form-kind", "Test compound");
  await fill(page, "substance-form-description", "Made for the browser test");
  await page.getByTestId("substance-form-save").click();
  const row = page.getByTestId("substance-row").filter({ hasText: name });
  await expect(row).toHaveCount(1);

  // The search narrows the list on the server.
  await fill(page, "substance-search", name);
  await expect(page.getByTestId("substance-row")).toHaveCount(1);

  await page.getByTestId(`substance-edit-${name}`).click();
  await fill(page, "substance-form-description", "Edited");
  await page.getByTestId("substance-form-save").click();
  await expect(row).toContainText("Edited");

  // A second entry with the same name, in any case, is refused.
  await page.getByTestId("substance-new").click();
  await fill(page, "substance-form-name", name.toLowerCase());
  await page.getByTestId("substance-form-save").click();
  await expect(page.getByTestId("substance-form-error")).toContainText("already exists");
  await page.keyboard.press("Escape");

  // The seeded study doses MPTP, so it cannot be deleted.
  await fill(page, "substance-search", "MPTP");
  await expect(page.getByTestId("substance-row")).toHaveCount(1);
  await page.getByTestId("substance-delete-MPTP").click();
  await confirm(page);
  await expect(page.getByTestId("substance-list-error")).toContainText("cannot be deleted");

  await fill(page, "substance-search", name);
  await page.getByTestId(`substance-delete-${name}`).click();
  await confirm(page);
  await expect(page.getByTestId("substance-row")).toHaveCount(0);
});
