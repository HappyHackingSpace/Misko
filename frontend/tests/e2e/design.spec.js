// Designing a study from an empty draft to a running study: the healthy
// control every study needs, groups with the regimens their roles allow, timepoints, a protocol with a session setting and a second
// version, endpoints, the readiness checklist, the start with day 0, and then
// allocating, randomising and excluding animals. Once started, the design is
// shown read-only.
import { expect, test } from "@playwright/test";
import { chooseValue, field, fill, signIn, unique } from "./helpers.js";

// The purposes listed in the open purpose menu (the chosen one is also drawn in
// the select itself, so the menu is the scope).
const purposeOptions = (page) => page.locator(".n-base-select-menu").filter({ visible: true }).getByTestId("regimen-purpose-option");

// Closes the open purpose menu, then the regimen dialog, without saving.
async function closeRegimenForm(page) {
  const form = page.getByTestId("regimen-form");
  await page.keyboard.press("Escape");
  try {
    await expect(form).toBeHidden({ timeout: 1000 });
  } catch {
    await page.keyboard.press("Escape");
    await expect(form).toBeHidden();
  }
}

test("a study is designed, started and filled with animals", async ({ page, request }) => {
  test.slow();
  const { api } = await signIn(page, request);
  const substances = (await api("GET", "/catalog/substances")).data;
  const substance = (name) => substances.find((s) => s.name === name).id;
  const model = (await api("GET", "/catalog/disease-models")).data.find((m) => m.name === "MPTP Parkinson").id;
  const code = unique("STUDY");

  // The experiment itself.
  await page.goto("/experiments");
  await page.getByTestId("experiment-new").click();
  await fill(page, "experiment-code", code);
  await fill(page, "experiment-title", "Designed in the browser");
  await fill(page, "experiment-hypothesis", "Compound X rescues the deficit");
  await fill(page, "experiment-randomisation", "Block randomisation");
  await page.getByTestId("experiment-save").click();
  await expect(page).toHaveURL(/\/experiments\/[0-9a-f-]{36}\?tab=design$/);
  const studyUrl = page.url().split("?")[0];
  await expect(page.getByTestId("experiment-status")).toHaveText("Draft");

  // Groups and what they receive.
  const addGroup = async (groupCode, name, role, disease) => {
    await page.getByTestId("group-add").click();
    await fill(page, "group-code", groupCode);
    await fill(page, "group-name", name);
    await chooseValue(page, "group-role", role);
    if (disease) await chooseValue(page, "group-disease", disease);
    await fill(page, "group-target", "2");
    await page.getByTestId("group-save").click();
    await expect(page.getByTestId(`group-card-${groupCode}`)).toBeVisible();
  };
  const addItem = async (groupCode, item) => {
    await page.getByTestId(`regimen-add-${groupCode}`).click();
    await chooseValue(page, "regimen-purpose", item.purpose);
    await chooseValue(page, "regimen-substance", substance(item.substance));
    await fill(page, "regimen-amount", item.amount);
    await chooseValue(page, "regimen-unit", item.unit);
    await chooseValue(page, "regimen-route", item.route);
    await chooseValue(page, "regimen-frequency", item.frequency);
    await fill(page, "regimen-start", "0");
    if (item.frequency !== "ONCE") await fill(page, "regimen-end", "7");
    await page.getByTestId("regimen-save").click();
    await expect(page.getByTestId("regimen-form")).toBeHidden();
  };
  // No study starts without a healthy control; the checklist offers to add one
  // and opens the group dialog with the role chosen.
  await page.goto(`${studyUrl}?tab=overview`);
  await page.getByTestId("readiness-unmet").locator('[data-rule="readiness.noGroups"]').getByRole("button", { name: "Add a group" }).click();
  await expect(page.getByTestId("group-form")).toBeVisible();
  await page.goto(`${studyUrl}?tab=overview`);
  await expect(page.getByTestId("readiness-unmet").locator('[data-rule="readiness.healthyControlMissing"]')).toContainText("Add a healthy control group");
  await page.getByTestId("readiness-add-healthy").click();
  await expect(page).toHaveURL(/tab=design/);
  await expect(page.getByTestId("group-form")).toBeVisible();
  await expect(field(page, "group-code")).toHaveValue("G1");
  await expect(field(page, "group-name")).toHaveValue("Healthy control");
  await expect(page.getByTestId("group-role")).toContainText("Healthy control");
  await fill(page, "group-target", "2");
  await page.getByTestId("group-save").click();
  const healthy = page.getByTestId("group-card-G1");
  await expect(healthy).toBeVisible();

  // A healthy control receives no substance at all: its item is only the
  // period without one, a first and a last study day.
  await expect(healthy.getByTestId("regimen-empty-G1")).toHaveText("No substance is given to this group (naive control).");
  await expect(healthy.getByTestId("regimen-rule-G1")).toContainText("Receives no substance at all");
  await expect(healthy.getByTestId("regimen-add-G1")).toHaveText("Add item");
  await healthy.getByTestId("regimen-add-G1").click();
  const periodForm = page.getByTestId("regimen-form");
  await expect(periodForm).toHaveAttribute("data-kind", "period");
  await expect(periodForm.locator("label.fld > span")).toHaveText(["First study day", "Last study day"]);
  for (const hidden of ["regimen-purpose", "regimen-substance", "regimen-amount", "regimen-unit", "regimen-route", "regimen-frequency"]) {
    await expect(page.getByTestId(hidden)).toHaveCount(0);
  }
  await fill(page, "regimen-start", "-3");
  await fill(page, "regimen-end", "14");
  await page.getByTestId("regimen-save").click();
  await expect(periodForm).toBeHidden();
  await expect(healthy.getByTestId("regimen-empty-G1")).toHaveCount(0);
  await expect(healthy.getByTestId("regimen-item")).toHaveCount(1);
  await expect(healthy.getByTestId("regimen-period")).toHaveText("No substance: day -3 to 14");

  await addGroup("G2", "Disease control", "DISEASE_CONTROL", model);
  // A disease control is induced and gets a vehicle, never a treatment.
  await expect(page.getByTestId("regimen-add-G2")).toHaveText("Add item");
  await page.getByTestId("regimen-add-G2").click();
  await page.getByTestId("regimen-purpose").click();
  await expect(purposeOptions(page)).toHaveCount(2);
  await expect(purposeOptions(page).and(page.locator('[data-value="TREATMENT"]'))).toHaveCount(0);
  await closeRegimenForm(page);
  await addItem("G2", { purpose: "INDUCTION", substance: "MPTP", amount: "20", unit: "mg/kg", route: "INTRAPERITONEAL", frequency: "ONCE" });
  await addItem("G2", { purpose: "VEHICLE", substance: "Saline", amount: "10", unit: "mL/kg", route: "ORAL", frequency: "DAILY" });
  await expect(page.getByTestId("group-card-G1").getByTestId("regimen-item")).toHaveCount(1);
  await expect(page.getByTestId("group-card-G2").getByTestId("regimen-item")).toHaveCount(2);
  await expect(page.getByTestId("group-card-G2").getByTestId("regimen-dose").first()).toContainText("20 mg/kg");

  // The checklist names what is missing and the study cannot start yet.
  await page.goto(`${studyUrl}?tab=overview`);
  const unmet = page.getByTestId("readiness-unmet");
  await expect(unmet.locator('[data-rule="readiness.noTimepoints"]')).toBeVisible();
  await expect(unmet.locator('[data-rule="readiness.primaryEndpointMissing"]')).toBeVisible();
  await expect(page.getByTestId("experiment-start")).toBeDisabled();

  // Timepoints and a protocol of two steps; the treadmill has session settings.
  await page.goto(`${studyUrl}?tab=timeline`);
  for (const [name, day, position] of [["Baseline", -1, 1], ["Day 7", 7, 2]]) {
    await page.getByTestId("timepoint-add").click();
    await fill(page, "timepoint-name", name);
    await fill(page, "timepoint-day", day);
    await fill(page, "timepoint-position", position);
    await page.getByTestId("timepoint-save").click();
    await expect(page.getByTestId("timepoint-form")).toBeHidden();
  }
  await expect(page.getByTestId("timepoints-table").locator("tbody tr")).toHaveCount(2);

  await page.getByTestId("protocol-add").click();
  const form = page.getByTestId("protocol-form");
  await fill(form, "protocol-name", "Battery");
  await chooseValue(page, "step-paradigm", "MWM", { scope: form.getByTestId("step-1"), search: "Morris" });
  // The trial types come with the paradigm's manifest; MWM has two to choose from.
  await expect(form.getByTestId("step-1").getByTestId("step-trial-type").locator(".n-base-selection--disabled")).toHaveCount(0);
  await chooseValue(page, "step-trial-type", "ACQUISITION", { scope: form.getByTestId("step-1") });
  await form.getByTestId("step-add").click();
  await chooseValue(page, "step-paradigm", "TREADMILL", { scope: form.getByTestId("step-2"), search: "Tread" });
  await expect(form.getByTestId("step-2").getByTestId("step-trial-type").locator(".n-base-selection--disabled")).toHaveCount(0);
  await chooseValue(page, "step-trial-type", "ENDURANCE", { scope: form.getByTestId("step-2") });
  await fill(form, "step-2-session-start_speed_cm_s", "6");
  await page.getByTestId("protocol-save").click();
  const protocol = page.getByTestId("protocol-Battery");
  await expect(protocol.getByTestId("protocol-steps").locator("tbody tr")).toHaveCount(2);
  await expect(protocol.getByTestId("protocol-steps")).toContainText("6 cm/s");

  // A second version starts from the first; the first never changes.
  await protocol.getByTestId("protocol-new-version").click();
  await fill(form.getByTestId("step-1"), "step-trials", "3");
  await fill(form, "protocol-version-notes", "Three water maze trials");
  await page.getByTestId("protocol-save").click();
  await expect(protocol.getByTestId("protocol-version-select")).toBeVisible();
  await expect(protocol).toContainText("2 version(s)");
  await expect(protocol).toContainText("Three water maze trials");

  // Endpoints: one primary only.
  const experimentId = studyUrl.split("/").pop();
  const day7 = (await api("GET", `/experiments/${experimentId}/timepoints`)).data.find((tp) => tp.name === "Day 7").id;
  await page.goto(`${studyUrl}?tab=endpoints`);
  const addEndpoint = async (metric, kind) => {
    await page.getByTestId("endpoint-add").click();
    await chooseValue(page, "endpoint-paradigm", "MWM");
    await chooseValue(page, "endpoint-metric", metric);
    await chooseValue(page, "endpoint-timepoint", day7);
    await page.getByTestId(`endpoint-kind-${kind}`).click();
    await page.getByTestId("endpoint-save").click();
  };
  await addEndpoint("path_length_cm", "PRIMARY");
  const rows = page.getByTestId("endpoints-table").locator("tbody tr");
  await expect(rows).toHaveCount(1);
  await addEndpoint("mean_swim_speed_cm_s", "PRIMARY");
  await expect(page.getByTestId("endpoint-error")).toContainText("already has a primary endpoint");
  await page.getByTestId("endpoint-kind-SECONDARY").click();
  await page.getByTestId("endpoint-save").click();
  await expect(rows).toHaveCount(2);

  // Ready: start with day 0 today.
  await page.goto(`${studyUrl}?tab=overview`);
  await expect(page.getByTestId("readiness-ready")).toBeVisible();
  await page.getByTestId("experiment-start").click();
  await page.getByTestId("start-confirm").click();
  await expect(page.getByTestId("experiment-status")).toHaveText("Running");
  const today = new Intl.DateTimeFormat("en", { dateStyle: "medium", timeZone: "UTC" }).format(new Date());
  await expect(page.getByTestId("overview-day-zero")).toContainText(today.split(",")[0]);

  // The design is now read-only.
  await page.goto(`${studyUrl}?tab=design`);
  await expect(page.getByTestId("design-frozen")).toBeVisible();
  await expect(page.getByTestId("group-add")).toHaveCount(0);
  await expect(page.getByTestId("regimen-add-G1")).toHaveCount(0);
  await expect(page.getByTestId("group-edit-G1")).toHaveCount(0);
  await page.goto(`${studyUrl}?tab=timeline`);
  await expect(page.getByTestId("protocol-Battery")).toBeVisible();
  await expect(page.getByTestId("timepoint-add")).toHaveCount(0);
  await page.goto(`${studyUrl}?tab=endpoints`);
  await expect(rows).toHaveCount(2);
  await expect(page.getByTestId("endpoint-add")).toHaveCount(0);

  // Animals: one put into a group by hand, three randomised up to the targets.
  const groups = (await api("GET", `/experiments/${experimentId}/groups`)).data;
  const prefix = unique("A");
  const codes = [1, 2, 3, 4].map((n) => `${prefix}-${n}`);
  for (const [i, c] of codes.entries()) await api("POST", "/subjects", { code: c, species: "MOUSE", sex: i % 2 ? "FEMALE" : "MALE" });
  await page.goto(`${studyUrl}?tab=animals`);
  const drawer = page.getByTestId("allocate-drawer");
  const candidates = drawer.getByTestId("allocate-subjects").locator("tbody tr");
  const open = async (count) => {
    await page.getByTestId("allocate-open").click();
    await fill(drawer, "allocate-search", prefix);
    await expect(candidates).toHaveCount(count);
  };
  await open(4);
  await candidates.filter({ hasText: codes[0] }).locator(".n-checkbox").click();
  await drawer.getByTestId("allocate-mode-group").click();
  await chooseValue(page, "allocate-group", groups.find((g) => g.code === "G1").id);
  await fill(drawer, "allocate-cage", "K1");
  await page.getByTestId("allocate-submit").click();
  await expect(drawer).toBeHidden();

  await open(3);
  await drawer.locator(".n-data-table-th--selection .n-checkbox").click();
  await page.getByTestId("allocate-submit").click();
  await expect(drawer).toBeHidden();

  await fill(page, "animals-search", prefix);
  const allocated = page.getByTestId("allocations-table").locator("tbody tr");
  await expect(allocated).toHaveCount(4);
  await expect(page.getByTestId("fill-G1")).toContainText("2 of 2 subjects");
  await expect(page.getByTestId("fill-G2")).toContainText("2 of 2 subjects");

  // Excluding keeps the animal on the list with its reason.
  const first = allocated.filter({ hasText: codes[0] });
  await expect(first).toContainText("K1");
  await first.getByTestId("allocation-exclude").click();
  await chooseValue(page, "exclude-reason", "DIED");
  await fill(page, "exclude-note", "Found dead in the cage");
  await page.getByTestId("allocation-dialog-save").click();
  await expect(first.getByTestId("allocation-excluded")).toContainText("Died");
  await expect(page.getByTestId("fill-G1")).toContainText("excluded");
});

