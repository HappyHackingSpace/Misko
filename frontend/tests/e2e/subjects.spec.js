// Animals: created and edited in a drawer, found with the search and the
// filters, and on their own page their allocation and body weights.
import { expect, test } from "@playwright/test";
import { chooseText, fill, signIn, unique } from "./helpers.js";

test("subjects are created, edited and filtered", async ({ page, request }) => {
  await signIn(page, request);
  const prefix = unique("S");
  await page.goto("/subjects");

  const create = async (code, species, sex) => {
    await page.getByTestId("subject-new").click();
    await fill(page, "subject-code", code);
    await page.getByTestId(`subject-species-${species}`).click();
    await page.getByTestId(`subject-sex-${sex}`).click();
    await page.getByTestId("subject-save").click();
    await expect(page.getByTestId("subject-form")).toBeHidden();
  };
  await create(`${prefix}-R1`, "RAT", "FEMALE");
  await create(`${prefix}-M1`, "MOUSE", "MALE");

  await fill(page, "list-search", prefix);
  const rows = page.getByTestId("subject-row");
  await expect(rows).toHaveCount(2);

  // A code already in use is refused.
  await page.getByTestId("subject-new").click();
  await fill(page, "subject-code", `${prefix}-r1`);
  await page.getByTestId("subject-save").click();
  await expect(page.getByTestId("subject-form-error")).toBeVisible();
  await page.keyboard.press("Escape");

  // Edit: the rat was a male after all.
  await page.getByTestId(`subject-edit-${prefix}-R1`).click();
  await page.getByTestId("subject-sex-MALE").click();
  await page.getByTestId("subject-save").click();
  await expect(rows.filter({ hasText: `${prefix}-R1` }).locator("[data-sex]")).toHaveAttribute("data-sex", "MALE");

  await chooseText(page, "subject-filter-species", "Rat");
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute("data-code", `${prefix}-R1`);
  await page.getByTestId("subject-filter-species").hover();
  await page.locator('[data-test="subject-filter-species"] .n-base-clear').click();
  await expect(rows).toHaveCount(2);

  // Only animals that belong to no group: the seeded M01 is allocated.
  await fill(page, "list-search", "M01");
  await expect(rows).toHaveCount(1);
  await expect(rows.first().getByTestId("subject-allocation")).toContainText("E2E");
  await page.getByTestId("subject-filter-unallocated").click();
  await expect(rows).toHaveCount(0);
  await fill(page, "list-search", prefix);
  await expect(rows).toHaveCount(2);
  await expect(page.getByTestId("subject-unallocated")).toHaveCount(2);
});

test("a subject's page shows its allocation and records weights", async ({ page, request }) => {
  const { api, data } = await signIn(page, request);
  const code = unique("W");
  const subject = await api("POST", "/subjects", { code, species: "MOUSE", sex: "FEMALE" });

  await page.goto("/subjects");
  await fill(page, "list-search", code);
  await page.getByTestId(`subject-open-${code}`).click();
  await expect(page).toHaveURL(new RegExp(`/subjects/${subject.id}$`));
  await expect(page.getByTestId("subject-allocation-card").getByTestId("subject-unallocated")).toBeVisible();
  await expect(page.getByTestId("subject-latest-weight")).toContainText("No weight");

  const weigh = async (grams) => {
    await fill(page, "weight-grams-input", grams);
    await page.getByTestId("weight-save").click();
  };
  await weigh("21.5");
  await expect(page.getByTestId("weight-row")).toHaveCount(1);
  await expect(page.getByTestId("subject-latest-weight")).toContainText("21.5 g");
  await weigh("22.25");
  await expect(page.getByTestId("weight-row")).toHaveCount(2);
  await expect(page.getByTestId("weight-row").first()).toContainText("22.25 g");
  await expect(page.getByTestId("weight-row").first()).toContainText("+0.75 g");
  await expect(page.getByTestId("subject-latest-weight")).toContainText("22.25 g");

  // An allocated animal names its study and group.
  await page.goto(`/subjects/${data.subjects.M04}`);
  await expect(page.getByTestId("subject-experiment")).toHaveText("E2E");
  await expect(page.getByTestId("subject-allocation-card")).toContainText("G2");
});
