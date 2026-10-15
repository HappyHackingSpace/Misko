---
title: Domain model
description: The study model Mişko owns as the system of record, from the catalog and study design to test sessions, reviewed results and the report.
---

Mişko is the system of record for preclinical behavioral studies on rodents. A
study answers one question: does substance X, at dose D, reverse the deficit
caused by disease model M? The model follows from that question. Metric
definitions and quality-control rules are in
[Measurement architecture](../measurements/). The full design, with the
reasoning and the decisions, is in `docs/plans/DOMAIN_REDESIGN.md`.

```txt
Catalog:    Substance, Disease model, Paradigm (code), Environment (arena)

Design:     Experiment -> Groups (role + disease model) -> Regimen items
                       -> Timepoints, Protocols, Endpoints

Execution:  Subject -> Allocation (one group, ever) -> Administrations,
                                                       Induction outcome
            Test (session: timepoint x protocol step, one video)
              -> Test subjects (animal + arena) -> Trials
                                                -> Calibration (arena or override, with zones)
                                                -> Scoring sheets (observer-scored paradigms)
                                                -> Analysis runs (AUTOMATED or SCORED)
                                                   -> Reviewed result

Report:     accepted results per animal -> groups -> role-based comparisons
```

## Tenancy (single-tenant, on-prem)

Mişko runs **one laboratory per installation**, a singleton, not multi-tenant.

- **`Laboratory`** (singleton): name, code, time zone. Day 0, dosing days and
  default test times use the laboratory's time zone.
- **Setup (CLI):** `bootstrap setup` takes the lab name and the administrator
  email and creates the laboratory and the `SUPERADMIN` together. Running it
  again creates nothing.

## Catalog

Laboratory-wide reference data reused across studies.

- **Substance:** anything given to an animal: an active compound, a reference
  drug, a vehicle (saline, 0.5% CMC) or an agent that induces a disease model
  (MPTP, LPS, scopolamine). `kind` (`COMPOUND`, `REFERENCE_DRUG`, `VEHICLE`,
  `INDUCER`) only guides pickers; the role of a substance in a study comes from
  where it is used.
- **Disease model:** how a deficit is produced (`CHEMICAL`, `SURGICAL`, `DIET`,
  `STRESS`, `OTHER`) and, as free text, how it is confirmed. Every model is
  induced.
- **Paradigm:** a scientific test type **defined in code**, not an editable
  database row: parameters, zones, scored events and metric definitions, in
  published versions that never change. Registry: `MWM`, `OPEN_FIELD`, `EPM`,
  `ROTAROD`, `Y_MAZE`, `NOVEL_OBJECT`, `BARNES_MAZE`, `THREE_CHAMBER`,
  `LIGHT_DARK`, `POLE`, `TREADMILL`.
- **Environment:** one physical apparatus of one paradigm, such as a particular
  water tank or one of three open field boxes under one camera. Its
  measurements are immutable numbered revisions. In a test, an animal is placed
  in an **arena**, which is one environment revision.

## Subject

The animal: code, species (`MOUSE`, `RAT`) and sex. A subject can be registered
before it joins a study. Body weights belong to the subject, are recorded with
the time of measurement and never change.

## Study design

Everything here is decided before any animal is touched and is frozen when the
study starts.

- **Experiment:** code, title, description, hypothesis, randomisation method and
  whether it is blinded. Lifecycle `DRAFT -> RUNNING -> COMPLETED`. Starting
  sets **day 0**; completing makes the experiment read-only.
- **Group:** an arm of the study, defined by its role and its disease model:

  | Role | Disease model | Receives | Purpose |
  |---|---|---|---|
  | `HEALTHY_CONTROL` | none | vehicle, if any | normal baseline |
  | `SHAM` | yes | a vehicle in place of the induction | controls for the induction procedure |
  | `DISEASE_CONTROL` | yes | induction and vehicle | the deficit |
  | `POSITIVE_CONTROL` | yes | induction and a reference drug | proves the test can detect a rescue |
  | `TREATMENT` | yes | induction and the tested substance | efficacy and dose response |

  A group has a target size (n).
- **Regimen item:** what a group receives: purpose (`INDUCTION`, `VEHICLE`,
  `TREATMENT`), substance, dose, unit, route, frequency, and the first and last
  study day. A group has at most one treatment item, so a treatment group has one
  substance at one dose level; treatment groups of the same substance form the
  dose response.
- **Timepoint:** when animals are tested, as a study day relative to day 0
  (Baseline on day -1, Day 14 on day 14).
- **Protocol:** the test battery, with immutable versions of ordered steps. A
  step is a paradigm version, a trial type, a number of trials and session
  settings. It names no apparatus.
- **Endpoint:** what the study is judged on: one metric of one paradigm at one
  timepoint, primary or secondary, with the direction that counts as better and
  an optional baseline timepoint. A study has one primary endpoint.

A study starts only when its design is complete: timepoints, groups with target
sizes, a primary endpoint, a regimen that fits each role, and a disease control
for every disease model that sham, treatment or positive control groups use.

## Execution

