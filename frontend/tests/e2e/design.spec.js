// Designing a study from an empty draft to a running study: groups with their
// regimens, timepoints, a protocol with a session setting and a second
// version, endpoints, the readiness checklist, the start with day 0, and then
// allocating, randomising and excluding animals. Once started, the design is
// shown read-only.
import { expect, test } from "@playwright/test";
import { chooseValue, fill, signIn, unique } from "./helpers.js";

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
  await addGroup("G1", "Healthy", "HEALTHY_CONTROL");
  await addItem("G1", { purpose: "VEHICLE", substance: "Saline", amount: "10", unit: "mL/kg", route: "ORAL", frequency: "DAILY" });
  await addGroup("G2", "Disease control", "DISEASE_CONTROL", model);
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

  // Timepoints and a protocol of two steps; the rotarod has session settings.
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
  await chooseValue(page, "step-paradigm", "OPEN_FIELD", { scope: form.getByTestId("step-1"), search: "Open" });
  await form.getByTestId("step-add").click();
  await chooseValue(page, "step-paradigm", "ROTAROD", { scope: form.getByTestId("step-2"), search: "Rota" });
  await chooseValue(page, "step-trial-type", "ACCELERATING", { scope: form.getByTestId("step-2") });
  await fill(form, "step-2-session-start_rpm", "6");
  await page.getByTestId("protocol-save").click();
  const protocol = page.getByTestId("protocol-Battery");
  await expect(protocol.getByTestId("protocol-steps").locator("tbody tr")).toHaveCount(2);
  await expect(protocol.getByTestId("protocol-steps")).toContainText("6 rpm");

  // A second version starts from the first; the first never changes.
  await protocol.getByTestId("protocol-new-version").click();
  await fill(form.getByTestId("step-1"), "step-trials", "3");
  await fill(form, "protocol-version-notes", "Three open field trials");
  await page.getByTestId("protocol-save").click();
  await expect(protocol.getByTestId("protocol-version-select")).toBeVisible();
  await expect(protocol).toContainText("2 version(s)");
  await expect(protocol).toContainText("Three open field trials");

  // Endpoints: one primary only.
  const experimentId = studyUrl.split("/").pop();
  const day7 = (await api("GET", `/experiments/${experimentId}/timepoints`)).data.find((tp) => tp.name === "Day 7").id;
  await page.goto(`${studyUrl}?tab=endpoints`);
  const addEndpoint = async (metric, kind) => {
    await page.getByTestId("endpoint-add").click();
    await chooseValue(page, "endpoint-paradigm", "OPEN_FIELD");
    await chooseValue(page, "endpoint-metric", metric);
    await chooseValue(page, "endpoint-timepoint", day7);
    await page.getByTestId(`endpoint-kind-${kind}`).click();
    await page.getByTestId("endpoint-save").click();
  };
  await addEndpoint("distance_cm", "PRIMARY");
  const rows = page.getByTestId("endpoints-table").locator("tbody tr");
  await expect(rows).toHaveCount(1);
  await addEndpoint("mean_speed_cm_s", "PRIMARY");
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
  const today = new Intl.DateTimeFormat("en", { dateStyle: "medium" }).format(new Date());
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
  await expect(page.getByTestId("fill-G1")).toContainText("2 of 2 animals");
  await expect(page.getByTestId("fill-G2")).toContainText("2 of 2 animals");

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
