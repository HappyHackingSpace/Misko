/**
 * Paradigm metric engine (docs/METRIC_ENGINE.md).
 *
 * A run collects EVENTS; each paradigm derives its METRIC RESULTS from them. This
 * is the single computation both manual entry (operator logs events) and the CV
 * service (signals -> events) flow through. Results are data, never a verdict.
 *
 * Event shape: { type: string, t: number (seconds from run start), payload?: {} }.
 *
 * Each metric has a RULE describing how it is derived:
 *   - source 'event':    aggMethod count|sum|first|last|any over matching events
 *   - source 'computed': aggMethod 'derived', a formula over already-computed metrics
 *   - source 'cvOnly':   supplied directly by the CV service (in cvInputs)
 * Templated metrics (zone-keyed, e.g. zone_time_s) produce a { zoneKey: value } map.
 */
import { metricsForParadigm } from "./metrics.js";

const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v : 0);

// --- low-level helpers over the event list ---

const matches = (ev, type, where = {}) =>
  ev.type === type && Object.entries(where).every(([k, v]) => ev.payload?.[k] === v);

const countOf = (events, type, where) => events.filter((e) => matches(e, type, where)).length;
const firstT = (events, type, where) => {
  const hit = events.filter((e) => matches(e, type, where)).sort((a, b) => num(a.t) - num(b.t))[0];
  return hit ? num(hit.t) : null;
};
const sumField = (events, type, field) =>
  events.filter((e) => e.type === type).reduce((s, e) => s + num(e.payload?.[field]), 0);
const anyOf = (events, type) => events.some((e) => e.type === type);
const maxT = (events) => events.reduce((m, e) => Math.max(m, num(e.t)), 0);

/** Per-zone occupancy time by pairing zone_enter/zone_exit (or to run end). */
function zoneTimes(events, runEnd) {
  const ordered = [...events].filter((e) => e.type === "zone_enter" || e.type === "zone_exit")
    .sort((a, b) => num(a.t) - num(b.t));
  const open = {}; // zone -> enter t
  const total = {};
  for (const e of ordered) {
    const z = e.payload?.zone;
    if (!z) continue;
    if (e.type === "zone_enter") open[z] = num(e.t);
    else if (e.type === "zone_exit" && open[z] != null) {
      total[z] = (total[z] ?? 0) + (num(e.t) - open[z]);
      delete open[z];
    }
  }
  // Zones still open at run end count until runEnd.
  for (const [z, t] of Object.entries(open)) total[z] = (total[z] ?? 0) + Math.max(0, runEnd - t);
  return total;
}
const zoneEntryCounts = (events) => {
  const out = {};
  for (const e of events) if (e.type === "zone_enter" && e.payload?.zone) out[e.payload.zone] = (out[e.payload.zone] ?? 0) + 1;
  return out;
};

/**
 * Per-metric derivation rules. Keyed by metric key. A metric without a rule and
 * not in cvInputs is simply absent (not every metric is collected every run).
 * `ctx` = { events, runEnd, metrics (so far), cvInputs }.
 */
