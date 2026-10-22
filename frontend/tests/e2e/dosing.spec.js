// Daily dosing in the seeded study (day 0 was three days ago): the doses due
// on a day per group and animal, a planned dose, a per-kilogram dose that asks
// for a weight first, an unplanned dose and the outcome of an induction.
import { expect, test } from "@playwright/test";
import { chooseValue, fill, signIn } from "./helpers.js";

test("doses due today are recorded, weighing first when needed", async ({ page, request }) => {
  const { data, api } = await signIn(page, request);
  await page.goto(`/experiments/${data.experimentId}?tab=dosing`);
  await expect(page.getByTestId("study-day")).toHaveText("Day 3");
  await expect(page.getByTestId("due-group")).toHaveCount(3);

  // A planned dose: the vehicle of a healthy animal weighed four days ago.
  const m01 = page.locator('[data-test="due-dose"][data-animal="M01"]');
  await expect(m01).toHaveAttribute("data-state", "due");
  await expect(m01).toContainText("10 mL/kg");
  await m01.getByTestId("record-dose").click();
  const dialog = page.getByTestId("dose-dialog");
  await expect(dialog.getByTestId("dose-planned")).toBeVisible();
  await expect(dialog.getByTestId("weight-needed")).toHaveCount(0);
  await dialog.getByTestId("dose-save").click();
  await expect(m01).toHaveAttribute("data-state", "given");

  // The treated group was never weighed: its mg/kg dose asks for a weight,
  // records it, then gives the dose with it.
  const m06 = page.locator('[data-test="due-row"][data-animal="M06"]');
  await expect(m06.getByTestId("due-weight")).toHaveAttribute("data-state", "missing");
  await m06.getByTestId("record-dose").click();
  await expect(dialog.getByTestId("weight-needed")).toBeVisible();
  await expect(dialog.getByTestId("dose-save")).toBeDisabled();
  await fill(dialog, "dose-grams", "27.5");
  await dialog.getByTestId("dose-save").click();
  await expect(m06.getByTestId("due-dose")).toHaveAttribute("data-state", "given");
  await expect(m06.getByTestId("due-weight")).toHaveAttribute("data-state", "ok");
  await expect(m06.getByTestId("due-weight")).toContainText("27.5 g");
  // 5 mg/kg of a 27.5 g mouse.
  await expect(m06.getByTestId("due-dose")).toContainText("0.1375 mg");

  // An unplanned dose names everything itself.
  const lDopa = (await api("GET", "/catalog/substances")).data.find((s) => s.name === "L-DOPA").id;
  await page.getByTestId("unplanned-dose").click();
  await chooseValue(page, "dose-animal", data.allocations.M05, { search: "M05" });
  await chooseValue(page, "dose-purpose", "TREATMENT");
  await chooseValue(page, "dose-substance", lDopa, { search: "L-DOPA" });
  await fill(dialog, "dose-amount", "2");
  await chooseValue(page, "dose-unit", "mg/kg");
  await chooseValue(page, "dose-route", "ORAL");
  await dialog.getByTestId("dose-save").click();
  await expect(dialog).toBeHidden();
  const history = page.getByTestId("dose-history");
  const lDopaRow = history.locator("tbody tr").filter({ hasText: "L-DOPA" });
  await expect(lDopaRow).toHaveCount(1);
  await expect(lDopaRow.getByTestId("history-animal")).toContainText("M05");
  await expect(lDopaRow.getByTestId("history-dose")).toHaveText("2 mg/kg");

  // Yesterday's doses that were never given show as missing.
  await page.getByTestId("day-previous").click();
  await expect(page.getByTestId("study-day")).toHaveText("Day 2");
  await expect(page.locator('[data-test="due-dose"][data-animal="M01"]')).toHaveAttribute("data-state", "missing");
  await page.getByTestId("day-today").click();
  await expect(page.getByTestId("study-day")).toHaveText("Day 3");
});

test("the outcome of an induction is recorded once", async ({ page, request }) => {
  const { data } = await signIn(page, request);
  await page.goto(`/experiments/${data.experimentId}?tab=dosing`);
  const outcomes = page.getByTestId("induction-outcomes");
  // M03 was confirmed when the study was seeded; M04 was induced but not assessed.
  await expect(outcomes.locator("tbody tr").filter({ hasText: "M03" }).getByTestId("induction-status")).toHaveText("Confirmed");
  const m04 = outcomes.locator("tbody tr").filter({ hasText: "M04" });
  await m04.getByTestId("record-induction").click();
  await page.getByTestId("induction-choice").getByText("Not confirmed").click();
  await page.getByTestId("induction-save").click();
  await expect(m04.getByTestId("induction-status")).toHaveText("Not confirmed");
  await expect(m04.getByTestId("record-induction")).toHaveCount(0);
});