- **Allocation:** an animal belongs to **exactly one group, so to one study,
  ever**. Animals can be allocated by hand or randomised over the groups up to
  their target sizes. In a draft an allocation can move or be removed; once the
  study runs, the group is fixed.
- **Exclusion:** an animal that leaves the study (`DIED`, `HUMANE_ENDPOINT`,
  `INDUCTION_FAILED`, `TECHNICAL`, `OTHER`) is excluded with a reason, never
  deleted. It stays visible in the report and is not tested or analyzed again.
- **Administration:** every dose actually given, including the disease
  induction and the vehicle, optionally linked to the regimen item it fulfils.
  A per-kilogram dose records the weight it was computed from. The daily dosing
  schedule is derived from the regimen items and day 0.
- **Induction outcome:** whether the animal's disease model was confirmed,
  recorded once.

## Test (one session)

A `Test` is one session of a running experiment: one timepoint, one protocol
step, **one video**. It films one or more animals, the **test subjects**, each in
its own arena; usually one, sometimes up to three arenas under one camera.

- Status: `PLANNED -> IN_PROGRESS -> COMPLETED`, or `CANCELLED` with a reason.
- Animals join or leave only while the test is planned. An animal is tested at
  most once per timepoint and protocol step, and every arena must run the step's
  paradigm.
- Trials are recorded per animal and are append-only; a repeated trial gets the
  next attempt number.
- Animals of different groups may share a session. A measurement's group always
  comes from the animal's allocation; nothing about the group is copied onto the
  test.

## Video, calibration and analysis

- **Video:** a test has one current verified video. A replacement upload
  supersedes it; earlier videos stay for provenance.
- **Calibration:** maps pixels to centimeters. By default each arena
  (environment revision) is calibrated once when the rig is set up, with its
  crop as the arena's region of the camera frame. When the camera or arena moved,
  a manual **override** applies to one test subject on the current video.
  Paradigms whose zones are not derived from parameters (elevated plus maze,
  Y maze, Barnes maze, three-chamber) carry their **zones** with the
  calibration, drawn on the frame in pixels and stored in centimetres too; a
  calibration and its zones never change. Observer-scored paradigms need no
  calibration.
- **Scoring sheet:** for an observer-scored paradigm (`ROTAROD`, `POLE`,
  `TREADMILL`, `NOVEL_OBJECT`), the events a person marks for one test subject
  on the current video, in microseconds from the clip start. A sheet is a
  `DRAFT` while it is scored and `COMPLETED` once its scored event types and
  recording length are fixed; reopening starts the next version and completed
  sheets never change.
- **Analysis run:** one per test subject. Its `method` is `AUTOMATED` when a
  worker tracks the animal inside its arena's crop and uploads the trajectory
  (`OPEN_FIELD`, `EPM`, `Y_MAZE`, `BARNES_MAZE`, `THREE_CHAMBER`, `MWM`,
  `LIGHT_DARK`), or `SCORED` when it is computed from a completed scoring sheet,
  with the scorer and no worker, calibration or video pair. Either way Mişko
  computes every metric and event itself. Inputs are pinned, so reanalysis or
  rescoring is a new run and old runs stay.
- **Reviewed result:** every result is approved by a person. The first
  successful run of an animal opens a `PENDING` result; a reviewer accepts it or
  rejects it with a reason. In a blinded study the reviewer does not see the
  group, disease, substance or dose. Only accepted results enter statistics.

A behavioral result is **data, not a verdict**: there is no pass or fail on a
test.

## Report

The experimental unit is the animal. For each endpoint the report takes the
accepted results of animals that are not excluded and gives, per group, n, mean,
SD, SEM and change from baseline, and the comparisons that follow from the group
roles:

- model validity: disease control against healthy control and sham;
- assay sensitivity: positive control against disease control;
- efficacy: each treatment group against its disease control, with rescue %;
- dose response: the treatment groups of one substance ordered by dose.

Results of different paradigm or metric engine versions are never pooled.

## Comment (test discussion)

A `Comment` is a free-form note attached to a `Test`, forming a discussion
thread.

- `testId`, `authorId`, `body` (plain text), `createdAt`, `updatedAt`.
- `body` is stored verbatim and **never** interpreted as HTML; it is bounded
  (max 2000 chars), trimmed, and stripped of control characters at the API layer.

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
| `study:write` (catalog, design, allocation, protocols) | ✅ | ✅ | ✅ | - | - |
| `subject:write`   | ✅ | ✅ | ✅ | - | - |
| `weight:write`    | ✅ | ✅ | ✅ | ✅ | - |
| `apparatus:write` (environments, arena calibration) | ✅ | ✅ | ✅ | - | - |
| `test:write` (planning tests) | ✅ | ✅ | ✅ | - | - |
| `test:run` (running tests, videos, doses, review) | ✅ | ✅ | ✅ | ✅ | - |
| `*:read`          | ✅ | ✅ | ✅ | ✅ | ✅ |

Every use case checks the permission itself (`actor.Require(...)`) before it
touches a store.

## What stays out of the database

Video files, analyzed videos and trajectories live in private object storage;
the database stores bucket, object name and generation, never video bytes or
signed URLs. Paradigm definitions are code, not rows.
