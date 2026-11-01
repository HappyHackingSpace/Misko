// Disease models have their own page: they are created, edited and deleted
// from a drawer; the API refuses a second model with the same name and the
// deletion of a model an experiment group uses, and the panel says why.
import { expect, test } from "@playwright/test";
import { chooseText, confirm, fill, signIn, unique } from "./helpers.js";

test("disease models: create, edit, duplicate name, in use, delete", async ({ page, request }) => {
  await signIn(page, request);
  await page.goto("/disease-models");
  const name = unique("Model");

  await page.getByTestId("disease-model-new").click();
  await fill(page, "disease-model-form-name", name);
  await chooseText(page, "disease-model-form-type", "Surgical");
  await fill(page, "disease-model-form-criterion", "Lesion seen on histology");
  await page.getByTestId("disease-model-form-save").click();
  const row = page.getByTestId("disease-model-row").filter({ hasText: name });
  await expect(row).toHaveCount(1);
  await expect(row).toContainText("Surgical");

  await page.getByTestId(`disease-model-edit-${name}`).click();
  await chooseText(page, "disease-model-form-type", "Diet");
  await page.getByTestId("disease-model-form-save").click();
  await expect(row).toContainText("Diet");

  await page.getByTestId("disease-model-new").click();
  await fill(page, "disease-model-form-name", name);
  await page.getByTestId("disease-model-form-save").click();
  await expect(page.getByTestId("disease-model-form-error")).toContainText("already exists");
  await page.keyboard.press("Escape");

  // Groups of the seeded study use the MPTP model.
  await page.getByTestId("disease-model-delete-MPTP Parkinson").click();
  await confirm(page);
  await expect(page.getByTestId("disease-model-list-error")).toContainText("cannot be deleted");

  await page.getByTestId(`disease-model-delete-${name}`).click();
  await confirm(page);
  await expect(row).toHaveCount(0);
});
