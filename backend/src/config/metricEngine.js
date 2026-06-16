/**
 * Paradigm metric engine (docs/METRIC_ENGINE.md).
 *
 * A run collects EVENTS; each paradigm's metrics are DERIVED from them. This is
 * the single computation both manual entry (operator logs events) and the CV
 * service (signals -> events) flow through. Results are data, never a verdict.
 *
 * Derivation is DECLARATIVE: each metric has a rule in METRIC_RULES describing
 * how it is computed (a data spec, not code). Adding a metric/paradigm = declare
 * it; the generic interpreter below handles it - no new engine code unless a
 * genuinely new aggregation kind is needed. The same declarations are the contract
 * the CV service reads later.
 *
 * Event shape: { type: string, t: number (seconds from run start), payload?: {} }.
 * Rule shape:
 *   { source: 'event'|'computed'|'cvOnly', agg, ...args }
 *   agg: 'runEnd' | 'count' | 'sumField' | 'firstT' | 'any'
 *      | 'zoneTime' | 'zoneCount' (templated zone maps) | 'ratio' (computed)
 */
import { metricsForParadigm } from "./metrics.js";

const num = (v) => (typeof v === "number" && Number.isFinite(v) ? v : 0);

/**
 * Declarative derivation rules per metric key. Metric keys are shared across
 * paradigms, so the derivation is declared once here and reused; each paradigm
 * just lists which metrics it has (metricsForParadigm) and which events it emits.
 */
export const METRIC_RULES = Object.freeze({
  // run-level
  duration_s: { source: "event", agg: "runEnd" },
  trial_duration_s: { source: "event", agg: "runEnd" },

  // zoned (templated -> { zoneKey: value })
  zone_time_s: { source: "event", agg: "zoneTime", templated: true },
  quadrant_time_s: { source: "event", agg: "zoneTime", templated: true },
  zone_entries: { source: "event", agg: "zoneCount", templated: true },

  // counts
  center_entries_count: { source: "event", agg: "count", event: "zone_enter", match: { zone: "center" } },
  open_arm_entries_count: { source: "event", agg: "count", event: "zone_enter", match: { zone: "open" } },
  closed_arm_entries_count: { source: "event", agg: "count", event: "zone_enter", match: { zone: "closed" } },
  total_arm_entries_count: { source: "event", agg: "count", event: "zone_enter" },
  light_entries_count: { source: "event", agg: "count", event: "zone_enter", match: { zone: "light" } },
  platform_crossings_count: { source: "event", agg: "count", event: "platform_cross" },
  transitions_count: { source: "event", agg: "count", event: "transition" },
  risk_assessment_count: { source: "event", agg: "count", event: "risk_assessment" },
  primary_errors_count: { source: "event", agg: "count", event: "error", match: { kind: "primary" } },
  total_errors_count: { source: "event", agg: "count", event: "error" },
  shock_count: { source: "event", agg: "count", event: "shock" },

  // durations summed from event payloads
  immobility_s: { source: "event", agg: "sumField", event: "immobile", field: "seconds" },
  interaction_time_s: { source: "event", agg: "sumField", event: "interaction", field: "seconds" },

  // latencies (first occurrence)
  escape_latency_s: { source: "event", agg: "firstT", event: "platform_reached" },
  latency_to_open_arm_s: { source: "event", agg: "firstT", event: "zone_enter", match: { zone: "open" } },
  latency_to_dark_s: { source: "event", agg: "firstT", event: "zone_enter", match: { zone: "dark" } },
  latency_to_zone_s: { source: "event", agg: "firstT", event: "zone_enter" },
  latency_to_fall_s: { source: "event", agg: "firstT", event: "fall" },
  primary_latency_s: { source: "event", agg: "firstT", event: "target_hole" },
  latency_to_exhaustion_s: { source: "event", agg: "firstT", event: "exhaustion" },

  // boolean
  fall_detected: { source: "event", agg: "any", event: "fall" },

  // computed (derived from other metrics)
  center_time_ratio: { source: "computed", agg: "ratio", of: "zone_time_s.center", over: "duration_s" },
  periphery_time_ratio: { source: "computed", agg: "ratio", of: "zone_time_s.periphery", over: "duration_s" },
  open_arm_time_ratio: { source: "computed", agg: "ratio", of: "zone_time_s.open", over: "duration_s" },
  closed_arm_time_ratio: { source: "computed", agg: "ratio", of: "zone_time_s.closed", over: "duration_s" },
  light_time_ratio: { source: "computed", agg: "ratio", of: "zone_time_s.light", over: "duration_s" },
});