const RULES = {
  // --- run-level ---
  duration_s: { source: "event", fn: (c) => c.runEnd || null },
  trial_duration_s: { source: "event", fn: (c) => c.runEnd || null },

  // --- zoned (templated) ---
  zone_time_s: { source: "event", templated: true, fn: (c) => zoneTimes(c.events, c.runEnd) },
  zone_entries: { source: "event", templated: true, fn: (c) => zoneEntryCounts(c.events) },
  quadrant_time_s: { source: "event", templated: true, fn: (c) => zoneTimes(c.events, c.runEnd) },

  // --- counts ---
  center_entries_count: { source: "event", fn: (c) => countOf(c.events, "zone_enter", { zone: "center" }) },
  open_arm_entries_count: { source: "event", fn: (c) => countOf(c.events, "zone_enter", { zone: "open" }) },
  closed_arm_entries_count: { source: "event", fn: (c) => countOf(c.events, "zone_enter", { zone: "closed" }) },
  total_arm_entries_count: { source: "event", fn: (c) => countOf(c.events, "zone_enter") },
  platform_crossings_count: { source: "event", fn: (c) => countOf(c.events, "platform_cross") },
  light_entries_count: { source: "event", fn: (c) => countOf(c.events, "zone_enter", { zone: "light" }) },
  transitions_count: { source: "event", fn: (c) => countOf(c.events, "transition") },
  risk_assessment_count: { source: "event", fn: (c) => countOf(c.events, "risk_assessment") },
  primary_errors_count: { source: "event", fn: (c) => countOf(c.events, "error", { kind: "primary" }) },
  total_errors_count: { source: "event", fn: (c) => countOf(c.events, "error") },
  shock_count: { source: "event", fn: (c) => countOf(c.events, "shock") },

  // --- durations summed from events (payload.seconds) ---
  immobility_s: { source: "event", fn: (c) => sumField(c.events, "immobile", "seconds") },
  interaction_time_s: { source: "event", fn: (c) => sumField(c.events, "interaction", "seconds") },

  // --- latencies (first occurrence) ---
  escape_latency_s: { source: "event", fn: (c) => firstT(c.events, "platform_reached") },
  latency_to_open_arm_s: { source: "event", fn: (c) => firstT(c.events, "zone_enter", { zone: "open" }) },
  latency_to_dark_s: { source: "event", fn: (c) => firstT(c.events, "zone_enter", { zone: "dark" }) },
  latency_to_zone_s: { source: "event", templated: false, fn: (c) => firstT(c.events, "zone_enter") },
  latency_to_fall_s: { source: "event", fn: (c) => firstT(c.events, "fall") },
  primary_latency_s: { source: "event", fn: (c) => firstT(c.events, "target_hole") },
  latency_to_exhaustion_s: { source: "event", fn: (c) => firstT(c.events, "exhaustion") },

  // --- boolean ---
  fall_detected: { source: "event", fn: (c) => anyOf(c.events, "fall") },

  // --- derived ratios (from other metrics) ---
  center_time_ratio: { source: "computed", fn: (c) => ratio(c.metrics.zone_time_s?.center, c.metrics.duration_s) },
  periphery_time_ratio: { source: "computed", fn: (c) => ratio(c.metrics.zone_time_s?.periphery, c.metrics.duration_s) },
  open_arm_time_ratio: { source: "computed", fn: (c) => ratio(c.metrics.zone_time_s?.open, c.metrics.duration_s) },
  closed_arm_time_ratio: { source: "computed", fn: (c) => ratio(c.metrics.zone_time_s?.closed, c.metrics.duration_s) },
  light_time_ratio: { source: "computed", fn: (c) => ratio(c.metrics.zone_time_s?.light, c.metrics.duration_s) },
};

function ratio(a, b) {
  if (typeof a !== "number" || typeof b !== "number" || b <= 0) return null;
  return Math.min(1, a / b);
}

/**
 * Derives a paradigm's metric results from a run's events (+ optional cvOnly
 * values). Returns a metrics object keyed by metric key. Two passes so that
 * `computed` (derived) metrics can read event-derived ones.
 */
export function computeMetrics(paradigmKey, events = [], cvInputs = {}) {
  const defs = metricsForParadigm(paradigmKey);
  const runEnd = maxT(events);
  const metrics = {};
  const ctx = { events, runEnd, metrics, cvInputs };

  // Pass 1: event-derived + cvOnly.
  for (const def of defs) {
    const rule = RULES[def.key];
    if (!rule) {
      if (cvInputs[def.key] != null) metrics[def.key] = cvInputs[def.key]; // cvOnly / unmapped
      continue;
    }
    if (rule.source === "computed") continue; // pass 2
    const v = rule.fn(ctx);
    if (v != null && !(typeof v === "object" && Object.keys(v).length === 0)) metrics[def.key] = v;
  }
  // Pass 2: derived.
  for (const def of defs) {
    const rule = RULES[def.key];
    if (rule?.source !== "computed") continue;
    const v = rule.fn(ctx);
    if (v != null) metrics[def.key] = v;
  }
  return metrics;
}

/** Validates an event against a paradigm's declared eventTypes. */
export function validateEvent(eventTypes, event) {
  if (!event || typeof event !== "object") return "event must be an object";
  const def = eventTypes.find((e) => e.type === event.type);
  if (!def) return `unknown event type: ${event.type}`;
  if (event.t != null && (typeof event.t !== "number" || event.t < 0)) return "t must be a non-negative number";
  return null;
}
