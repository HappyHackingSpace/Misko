---
title: Domain model
description: The lean, scenario-centric data model Mişko owns as the system of record.
---

Mişko is a **video-only behavioral test platform** with a deliberately small
domain. Mişko owns this model as the **system of record**; the CV service stays
independent. Metric definitions, MWM normalization and quality-control semantics
are in [Measurement architecture](../measurements/).

Five concepts, one straight line from setup to result:

```txt
Subject (mouse)
Paradigm (read-only code catalog) -> Environment (named physical instance)
                                          |
                                          v
                                       Scenario  (environment + metrics +
                                                   expected results)
                                          |
                                          v
Test = Subject + Scenario -> result (metrics) + passed
```

## Tenancy (single-tenant, on-prem)

Mişko runs **one laboratory per installation** - a singleton, not multi-tenant.

- **`Laboratory`** (singleton): name, code, timezone, settings.
- **Installation wizard (CLI, at startup):** takes the lab name + superadmin
  email and creates the `Laboratory` **and** the `SUPERADMIN` together. A
  singleton guard refuses a second lab; the step is idempotent.

## Subject (the mouse)

Kept simple: `code` (unique), `sex`, `groupName` (free-text experiment group),
`birthDate`, `notes`.

## Paradigm and Environment

- **Paradigm:** a scientific test type **defined in code**, not an editable DB
  row. The spec (parameters, zones, metric dictionary, suggested acceptance) is
  engineering-owned and stable; only the values are user data. Read-only catalog
  (`GET /api/paradigms`). Registry: `MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`,
  `Y_MAZE`, `NOVEL_OBJECT`, `BARNES_MAZE`, `THREE_CHAMBER`, `LIGHT_DARK`, `POLE`,
  `TREADMILL`.
- **Environment (Ortam):** a lab's named, persisted instance of a paradigm. It
  stores a self-contained snapshot `{ paradigmKey, schemaVersion, apparatus,
  zones }`; apparatus values are validated against the code-fixed parameter
  ranges and locked at test time. Many environments per paradigm (e.g. two
  distinct Morris water tanks).

The physical rig **is** the Environment - there is no separate `Apparatus` model.
Camera-to-cm calibration is a later concern (roadmap Step 5).

## Scenario (the central experiment definition)

A `Scenario` is the complete, reusable definition of one experiment, built once
by a researcher. It carries every detail:

- `name`.
- `environmentId` - the environment to run on (and therefore the paradigm).
- **selected metrics** - which metrics from the paradigm's dictionary it collects.
- **expected results / acceptance criteria** - the pass/fail contract per metric.
- **session parameters** - trial count, duration, etc., from the paradigm.

Because the environment carries the paradigm and the scenario carries the metrics
and expected results, a scenario is fully self-describing.

## Test (one run)

A `Test` is a single run of a `Scenario` against a `Subject`:

- `scenarioId` (brings the environment, paradigm, metrics and expected results),
  `subjectId`, `operatorId`, `deviceId?`.
- `status` (`PENDING | RUNNING | DONE | FAILED`), `startedAt`, `endedAt`.
- `result` (JSON) - CV-computed metrics + QC + artifact URLs, validated against
  the scenario's metrics.
- `passed` - `result` evaluated against the scenario's expected results (null
  when none).

There are **no** per-test paradigm, environment, metric or acceptance choices -
they all live on the scenario. Starting a test only asks for a subject (and the
operating device).

## Roles & permissions (RBAC)

Five lab-oriented roles with a **code-defined permission matrix** (no DB-editable
permissions):

| Permission \ Role | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|:--:|:--:|:--:|:--:|:--:|
| `user:manage`     | ✅ | ✅ | — | — | — |
| `lab:configure`   | ✅ | ✅ | — | — | — |
| `subject:write`   | ✅ | ✅ | ✅ | — | — |
| `apparatus:write` (environments & scenarios) | ✅ | ✅ | ✅ | — | — |
| `test:write`      | ✅ | ✅ | ✅ | — | — |
| `test:run`        | ✅ | ✅ | ✅ | ✅ | — |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

Middleware `requirePermission("test:run")` is the primary gate.

## What stays out of Mişko

Per-frame telemetry and event rows live in the CV service's PostgreSQL. Video and
trajectory files live in object storage - Mişko stores only URLs. There is no
timeseries table in Mişko. See [Integration](../integration/).
