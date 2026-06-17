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
                                       Scenario  (one or more environments)
                                          |
                                          v
Test = Subject + Scenario -> per-environment metric results (data, no verdict)
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
  row. The spec (parameters, zones, metric dictionary, and the event types, each
  with a CV detect spec) is
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

A `Scenario` is the reusable definition of one experiment, built once. It carries:

- `name`, `description`.
- **environments** (N-N) - one or more environments to run on, so a scenario can
  span one or more paradigms. Each environment's paradigm fixes its metrics.
- **session parameters** (optional) - trial count, duration, etc.

A scenario carries **no** pass/fail criteria. A behavioral result is **data, not
a verdict** - interpretation happens at the analysis layer.

### Signal vs metric vs metric result

Three separate layers: **signals** (the camera's raw real-time time series -
trajectory, timestamps; they stay in the CV service, never in Mişko), **metrics**
(the paradigm's definitions - what is measured, its unit/range/inputs), and
**metric results** (the values for one run, computed by CV from signals or entered
by hand). Mişko stores metric results, not signals.

## Test (one run)

A `Test` is a run of a `Scenario` against a `Subject`. The operator runs it
**environment by environment** (Start → record metric results → Finish):

- `scenarioId`, `subjectId`, `operatorId`.
- `status` (`PENDING | RUNNING | DONE | FAILED`), `startedAt`, `endedAt`.
- `result` (JSON) - per environment:
  `{ schemaVersion, environments: { [envId]: { status, startedAt, endedAt, metrics } } }`.
  Metric values are validated against the environment's paradigm dictionary. The
  same shape the CV service will push - manual entry fills the observable metrics
  by hand.

There is **no** `passed`/verdict. Starting a test only asks for a subject and a
scenario.

## Comment (test discussion)

A `Comment` is a free-form note attached to a `Test`, forming a discussion
thread.

- `testId`, `authorId`, `body` (raw text), `createdAt`, `updatedAt`.
- `body` is stored verbatim and **never** interpreted as HTML; it is bounded
  (max 2000 chars), trimmed, and stripped of control characters at the API layer.
- Deleting a test cascades to its comments; deleting a user cascades to theirs.

Authorization is deliberately simple and not part of the permission matrix:

- **read / create**: any authenticated user (so every role can comment).
- **edit**: the author only.
- **delete**: the author, or a privileged role (`SUPERADMIN` / `LAB_MANAGER`) for
  moderation.

Comment writes are rate-limited per user (20/min) as a light anti-abuse measure.

## Roles & permissions (RBAC)

Five lab-oriented roles with a **code-defined permission matrix** (no DB-editable
permissions):

| Permission \ Role | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|:--:|:--:|:--:|:--:|:--:|
| `user:manage`     | ✅ | ✅ | - | - | - |
| `lab:configure`   | ✅ | ✅ | - | - | - |
| `subject:write`   | ✅ | ✅ | ✅ | - | - |
| `apparatus:write` (environments & scenarios) | ✅ | ✅ | ✅ | - | - |
| `test:write`      | ✅ | ✅ | ✅ | - | - |
| `test:run`        | ✅ | ✅ | ✅ | ✅ | - |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

Middleware `requirePermission("test:run")` is the primary gate.

## What stays out of Mişko

Per-frame telemetry and event rows live in the CV service's PostgreSQL. Video and
trajectory files live in object storage - Mişko stores only URLs. There is no
timeseries table in Mişko. See [Integration](../integration/).
