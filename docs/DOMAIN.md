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
                                                     environment + metrics +
                                                     expected results)
                                            |
                                            v
Test = Subject + Scenario  ->  result (metrics) + passed
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
  parameters, zones, the metric dictionary and suggested acceptance templates.
  Read-only catalog: `GET /api/paradigms`, `GET /api/paradigms/:key`.
- **Environment** ("Ortam"): a lab's named, persisted instance of a paradigm. It
  stores a self-contained snapshot `{ paradigmKey, schemaVersion, apparatus,
  zones }`; apparatus values are validated against the code-fixed parameter
  ranges and locked at test time. A lab can hold many environments per paradigm
  (e.g. two distinct Morris water tanks). CRUD under `/api/environments`.

> The physical rig **is** the Environment. There is no separate `Apparatus`
> model. Camera-to-cm calibration is a later concern (roadmap Step 5).

## 3. Scenario (the central experiment definition)

A `Scenario` is the complete, reusable definition of one experiment. It is built
once by a researcher; after that, running a test is just "pick a subject and go".

A scenario carries every detail of the experiment:

- `name`
- `environmentId` - the environment to run on (and therefore the paradigm).
- **selected metrics** - which metrics from the paradigm's metric dictionary this
  scenario collects.
- **expected results / acceptance criteria** - the pass/fail contract per metric,
  using the Step 2 acceptance engine (`backend/src/config/acceptance.js`).
- **session parameters** - trial count, duration, etc., from the paradigm.

Because the environment carries the paradigm, and the scenario carries the
metrics and expected results, a scenario is fully self-describing.

## 4. Test (one run)

A `Test` is a single run of a `Scenario` against a `Subject`.

- `scenarioId` - what is being run (brings the environment, paradigm, metrics and
  expected results with it).
- `subjectId` - which mouse.
- `operatorId`, `deviceId?` - who ran it, on what device.
- `status` - `PENDING | RUNNING | DONE | FAILED`, `startedAt`, `endedAt`.
- `result` (JSON) - the metrics the CV service computed, plus QC and artifact
  URLs. Validated against the scenario's selected metrics / paradigm dictionary.
- `passed` - evaluation of `result` against the scenario's expected results
  (null when the scenario defines none).

There are **no** per-test paradigm, environment, metric or acceptance choices -
they all live on the scenario. Starting a test only asks for a subject (and the
operating device).

## 5. Roles and permissions (RBAC)

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

## 6. What stays out of Mişko

- Per-frame trajectory/pose telemetry and event rows -> CV service's PostgreSQL.
- Video / trajectory files -> object storage; Mişko stores only URLs.
- No editable DB rows for paradigm definitions (they are code).
