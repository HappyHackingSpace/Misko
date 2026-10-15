// Locale-aware formatting for the dosing and test screens: dates, numbers and
// doses always follow the panel language, and enums are shown in words.
import { useI18n } from "vue-i18n";

// A calendar date "YYYY-MM-DD" of an instant in a time zone (the browser's when omitted).
export function dateIn(instant, timeZone) {
  const parts = new Intl.DateTimeFormat("en-CA", { timeZone, year: "numeric", month: "2-digit", day: "2-digit" }).format(instant);
  return parts.slice(0, 10);
}

// Adds whole days to a "YYYY-MM-DD" date.
export function addDays(date, days) {
  const [y, m, d] = date.split("-").map(Number);
  const shifted = new Date(Date.UTC(y, m - 1, d + days));
  return shifted.toISOString().slice(0, 10);
}

// The tag colour of every status these screens show.
const TAGS = {
  PLANNED: "info",
  IN_PROGRESS: "warning",
  COMPLETED: "success",
  CANCELLED: "error",
  PENDING: "warning",
  VERIFIED: "success",
  REJECTED: "error",
  NOT_REQUIRED: "default",
  WAITING_FOR_VIDEO: "default",
  WAITING_FOR_CALIBRATION: "warning",
  CALIBRATED: "success",
  VALID: "success",
  CONFIRMED: "success",
  NOT_CONFIRMED: "error",
};
export const tagType = (status) => TAGS[status] || "default";

export function useExecutionFormat() {
  const { t, locale } = useI18n();
  const number = (value, digits = 3) =>
    value === null || value === undefined || value === "" ? "" : Number(value).toLocaleString(locale.value, { maximumFractionDigits: digits });
  const dateTime = (iso) => (iso ? new Date(iso).toLocaleString(locale.value, { dateStyle: "medium", timeStyle: "short" }) : "");
  const time = (iso) => (iso ? new Date(iso).toLocaleTimeString(locale.value, { hour: "2-digit", minute: "2-digit" }) : "");
  // A calendar date, either "YYYY-MM-DD" (shown as that day) or an instant.
  const date = (value) => {
    if (!value) return "";
    const instant = /^\d{4}-\d{2}-\d{2}$/.test(value) ? new Date(`${value}T12:00:00Z`) : new Date(value);
    return instant.toLocaleDateString(locale.value, { dateStyle: "medium", timeZone: /^\d{4}-\d{2}-\d{2}$/.test(value) ? "UTC" : undefined });
  };
  const dose = (d) => (d ? `${number(d.amount, 6)} ${d.unit}` : "");
  const grams = (value) => (value ? `${number(value, 3)} g` : "");
  const route = (key) => (key ? t(`execution.routes.${key}`) : "");
  const purpose = (key) => (key ? t(`execution.purposes.${key}`) : "");
  const status = (key) => (key ? t(`execution.statuses.${key}`) : "");
  const bytes = (size) => {
    if (!size) return "";
    const mb = size / (1024 * 1024);
    return mb >= 1 ? `${number(mb, 1)} MB` : `${number(size / 1024, 1)} KB`;
  };
  return { number, dateTime, time, date, dose, grams, route, purpose, status, bytes };
}
