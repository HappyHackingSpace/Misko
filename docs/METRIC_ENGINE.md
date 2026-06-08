# Mişko - Paradigm-Owned Metric Engine (roadmap)

> Status: design / roadmap. Decided with the team:
> - Backend stays **Node** (no Go rewrite); add the computation architecture here.
> - Each **paradigm owns its computation** (hardcoded, SOLID, one service per paradigm).
> - A run collects **events**; the paradigm derives **metric results** from them,
>   **live** (as events are added) and **finalized on Finish**.
> - Same engine for **manual entry** (operator logs events) and the future **CV**
>   service (signals → events). Results are **data, never a pass/fail verdict**.

## 1. The three layers (recap)

| Layer | What | Where |
|---|---|---|
| **Signal** | raw, real-time time series (trajectory, timestamps, pose) | CV service only (never Mişko) |
| **Event** | a discrete occurrence during a run (`zone_enter{center}`, `fall`, `platform_reached`, `lap{duration}`) | Mişko, per test run |
| **Metric (definition)** | what is measured for a paradigm (key, unit, valueType, validRange, aggMethod, source) | code (paradigm spec) |
| **Metric result (value)** | the value for one run, **derived from events** by the paradigm's `compute()` | Mişko (`Test.result`) |

Manual entry produces **events**; CV produces **signals → events** (and a few
`cvOnly` metrics directly). Both flow through the **same paradigm `compute()`**.

## 2. Paradigm spec interface (hardcoded, one per paradigm)

Each paradigm is a self-contained module implementing a common interface; the
registry is the single list (Open/Closed - add a paradigm = add a module).

```ts
interface ParadigmSpec {
  key: string;                 // 'MWM' | 'OPEN_FIELD' | ...
  name: string; category: string;
  apparatusParameters: Field[];
  zones(config): Zone[];

  // Metric DEFINITIONS (extends today's dictionary entry):
  //   ...existing (unit, valueType, validRange, templated)
  //   + aggMethod: 'count'|'sum'|'first'|'last'|'mean'|'max'|'min'|'any'|'derived'|'cvOnly'
  //   + source:    'event' | 'computed' | 'cvOnly'
  metrics: MetricDef[];

  // Event TYPES this paradigm understands (drives the manual event-logging UI):
  //   { type, label, payload?: { zone?: enum, value?: number }, ... }
  eventTypes: EventType[];

  // THE paradigm's own calculation. Pure function: events (+ optional cvOnly
  // values + apparatus config) -> { [metricKey]: value }. Used live and on finish.
  compute(events: Event[], config, extra?): Record<string, MetricValue>;

  validate(config): void;
}
```

`compute()` is where each paradigm's hardcoded logic lives, e.g. for `OPEN_FIELD`:
count `zone_enter{center}` → `center_entries_count`; sum center intervals →
`zone_time_s.center`; `center_time_ratio = zone_time_s.center / duration_s`.

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
- **metrics** are always recomputed by `compute()` - never hand-edited directly
  (except `cvOnly`/optional values the user may type, kept in `cvInputs`).

## 4. Metric source taxonomy

- **event** (`count`/`sum`/`first`/`last`/`any`): derived from logged events.
  Shown in the manual UI as event buttons.
- **computed** (`derived`): ratios/indices computed from other metrics inside
  `compute()` (e.g. `center_time_ratio`, `discrimination_index`). Not entered.
- **cvOnly**: trajectory/sensor values a human cannot log (`distance_cm`,
  `mean_speed_cm_s`, `heading_error_deg`, `rpm_at_fall`). Hidden from manual entry
  (or an explicit optional override); the CV service supplies them.

(The full metric → aggMethod/source table is in §6; it is reviewed and tuned per
paradigm, the per-paradigm "own calculation".)

## 5. Data flow

```
MANUAL:  operator taps event buttons → POST events → compute(events) → metrics (live)
                                                   → Finish → final compute, status DONE
CV:      camera → signals → CV derives events (+ cvOnly values) → POST → same compute
```

- Live: appending an event re-runs `compute` and updates `metrics` (preview).
- Finish: marks the environment DONE; final `compute` is authoritative.
- One `compute` per paradigm, one code path for both sources.

## 6. Roadmap (phased)

**Phase 0 - done (uncommitted on `feat/manual-result-entry`):** removed the
pass/fail verdict + per-scenario acceptance; per-environment result structure;
metric dictionary with `valueType`/`validRange`/`templated`; `validateMetrics`.

**Phase 1 - paradigm spec + metric taxonomy (code, no UI):**
- Add `aggMethod` + `source` to every metric definition (the §6 table).
- Add `eventTypes` to each paradigm spec.
- Define the `ParadigmSpec.compute(events, config)` interface + a registry.
- Implement `compute()` per paradigm, hardcoded, with unit tests. Order:
  `OPEN_FIELD`, `EPM`, `ROTAROD`, `MWM` first, then the rest.

**Phase 2 - run events + computation (backend):**
- `result.environments[envId].events` storage; `POST /api/tests/:id/
  environments/:envId/events` (append) and event-validation against the
  paradigm's `eventTypes`.
- Recompute metrics on each append (live) and on Finish (authoritative).
- `cvInputs` channel for `cvOnly` values.

**Phase 3 - manual event-logging UI (frontend):**
- Per-environment panel: buttons for the paradigm's `eventTypes` (with zone
  pickers where needed), a running event log (add/remove), and a **live metric
  preview** computed from events; Finish.
- `cvOnly` metrics shown read-only / optional override.

**Phase 4 - CV integration (Step 4):**
- CV pushes `events` (+ `cvInputs`) to the same endpoint; identical `compute`.
- Service auth (`X-Service-Key`), idempotency, artifacts.

**Phase 5 - analysis:** aggregate metric results across tests/groups (no verdicts).

Docs (DOMAIN/usage/integration EN+TR) update with each phase.

## 7. Out of scope / non-goals

- No backend rewrite (stays Node/Express/Prisma).
- No pass/fail verdict anywhere - results are data.
- Raw per-frame signals never enter Mişko (CV service only).
