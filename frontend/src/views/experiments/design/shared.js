// Shared pieces of the experiment design tabs: the enums the API speaks, how
// they read in words, and a cache of paradigm manifests in the current language.
import { reactive } from "vue";
import { useI18n } from "vue-i18n";
import { useRoute, useRouter } from "vue-router";
import { paradigmCatalog } from "../../../api/protocols.js";

export const ROLES = ["HEALTHY_CONTROL", "SHAM", "DISEASE_CONTROL", "POSITIVE_CONTROL", "TREATMENT"];
export const PURPOSES = ["INDUCTION", "VEHICLE", "TREATMENT"];

// What a group of each role can ever receive, as the API enforces it: a
// healthy control and a sham only a vehicle, a disease control its induction
// and a vehicle, the treated groups also their one treatment. A healthy
// control may receive nothing at all (a naive control).
export const ROLE_PURPOSES = {
  HEALTHY_CONTROL: ["VEHICLE"],
  SHAM: ["VEHICLE"],
  DISEASE_CONTROL: ["INDUCTION", "VEHICLE"],
  POSITIVE_CONTROL: ["INDUCTION", "VEHICLE", "TREATMENT"],
  TREATMENT: ["INDUCTION", "VEHICLE", "TREATMENT"],
};

export const UNITS = ["mg/kg", "ug/kg", "mL/kg", "IU/kg", "mg", "ug", "g", "mL", "uL", "IU"];
export const ROUTES = [
  "ORAL",
  "INTRAPERITONEAL",
  "SUBCUTANEOUS",
  "INTRAVENOUS",
  "INTRAMUSCULAR",
  "INTRANASAL",
  "INTRACEREBROVENTRICULAR",
  "TOPICAL",
  "INHALATION",
];
export const FREQUENCIES = ["ONCE", "DAILY", "TWICE_DAILY", "WEEKLY", "EVERY_N_DAYS"];
export const EXCLUSION_REASONS = ["DIED", "HUMANE_ENDPOINT", "INDUCTION_FAILED", "TECHNICAL", "OTHER"];
export const ENDPOINT_KINDS = ["PRIMARY", "SECONDARY"];
export const DIRECTIONS = ["HIGHER_IS_BETTER", "LOWER_IS_BETTER"];

// Each role has its own colour, taken from the theme so both themes work.
const ROLE_TAG = {
  HEALTHY_CONTROL: "success",
  SHAM: "default",
  DISEASE_CONTROL: "error",
  POSITIVE_CONTROL: "warning",
  TREATMENT: "info",
};
const ROLE_COLOR = {
  HEALTHY_CONTROL: "var(--good)",
  SHAM: "var(--muted)",
  DISEASE_CONTROL: "var(--bad)",
  POSITIVE_CONTROL: "var(--warn)",
  TREATMENT: "var(--accent)",
};
export const roleTagType = (role) => ROLE_TAG[role] || "default";
export const roleColor = (role) => ROLE_COLOR[role] || "var(--line)";

const PURPOSE_TAG = { INDUCTION: "error", VEHICLE: "default", TREATMENT: "info" };
export const purposeTagType = (purpose) => PURPOSE_TAG[purpose] || "default";

// A study date ("2026-10-15") is a calendar day: it is read in local time so
// it never shifts by a day across time zones.
function calendarDate(value) {
  const [y, m, d] = String(value).split("-").map(Number);
  return new Date(y, m - 1, d);
}

export function todayISO() {
  const now = new Date();
  const pad = (n) => String(n).padStart(2, "0");
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
}

export function useDesignFormat() {
  const { t, locale } = useI18n();
  const number = (value) => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 6 }).format(Number(value));

  return {
    number,
    role: (role) => t(`design.roles.${role}`),
    purpose: (purpose) => t(`design.purposes.${purpose}`),
    route: (route) => t(`design.routes.${route}`),
    unit: (unit) => t(`design.units.${unit}`),
    // "10 mg/kg"
    dose: (item) => `${number(item.amount)} ${t(`design.units.${item.unit}`)}`,
    frequency: (item) =>
      item.frequency === "EVERY_N_DAYS"
        ? t("design.frequencyEvery", { n: item.intervalDays })
        : t(`design.frequencies.${item.frequency}`),
    // "Day 0" or "Days 0 to 14"
    days: (item) =>
      item.startDay === item.endDay
        ? t("design.studyDay", { day: item.startDay })
        : t("design.studyDays", { start: item.startDay, end: item.endDay }),
    studyDay: (day) => t("design.studyDay", { day }),
    // "Baseline (Day -1)", or just "Day 14" when the name already says it.
    timepoint: (tp) => {
      const day = t("design.studyDay", { day: tp.studyDay });
      return tp.name.trim().toLowerCase() === day.toLowerCase() ? tp.name : `${tp.name} (${day})`;
    },
    date: (value) => (value ? new Intl.DateTimeFormat(locale.value, { dateStyle: "medium" }).format(calendarDate(value)) : "-"),
    dateTime: (value) =>
      value ? new Intl.DateTimeFormat(locale.value, { dateStyle: "medium", timeStyle: "short" }).format(new Date(value)) : "-",
  };
}

// Moves the experiment page to another tab.
export function useTabLink() {
  const route = useRoute();
  const router = useRouter();
  return (tab) => router.replace({ query: { ...route.query, tab } });
}

// Paradigm summaries and manifests by language. Names, trial types, parameter
// labels and metric labels come from the API in the language asked for.
const cache = reactive({ lists: {}, manifests: {} });
const pending = new Map();

function once(key, load, store) {
  if (store[key] || pending.has(key)) return pending.get(key) || Promise.resolve(store[key]);
  const promise = load()
    .then((value) => {
      store[key] = value;
      return value;
    })
    .finally(() => pending.delete(key));
  pending.set(key, promise);
  return promise;
}

export function useParadigms() {
  const { locale } = useI18n();
  const lang = () => (locale.value === "tr" ? "tr" : "en");

  const loadList = () => once(`list:${lang()}`, () => paradigmCatalog.list(lang()).then((r) => r.data || []), cache.lists);
  const loadManifest = (key, version) =>
    once(`${lang()}:${key}:${version}`, () => paradigmCatalog.version(key, version, lang()), cache.manifests);

  return {
    loadList,
    loadManifest,
    list: () => cache.lists[`list:${lang()}`] || [],
    manifest: (key, version) => cache.manifests[`${lang()}:${key}:${version}`] || null,
    // Display name of a paradigm, falling back to its key until the list loads.
    name: (key) => (cache.lists[`list:${lang()}`] || []).find((p) => p.key === key)?.name || key,
    // Any loaded manifest of the paradigm, for labels shared across versions.
    anyManifest: (key) =>
      Object.entries(cache.manifests).find(([k]) => k.startsWith(`${lang()}:${key}:`))?.[1] || null,
  };
}
