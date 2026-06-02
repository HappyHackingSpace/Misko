---
title: Domain model
description: The research-grade data model Mişko owns as the system of record.
---

This is the target model that turns the thin starter schema into one that
supports real behavioral-neuroscience workflows. Mişko owns this model as the
**system of record**; the CV service stays independent.

Metric definitions, MWM normalization and quality-control semantics are detailed
in [Measurement architecture](../measurements/).

## Tenancy (single-tenant, on-prem)

Mişko runs **one laboratory per installation** — a singleton, not multi-tenant.

- **`Laboratory`** (singleton): name, code, timezone, settings.
- **Installation wizard (CLI, at startup):** takes the lab name + superadmin
  email and creates the `Laboratory` **and** the `SUPERADMIN` together. A
  singleton guard refuses a second lab; the step is idempotent.
- **`LabParadigm`:** the paradigm catalog is global, but each lab enables a
  **subset** (e.g. "this lab only has the pool"). Toggled by admins.

## Paradigm ≠ Apparatus ≠ Calibration

The single biggest design decision — three concepts the old `Scenario`
conflated, split apart:

| Concept | Answers | Varies per | Owner |
|---|---|---|---|
| **Paradigm** | *what* is tested & which metrics matter | fixed catalog | Mişko (code) |
| **Apparatus** | *which physical rig* (geometry, color, size) | per lab/setup | Mişko |
| **Calibration** | *how pixels map to cm + zones* | fixed rig or per session | Mişko / CV |

The camera does **not** infer the paradigm or zones. CV tracks the animal; the
geometry (zones in cm) and the pixel↔cm mapping are *provided* to it.

### Paradigm — a hardcoded, SOLID registry

A paradigm is a scientific test type **defined in code**, not an editable DB row.
The spec (which parameters, zones, metrics exist) is engineering-owned and
stable; only the **values** are user data. Each paradigm implements a common
interface; adding one means adding a class and touching nothing else.

```ts
interface ParadigmSpec {
  key: string;            // 'MWM' | 'OPEN_FIELD' | 'EPM' | 'ROTAROD'
  name: string;
  category: string;       // learning_memory | anxiety | motor | social
  parameters: Field[];    // → auto-rendered UI form + validation
  zones(config): Zone[];  // concrete zones from the configured geometry
  metrics: MetricDef[];   // what the CV service must compute
  acceptance(config): Rule | null;
  qc: QualityRequirement[];
  validate(config): void;
}
```

Initial registry: `MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`.

The result JSON must validate against the active paradigm's metric dictionary,
result schema, QC requirements and protocol version.

### Apparatus (the physical "environment")

The same paradigm runs on different rigs — a white tank vs a black tank,
different diameter, different platform position. Because CV accuracy depends on
**animal-vs-background contrast**, appearance is first-class: `surfaceColor`,
`material`, `shape`, `dimensions` (cm), paradigm-specific config, and concrete
`zones` in cm.

### Calibration (camera ↔ world)

Supports both a permanently mounted camera (calibrate once, store on the
apparatus) and a movable camera (calibrate per session, override on the Test).
Resolution order: **Test override → Apparatus default**.

## Subject (the mouse) — rich domain

- **Identity:** code, microchip ID, ear tag.
- **Biological:** species, strain, line/genotype, zygosity, sex, birth date,
  **coat color** (vision contrast).
- **Physiology:** `WeightLog` time series (many protocols weigh daily), health
  status.
- **Housing & lifecycle:** cage, litter/cohort, status (`ALIVE | SACRIFICED |
  DEAD`).
- **Experimental:** N–N disease models and N–N treatments (with dose, route,
  schedule).

## Disease models & treatments

A mouse can carry **multiple** disease models and treatments at once. Both are
catalogs joined N–N to the subject, capturing induction method, dose, route and
schedule.

## Study → Group (longitudinal)

Real labs organize around a Study with comparison groups (control, model,
treated). A `Test` references a study and carries a `timepoint` label so repeated
runs of the same subject are comparable — the basis for later statistics.

## Roles & permissions (RBAC)

Five lab-oriented roles with a **code-defined permission matrix** (no
DB-editable permissions):

| Permission \ Role | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|:--:|:--:|:--:|:--:|:--:|
| `user:manage`     | ✅ | ✅ | — | — | — |
| `lab:configure`   | ✅ | ✅ | — | — | — |
| `paradigm:toggle` | ✅ | ✅ | — | — | — |
| `study:write`     | ✅ | ✅ | ✅ | — | — |
| `subject:write`   | ✅ | ✅ | ✅ | — | — |
| `apparatus:write` | ✅ | ✅ | ✅ | — | — |
| `weight:write`    | ✅ | ✅ | ✅ | ✅ | — |
| `test:write`      | ✅ | ✅ | ✅ | — | — |
| `test:run`        | ✅ | ✅ | ✅ | ✅ | — |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

Middleware `requirePermission("test:run")` is the primary gate.

## What stays out of Mişko

Per-frame telemetry and event rows live in the CV service's PostgreSQL. Video
and trajectory files live in object storage — Mişko stores only URLs. There is
no timeseries table in Mişko. See [Integration](../integration/).