test("a treatment group is offered one treatment, a sham only a vehicle and a healthy control no substance", async ({ page, request }) => {
  const { api } = await signIn(page, request);
  const substances = (await api("GET", "/catalog/substances")).data;
  const substance = (name) => substances.find((s) => s.name === name).id;
  const model = (await api("GET", "/catalog/disease-models")).data.find((m) => m.name === "MPTP Parkinson").id;
  const experiment = await api("POST", "/experiments", { code: unique("ROLES"), title: "Purposes by role" });
  const base = `/experiments/${experiment.id}`;
  const treated = await api("POST", `${base}/groups`, { code: "T1", name: "Treated", role: "TREATMENT", diseaseModelId: model });
  await api("POST", `${base}/groups`, { code: "S1", name: "Sham", role: "SHAM", diseaseModelId: model });
  const healthy = await api("POST", `${base}/groups`, { code: "H1", name: "Healthy", role: "HEALTHY_CONTROL" });
  const item = (purpose, name) => ({ groupId: treated.id, purpose, substanceId: substance(name), amount: "5", unit: "mg/kg",
    route: "ORAL", frequency: "DAILY", startDay: 0, endDay: 7 });

  await page.goto(`/experiments/${experiment.id}?tab=design`);
  const purposes = purposeOptions(page);
  const offered = async (groupCode) => {
    await page.getByTestId(`regimen-add-${groupCode}`).click();
    await page.getByTestId("regimen-purpose").click();
    await expect(purposes.first()).toBeVisible();
    const values = await purposes.evaluateAll((nodes) => nodes.map((n) => n.dataset.value));
    await closeRegimenForm(page);
    return values;
  };
  await expect(page.getByTestId("regimen-add-S1")).toHaveText("Add vehicle");
  expect(await offered("S1")).toEqual(["VEHICLE"]);
  expect(await offered("T1")).toEqual(["INDUCTION", "VEHICLE", "TREATMENT"]);

  // A healthy control's form has only the two study days, no purpose.
  await expect(page.getByTestId("regimen-add-H1")).toHaveText("Add item");
  await page.getByTestId("regimen-add-H1").click();
  await expect(page.getByTestId("regimen-form")).toHaveAttribute("data-kind", "period");
  await expect(page.getByTestId("regimen-purpose")).toHaveCount(0);
  await expect(page.getByTestId("regimen-substance")).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page.getByTestId("regimen-form")).toBeHidden();
  await expect(api("POST", `${base}/regimen-items`, { ...item("VEHICLE", "Saline"), groupId: healthy.id })).rejects.toThrow(/regimen.purposeNotAllowed/);

  // Once the group has its treatment, a second one is not offered.
  await api("POST", `${base}/regimen-items`, item("TREATMENT", "Compound X"));
  await page.reload();
  expect(await offered("T1")).toEqual(["INDUCTION", "VEHICLE"]);
  // The API refuses what the form no longer offers.
  await expect(api("POST", `${base}/regimen-items`, { ...item("INDUCTION", "MPTP"), groupId: (await api("GET", `${base}/groups`)).data.find((g) => g.code === "S1").id }))
    .rejects.toThrow(/regimen.purposeNotAllowed/);
});

