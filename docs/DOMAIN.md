# Mişko - Domain Model

> Scope: the data model Mişko owns as the **system of record**. The CV service
> (vision, raw telemetry, video) stays independent - see `docs/INTEGRATION.md`.
> Measurement definitions, metric rules and QC are in `docs/MEASUREMENTS.md`.

Mişko is a **video-only behavioral test platform**. The domain is intentionally
small. Five concepts, one straight line from setup to result:

```
Subject (mouse)
Paradigm (read-only code catalog)  ->  Environment (named physical instance)
                                            |
                                            v
                                         Scenario  (the central definition:
                                                     one or more environments)
                                            |
                                            v
Test = Subject + Scenario  ->  result (events -> derived metrics, per environment)
```

## 0. Tenancy (single-tenant, on-prem)

Mişko is deployed **on-prem, one installation per laboratory**. There is exactly
**one `Laboratory`** (a singleton, not multi-tenant). It holds lab-wide
configuration (`name`, `code`, `timezone`, `settings`). We do not sprinkle a
`labId` across every table - everything implicitly belongs to the one lab.

The installation wizard (CLI, at Docker startup) creates the `Laboratory` and the
`SUPERADMIN` user together; a singleton guard refuses a second laboratory.

## 1. Subject (the mouse)

Kept simple - the rich research-grade subject was dropped from scope.

- `code` (unique) - lab/cage label
- `sex` - `M | F`
- `groupName` - free-text experiment group (e.g. control, model, treated)
- `birthDate`
- `notes`

## 2. Paradigm and Environment

These already exist (Steps 1-2).

- **Paradigm**: a scientific test type **defined in code**, not editable DB rows
  (`backend/src/config/paradigms.js`). It declares the configurable apparatus
  parameters, zones, the metric dictionary and the event types (each with a CV
  `detect` spec). Read-only catalog: `GET /api/paradigms`, `GET /api/paradigms/:key`.
- **Environment** ("Ortam"): a lab's named, persisted instance of a paradigm. It
  stores a self-contained snapshot `{ paradigmKey, schemaVersion, apparatus,
  zones }`; apparatus values are validated against the code-fixed parameter
  ranges and locked at test time. A lab can hold many environments per paradigm
  (e.g. two distinct Morris water tanks). CRUD under `/api/environments`.

> The physical rig **is** the Environment. There is no separate `Apparatus`
> model. Camera-to-cm calibration is a later concern (roadmap Step 5).

## 3. Metric vs metric result (signals stay out)

Three distinct layers, deliberately separated:

- **Signal** - the camera's raw, real-time time series (trajectory `(t, x, y)`,
  timestamps, pose...). Lives in the **CV service**, never in Mişko. The camera
  does not emit "mean speed"; it emits a trajectory.
- **Metric (definition)** - what is measured for a paradigm: the code dictionary
  entry (`key`, `unit`, `valueType`, `validRange`, and the `inputs` it is derived
  from). Paradigm-level, stable.
- **Metric result (value)** - the measured/computed value for one test run. The
  CV service computes it from signals; for manual entry the operator records the
  observable ones directly. Stored in `Test.result`.

Mişko stores **metric results** (derived values), not signals.

## 4. Scenario (the central experiment definition)

A `Scenario` is the reusable definition of one experiment, built once. It carries:

- `name`, `description`.
- **environments** (N-N) - one or more environments to run on, so a scenario can
  span one or more paradigms. Each environment's paradigm fixes its metrics.
- **session parameters** (optional) - trial count, duration, etc.

A scenario carries **no** pass/fail criteria. A behavioral result is **data, not
a verdict** - interpretation and comparison happen at the analysis layer.

## 5. Test (one run)

A `Test` is a run of a `Scenario` against a `Subject`. The operator runs it
**environment by environment**: Start an environment, record its metric results
(manual now, camera later), Finish. The test is DONE when every environment is.

- `scenarioId`, `subjectId`, `operatorId`.
- `status` - `PENDING | RUNNING | DONE | FAILED`, `startedAt`, `endedAt`.
- `result` (JSON) - per environment:
  `{ schemaVersion, environments: { [envId]: { status, startedAt, endedAt, metrics } } }`.
  Metric keys/values are validated against the environment's paradigm dictionary
  (`valueType`, `validRange`, templated zone maps). The exact same shape the CV
  service will push later - manual entry just fills the observable metrics by hand.

There is **no** `passed`/verdict. Starting a test only asks for a subject and a
scenario; everything else comes from the scenario's environments.

## 6. Roles and permissions (RBAC)

Five roles with a **code-defined permission matrix** (no DB-editable permissions);
see `backend/src/config/permissions.js`.

| Role | Meaning |
|---|---|
| `SUPERADMIN` | Bootstrap user. Full control incl. users and lab config. |
| `LAB_MANAGER` | Runs the lab: manages users, configures the lab, full data access. |
| `RESEARCHER` | Builds environments and scenarios, creates and runs tests. |
| `TECHNICIAN` | Executes tests; limited edits. |
| `VIEWER` | Read-only across everything. |

Writes are gated by permissions (`requirePermission(...)`): `user:manage`,
`lab:configure`, `subject:write`, `apparatus:write` (environments/scenarios),
`test:write`, `test:run`, and read access (`*:read`).

## 7. What stays out of Mişko

- Per-frame trajectory/pose telemetry and event rows -> CV service's PostgreSQL.
- Video / trajectory files -> object storage; Mişko stores only URLs.
- No editable DB rows for paradigm definitions (they are code).
