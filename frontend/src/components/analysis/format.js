// Wording and number formatting shared by the review, result and report screens.
import { computed, reactive, watch } from "vue";
import { useI18n } from "vue-i18n";
import { paradigms } from "../../api/analysis.js";

// Paradigm names, metric and event labels in the current language, loaded on
// demand. Unknown keys fall back to the key itself.
export function useParadigmText() {
  const { locale } = useI18n();
  const state = reactive({ names: {}, versions: {} });

  async function loadNames() {
    try {
      const list = await paradigms.list();
      state.names = Object.fromEntries(list.map((p) => [p.key, p.name]));
    } catch {
      // Names are a nicety; the key still reads.
    }
  }
  async function need(key, version) {
    const id = `${key}@${version}`;
    if (state.versions[id]) return state.versions[id];
    try {
      state.versions[id] = await paradigms.version(key, version);
    } catch {
      state.versions[id] = null;
    }
    return state.versions[id];
  }
  watch(locale, () => {
    state.versions = {};
    loadNames();
  });
  loadNames();

  const manifestOf = (key, version) => state.versions[`${key}@${version}`] || null;
  // Any loaded version of the paradigm will do for a label.
  const anyManifest = (key) => Object.entries(state.versions).find(([id, m]) => m && id.startsWith(`${key}@`))?.[1] || null;
  const paradigmName = (key) => state.names[key] || key;
  const metricLabel = (paradigmKey, metricKey) =>
    anyManifest(paradigmKey)?.metrics?.find((m) => m.key === metricKey)?.label || metricKey;
  const eventLabels = (paradigmKey) => Object.fromEntries((anyManifest(paradigmKey)?.events || []).map((e) => [e.type, e.label]));

  return { need, manifestOf, paradigmName, metricLabel, eventLabels };
}

// A count or a switch reads as the bare number.
const BARE_UNITS = new Set(["count", "boolean"]);

export function useNumbers() {
  const { locale, t, te } = useI18n();
  const formatter = computed(() => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 2 }));
  const signed = computed(() => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 1, signDisplay: "exceptZero" }));

  // Units in the panel's language ("cm/sn" in Turkish), from the paradigm wording.
  const unitText = (unit) => {
    if (!unit || BARE_UNITS.has(unit)) return "";
    return te(`paradigms.units.${unit}`) ? t(`paradigms.units.${unit}`) : unit;
  };
  const number = (value) => (typeof value === "number" && Number.isFinite(value) ? formatter.value.format(value) : "–");
  // A ratio reads as a share in percent; other units follow the number.
  function metric(value, unit) {
    if (value === null || value === undefined) return "–";
    if (unit === "ratio") return `${number(value * 100)}%`;
    const text = unitText(unit);
    return text ? `${number(value)} ${text}` : number(value);
  }
  // A difference keeps its sign: +10 cm, -60 cm.
  const signedNumber = computed(() => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 2, signDisplay: "exceptZero" }));
  function signedMetric(value, unit) {
    if (typeof value !== "number" || !Number.isFinite(value)) return "–";
    if (unit === "ratio") return `${signedNumber.value.format(value * 100)}%`;
    const text = unitText(unit);
    return text ? `${signedNumber.value.format(value)} ${text}` : signedNumber.value.format(value);
  }
  const percent = (value) => (typeof value === "number" && Number.isFinite(value) ? `${signed.value.format(value)}%` : "–");
  // Doses are stored in millionths of the unit: 3000000 mg/kg is 3 mg/kg.
  const dose = (doseMicro, unit) => (doseMicro == null ? "" : `${number(doseMicro / 1_000_000)} ${unit || ""}`.trim());
  const dateTime = (iso) => (iso ? new Date(iso).toLocaleString(locale.value) : "");
  const date = (iso) => (iso ? new Date(iso).toLocaleDateString(locale.value) : "");
  const missingReason = (reason) => (reason && te(`analysis.missingReasons.${reason}`) ? t(`analysis.missingReasons.${reason}`) : reason || "");
  // Why a job failed, in words. A QC failure carries the rule after a colon
  // ("QC_FAILED: max_lost_frame_ratio 0.4 <= 0.2 not met"), kept as the detail.
  function failure(reason) {
    if (!reason) return "";
    const [code, ...rest] = reason.split(":");
    const detail = rest.join(":").trim();
    if (!te(`analysis.failureReasons.${code}`)) return reason;
    return detail ? `${t(`analysis.failureReasons.${code}`)} (${detail})` : t(`analysis.failureReasons.${code}`);
  }

  return { number, metric, signedMetric, unitText, percent, dose, dateTime, date, missingReason, failure };
}

// Tag colours of review, run and experiment statuses.
export const reviewTag = (status) => ({ PENDING: "warning", ACCEPTED: "success", REJECTED: "error" })[status] || "default";
export const runTag = (status) => ({ QUEUED: "default", RUNNING: "info", SUCCEEDED: "success", FAILED: "error" })[status] || "default";

// Saves a blob as a file the way a link download would.
export function saveBlob(blob, fileName) {
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  document.body.appendChild(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