test("each unmet start rule offers its own fix", async ({ page, request }) => {
  const { api } = await signIn(page, request);
  const model = (await api("GET", "/catalog/disease-models")).data.find((m) => m.name === "MPTP Parkinson").id;
  const experiment = await api("POST", "/experiments", { code: unique("FIXES"), title: "One fix per rule" });
  // A treatment group misses its treatment and induction, a sham its vehicle;
  // neither has a disease control to compare against or a target size.
  await api("POST", `/experiments/${experiment.id}/groups`, { code: "T1", name: "Treated", role: "TREATMENT", diseaseModelId: model });
  await api("POST", `/experiments/${experiment.id}/groups`, { code: "S1", name: "Sham", role: "SHAM", diseaseModelId: model });

  const overview = `/experiments/${experiment.id}?tab=overview`;
  const action = (rule) => page.getByTestId("readiness-unmet").locator(`[data-rule="readiness.${rule}"]`).getByTestId("readiness-action").first();
  await page.goto(overview);
  const fixes = {
    vehicleItemMissing: "Add a vehicle item",
    inductionItemMissing: "Add the induction item",
    treatmentItemMissing: "Add the treatment item",
    diseaseControlMissing: "Add a disease control group",
    targetSizeMissing: "Set the target size",
  };
  for (const [rule, label] of Object.entries(fixes)) await expect(action(rule)).toHaveText(label);

  // A missing item opens the group's regimen dialog with that purpose chosen
  // among the three a treatment group may receive.
  await action("inductionItemMissing").click();
  await expect(page).toHaveURL(/tab=design/);
  await expect(page.getByTestId("regimen-form")).toBeVisible();
  await expect(page.getByTestId("regimen-purpose")).toContainText("Induction");
  await closeRegimenForm(page);

  // The missing disease control opens a new group with that role and the same disease model.
  await page.goto(overview);
  await action("diseaseControlMissing").click();
  await expect(page.getByTestId("group-form")).toBeVisible();
  await expect(page.getByTestId("group-role")).toContainText("Disease control");
  await expect(page.getByTestId("group-disease")).toContainText("MPTP Parkinson");
  await page.keyboard.press("Escape");

  // The missing target size opens the group itself.
  await page.goto(overview);
  await page.getByTestId("readiness-unmet").locator('[data-rule="readiness.targetSizeMissing"]', { hasText: "S1 Sham" }).getByTestId("readiness-action").click();
  await expect(page.getByTestId("group-form")).toBeVisible();
  await expect(field(page, "group-code")).toHaveValue("S1");
});
