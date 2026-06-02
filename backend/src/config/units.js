/**
 * Canonical measurement units (Step 2 - scientific contract).
 *
 * Only these units may enter Misko result metrics. Pixel values are never
 * stored in result metrics; the CV service converts everything to apparatus
 * coordinates (cm) before sending.
 *
 * Source: docs/MEASUREMENTS.md, section 3.2 (Unit policy).
 */
export const UNITS = Object.freeze({
  CM: "cm", // position and distance
  MM: "mm", // small apparatus diameter (pole/rod); not a result metric
  CM_S: "cm_s", // speed
  S: "s", // duration
  C: "c", // temperature (apparatus parameter); not a result metric
  COUNT: "count", // counts such as entries, crossings, falls
  RATIO: "ratio", // 0..1 range, may be shown as a percentage in the UI
  PERCENT: "percent", // fields carrying a percentage directly
  DEG: "deg", // angle (heading/pose later)
  RPM: "rpm", // rotarod revolutions
  G: "g", // subject weight (physiology, not a CV result)
  BOOLEAN: "boolean", // event present/absent
});

export const UNIT_LIST = Object.freeze(Object.values(UNITS));

const UNIT_SET = new Set(UNIT_LIST);

/** Is the given unit in the canonical unit list? */
export function isCanonicalUnit(unit) {
  return UNIT_SET.has(unit);
}
