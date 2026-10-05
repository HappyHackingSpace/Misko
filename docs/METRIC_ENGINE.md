> **Outdated.** This document describes the earlier Node/Prisma implementation (scenarios, `X-Service-Key`, event-based manual entry). The current system is the Go backend with analysis runs and workers. The source of truth is [backend/README.md](../backend/README.md), [worker/README.md](../worker/README.md) and the Go catalog in `backend/internal/paradigms`.

# Mişko - Paradigm-Owned Metric Engine (roadmap)

> Status: implemented (declarative engine). Decided with the team:
> - Backend stays **Node** (no Go rewrite); the computation architecture lives here.
> - Computation is **declarative**: each metric has a rule in `METRIC_RULES`
>   (a data spec, not code), interpreted by one generic engine. Adding a metric or
>   paradigm = declare it; no per-paradigm service code unless a genuinely new
>   aggregation kind is needed.
> - A run collects **events**; the engine derives **metric results** from them,
>   **live** (as events are added) and **finalized on Finish**.
> - Same engine for **manual entry** (operator logs events) and the future **CV**
>   service (signals → events). Results are **data, never a pass/fail verdict**.

## 1. The three layers (recap)

| Layer | What | Where |
|---|---|---|
| **Signal** | raw, real-time time series (trajectory, timestamps, pose) | CV service only (never Mişko) |
| **Event** | a discrete occurrence during a run (`zone_enter{center}`, `fall`, `platform_reached`, `lap{duration}`) | Mişko, per test run |
| **Metric (definition)** | what is measured for a paradigm (key, unit, valueType, validRange, aggMethod, source) | code (paradigm spec) |
| **Metric result (value)** | the value for one run, **derived from events** by `computeMetrics()` | Mişko (`Test.result`) |

Manual entry produces **events**; CV produces **signals → events** (and a few
`cvOnly` metrics directly). Both flow through the **same `computeMetrics()`**.

## 2. Declarative engine (one engine, data-driven rules)

There is a single engine, `backend/src/config/metricEngine.js`. A paradigm does
not ship code; it declares two things and the engine does the rest:

1. **Which metrics it has** - `metricsForParadigm(key)` (from the metric dictionary).
2. **Which events it emits** - `eventTypes` in the paradigm spec, each carrying a
   CV `detect` spec (see §4.1).

Each metric key has a derivation **rule** in `METRIC_RULES` (a data spec, not a
function):

```ts
type MetricRule =
  | { source: "event"; agg: "runEnd" }                                    // duration_s
  | { source: "event"; agg: "count"; event: string; match?: object }      // center_entries_count
  | { source: "event"; agg: "sumField"; event: string; field: string }    // immobility_s
  | { source: "event"; agg: "firstT"; event: string; match?: object }     // escape_latency_s
  | { source: "event"; agg: "any"; event: string }                        // fall_detected
  | { source: "event"; agg: "zoneTime" }                                  // zone_time_s (templated)
  | { source: "event"; agg: "zoneCount" }                                 // zone_entries (templated)
  | { source: "computed"; agg: "ratio"; of: string; over: string }        // center_time_ratio
  | { source: "cvOnly" };                                                  // distance_cm (from CV)
```

`computeMetrics(paradigmKey, events, cvInputs)` runs two passes: event/cvOnly
metrics first, then `computed` ones (which read the first pass via dotted paths
like `zone_time_s.center`). Generic interpreters (`count`, `sumField`, `firstT`,
`zoneTime`, `zoneCount`, `ratio`, ...) cover every paradigm. Adding a metric =
add a rule; adding a paradigm = list its metrics + event types. New engine code
is only needed for a genuinely new aggregation kind.

## 3. Event model on a run

`Test.result` (per environment) gains an event log; metrics are **derived**:

```json
{
  "schemaVersion": 2,
  "environments": {
    "<envId>": {
      "status": "RUNNING | DONE",
      "startedAt": "...", "endedAt": "...",
      "events":  [ { "type": "zone_enter", "t": 4.2, "payload": { "zone": "center" } },
                   { "type": "zone_exit",  "t": 9.6, "payload": { "zone": "center" } } ],
      "cvInputs": { "distance_cm": 1234 },     // optional: cvOnly values from CV
      "metrics": { "center_entries_count": 1, "zone_time_s": { "center": 5.4 } }  // DERIVED
    }
  }
}
```