// --- generic interpreters over the event list ---

const matchEv = (ev, type, where = {}) =>
  ev.type === type && Object.entries(where).every(([k, v]) => ev.payload?.[k] === v);

/** Per-zone occupancy time by pairing zone_enter/zone_exit (open zones close at run end). */
function zoneTimes(events, runEnd) {
  const ordered = events.filter((e) => e.type === "zone_enter" || e.type === "zone_exit")
    .slice().sort((a, b) => num(a.t) - num(b.t));
  const open = {};
  const total = {};
  for (const e of ordered) {
    const z = e.payload?.zone;
    if (!z) continue;
    if (e.type === "zone_enter") open[z] = num(e.t);
    else if (open[z] != null) { total[z] = (total[z] ?? 0) + (num(e.t) - open[z]); delete open[z]; }
  }
  for (const [z, t] of Object.entries(open)) total[z] = (total[z] ?? 0) + Math.max(0, runEnd - t);
  return total;
}
function zoneCounts(events) {
  const out = {};
  for (const e of events) if (e.type === "zone_enter" && e.payload?.zone) out[e.payload.zone] = (out[e.payload.zone] ?? 0) + 1;
  return out;
}
function pick(obj, path) {
  return path.split(".").reduce((o, k) => (o == null ? undefined : o[k]), obj);
}

/** Applies one declarative rule. ctx = { events, runEnd, metrics }. */
function applyRule(rule, ctx) {
  const { events, runEnd, metrics } = ctx;
  switch (rule.agg) {
    case "runEnd": return runEnd || null;
    case "count": return events.filter((e) => matchEv(e, rule.event, rule.match)).length;
    case "sumField": return events.filter((e) => e.type === rule.event).reduce((s, e) => s + num(e.payload?.[rule.field]), 0);
    case "firstT": {
      const hit = events.filter((e) => matchEv(e, rule.event, rule.match)).sort((a, b) => num(a.t) - num(b.t))[0];
      return hit ? num(hit.t) : null;
    }
    case "any": return events.some((e) => e.type === rule.event);
    case "zoneTime": return zoneTimes(events, runEnd);
    case "zoneCount": return zoneCounts(events);
    case "ratio": {
      const a = pick(metrics, rule.of);
      const b = pick(metrics, rule.over);
      if (typeof a !== "number" || typeof b !== "number" || b <= 0) return null;
      return Math.min(1, a / b);
    }
    default: return null;
  }
}

const empty = (v) => v == null || (typeof v === "object" && Object.keys(v).length === 0);

/**
 * Derives a paradigm's metric results from a run's events (+ optional cvOnly
 * values). Two passes so `computed` metrics can read event-derived ones.
 */
export function computeMetrics(paradigmKey, events = [], cvInputs = {}) {
  const defs = metricsForParadigm(paradigmKey);
  const runEnd = events.reduce((m, e) => Math.max(m, num(e.t)), 0);
  const metrics = {};
  const ctx = { events, runEnd, metrics };

  for (const def of defs) {
    const rule = METRIC_RULES[def.key];
    if (!rule) { if (cvInputs[def.key] != null) metrics[def.key] = cvInputs[def.key]; continue; }
    if (rule.source === "cvOnly") { if (cvInputs[def.key] != null) metrics[def.key] = cvInputs[def.key]; continue; }
    if (rule.source === "computed") continue; // pass 2
    const v = applyRule(rule, ctx);
    if (!empty(v)) metrics[def.key] = v;
  }
  for (const def of defs) {
    const rule = METRIC_RULES[def.key];
    if (rule?.source !== "computed") continue;
    const v = applyRule(rule, ctx);
    if (!empty(v)) metrics[def.key] = v;
  }
  return metrics;
}

/** Validates an event against a paradigm's declared eventTypes. */
export function validateEvent(eventTypes, event) {
  if (!event || typeof event !== "object") return "event must be an object";
  if (!eventTypes.some((e) => e.type === event.type)) return `unknown event type: ${event.type}`;
  if (event.t != null && (typeof event.t !== "number" || event.t < 0)) return "t must be a non-negative number";
  return null;
}