- **events** are the source of truth for manual entry.
- **metrics** are always recomputed by `computeMetrics()` - never hand-edited
  directly (except `cvOnly`/optional values the user may type, kept in `cvInputs`).

## 4. Metric source taxonomy

- **event** (`count`/`sumField`/`firstT`/`any`/`zoneTime`/`zoneCount`): derived
  from logged events. Shown in the manual UI as event buttons.
- **computed** (`ratio`): ratios/indices computed from other metrics in a second
  pass (e.g. `center_time_ratio`). Not entered.
- **cvOnly**: trajectory/sensor values a human cannot log (`distance_cm`,
  `mean_speed_cm_s`, `heading_error_deg`, `rpm_at_fall`). Hidden from manual entry
  (or an explicit optional override); the CV service supplies them via `cvInputs`.

### 4.1 Event detection (`detect` spec) - the CV contract

Each `eventTypes` entry carries a `detect` spec describing how the CV service
derives that event from the trajectory/zones. The generic kinds are
**data-driven** (no per-paradigm CV code needed); `custom` flags a dedicated
detector:

| `detect.kind` | Meaning | Example events |
|---|---|---|
| `zone_transition` | animal crosses a zone boundary (`edge: enter\|exit`) | `zone_enter`, `zone_exit`, `transition` |
| `zone_first_enter` | first time a target zone is entered (latency) | `platform_reached`, `target_hole` |
| `speed_below` | speed under a threshold for `min_duration_s` | `immobile` |
| `custom` | needs a dedicated detector / model | `fall`, `interaction`, `risk_assessment` |

This is the key to "**define the paradigm, no CV code change**": a paradigm that
only uses the generic kinds extends coverage purely by declaration. Only `custom`
events require new CV work.

## 5. Data flow

```
MANUAL:  operator taps event buttons → POST events → computeMetrics(events) → metrics (live)
                                                   → Finish → final compute, status DONE
CV:      camera → signals → CV derives events via detect specs (+ cvInputs) → POST → same compute
```

- Live: appending an event re-runs `computeMetrics` and updates `metrics` (preview).
- Finish: marks the environment DONE; the final `computeMetrics` is authoritative.
- One engine, one code path for both sources.

## 6. Roadmap (phased)

**Phase 0 - done:** removed the pass/fail verdict + per-scenario acceptance;
per-environment result structure; metric dictionary with
`valueType`/`validRange`/`templated`.

**Phase 1 - declarative engine + event types - done:**
- `metricEngine.js` with the `METRIC_RULES` data-driven map and generic
  interpreters; `computeMetrics(paradigmKey, events, cvInputs)`.
- `eventTypes` on each paradigm spec, each with a `detect` CV spec (§4.1).
- Removed the dead acceptance machinery (`acceptance.js`, `suggestedAcceptance`,
  `/api/paradigms/acceptance-operators`).
- Coverage is complete for `OPEN_FIELD`; rules for the other paradigms' shared
  metrics are declared and extended as their event types are exercised.

**Phase 2 - run events + computation (backend) - done:**
- `result.environments[envId].events` storage; `POST /api/tests/:id/
  environments/:envId/events` (append) + `DELETE .../events/:index`, validated
  against the paradigm's `eventTypes`.
- Recompute metrics on each append (live) and on Finish (authoritative).
- `cvInputs` channel for `cvOnly` values.

**Phase 3 - manual event-logging UI (frontend) - done:**
- Per-environment panel: a **metric counter bar** on top (live parameter-based
  metric values), then two tabs:
  - **Timeline**: an event-based vertical timeline (each event placed by its
    second `t`, color-coded by event type) plus the editable event log
    (add via the drawer, remove per row).
  - **Charts**: a parameter-based **bar chart** with a metric picker.
- Event-type picker (with zone pickers where needed); Finish/Reopen.
- Paradigm detail page surfaces `eventTypes` + their `detect` kinds.

**Phase 4 - CV integration (Step 4):**
- CV pushes `events` (+ `cvInputs`) to the same endpoint; identical compute. The
  `detect` specs are the contract: generic kinds need no new CV code.
- Service auth (`X-Service-Key`), idempotency, artifacts.

**Phase 5 - analysis:** aggregate metric results across tests/groups (no verdicts).

Docs (DOMAIN/usage/integration EN+TR) update with each phase.

## 7. Out of scope / non-goals

- No backend rewrite (stays Node/Express/Prisma).
- No pass/fail verdict anywhere - results are data.
- Raw per-frame signals never enter Mişko (CV service only).
