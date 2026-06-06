---
title: Roadmap
description: Step-by-step architecture for the video-only behavioral testing platform.
---

Mişko starts as a **video-only behavioral test platform**. There are no sensors
in the core roadmap. All measurements are derived from camera frames,
calibration, apparatus geometry and code-backed paradigm specs.

```txt
Camera frame
  -> detection or segmentation
  -> tracking
  -> pixel-to-cm calibration
  -> trajectory in apparatus coordinates
  -> zone and event metrics
  -> quality-control metrics
  -> validated summary result in Mişko
```

## Step 0 - Identity ✅

- Superadmin bootstrap at Docker startup.
- Public sign-up removed.
- Internal user management.

## Step 1 - Lab foundation ✅

- `Laboratory` singleton: one lab per installation. ✅
- Installation wizard creates the lab and `SUPERADMIN` together (CLI, Docker startup). ✅
- A second laboratory is refused (singleton guard). ✅
- Five roles: `SUPERADMIN`, `LAB_MANAGER`, `RESEARCHER`, `TECHNICIAN`, `VIEWER`. ✅
- Code-defined permission matrix with `requirePermission(...)`. ✅
- `Environment` instances: named, persisted test setups created from a paradigm template (many per paradigm). ✅

Done: `Laboratory` singleton and `Environment` models + migrations; the installation wizard (`backend/prisma/bootstrap-admin.js`) creates the lab and superadmin together. Lab API: `GET/PATCH /api/lab` (`lab:configure`). Paradigms are read-only, code-owned templates served by `GET /api/paradigms` and `GET /api/paradigms/:key`. An environment is a named instance of a paradigm whose physical values are resolved into a self-contained snapshot (`{ paradigmKey, schemaVersion, apparatus, zones }`); the values are validated against the code-fixed parameter ranges and stay locked at test time. Environment API: `GET /api/environments`, `GET /api/environments/:id`, `POST /api/environments`, `PATCH /api/environments/:id`, `DELETE /api/environments/:id` (writes need `apparatus:write`). A lab can hold several environments per paradigm (e.g. two distinct Morris water tanks). The frontend exposes an Environments menu; the Paradigms page is a read-only catalog, and each paradigm detail page can spawn a new environment.

## Step 2 - Scientific contract

- Code-backed `ParadigmSpec` registry (11 paradigms): `MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`, `Y_MAZE`, `NOVEL_OBJECT`, `BARNES_MAZE`, `THREE_CHAMBER`, `LIGHT_DARK`, `POLE`, `TREADMILL`. ✅
- Code-backed metric dictionary from [Measurement architecture](../measurements/). ✅
- Canonical units: results use `cm`, `cm_s`, `s`, `count`, `ratio`, `percent`, `deg`, `rpm`, `g`, `boolean`; apparatus parameters add `mm` and `c`. ✅
- Per-paradigm parameters, zones, metrics, suggested acceptance templates, QC requirements and artifact expectations. ✅
- Optional, per-test, user-defined acceptance criteria (engine `backend/src/config/acceptance.js`, builder `frontend/src/components/AcceptanceEditor.vue`). ✅
- Result schema versioning with `schemaVersion`. ✅
- Read-only inspection API: `GET /api/paradigms`, `GET /api/paradigms/:key`, `GET /api/paradigms/metrics`, `GET /api/paradigms/units`. ✅

Done: the registries live in `backend/src/config/{units,metrics,paradigms}.js` with `isKnownMetricKey(...)` ready to reject unknown result keys at Step 4. Pending: surfacing paradigm detail pages in the frontend and wiring rejection into result submission.

Exit criteria: every metric has a unit, definition, input list and aggregation behavior.

## Step 3 - Scenario: the central experiment definition ✅

A `Scenario` is the complete, reusable definition of one experiment, so running a
test is just "pick a subject and go". The domain stays small: **Subject** (the
mouse, kept simple), **Paradigm** (read-only catalog), **Environment** (a named
paradigm instance - the physical setup), **Scenario** (the central object), and
**Test** (one run).

A scenario carries every detail:

- references an `Environment` (and therefore a paradigm).
- selects which metrics are collected, from that paradigm's metric dictionary.
- defines the expected results / acceptance criteria per metric (reusing the
  Step 2 acceptance engine).
- session parameters from the paradigm.

A `Test` is then just `Subject + Scenario`; running it collects the scenario's
metrics and evaluates them against its expected results.

- Redesign the starter `Scenario` (`POOL | MAZE | STICK | PATH`) into this
  experiment definition (environment + metrics + expected results + session params).
- Move acceptance/expected results from `Test` onto `Scenario`.
- Keep `Test = Subject + Scenario`; `Test.result` becomes structured JSON.
- `Subject` stays the simple starter model.

Dropped from the earlier plan: rich `Subject`, `WeightLog`,
`DiseaseModel`/`Treatment`, `Study -> Group`, a separate `Apparatus` model. The
physical rig is the `Environment`; calibration stays in Step 5.

## Step 4 - Video-only boundary with fake CV

- `POST /api/tests/:id/result` with `X-Service-Key`.
- Idempotency through `captureSessionId`.
- Result validation against the active paradigm spec and metric dictionary.
- `cv-service/` skeleton with its own PostgreSQL and `/health`.
- Stub result push with fake but valid metrics.
- MinIO for videos and artifacts.

## Step 5 - Geometry and calibration

- Apparatus geometry editor for circle, rectangle, plus and custom polygon.
- Zone editor for platform, center, periphery, quadrants, wall annulus and arms.
- Reference-frame calibration with pixel-to-cm mapping and reprojection error.
- Fixed apparatus calibration and per-test override.
- MWM normalization with tank-centered coordinates, raw cm values and normalized distances.

## Step 6 - Real video CV MVP

- Camera adapters: `local_usb`, uploaded video file, and later phone stream.
- Detection or segmentation model for mouse localization.
- ByteTrack or equivalent tracker.
- OpenCV preprocessing and homography.
- Per-frame telemetry stays in the CV service.
- Summary metrics are pushed to Mişko.

| Paradigm | MVP metrics |
|---|---|
| MWM | Escape latency, path length, swim speed, quadrant time, thigmotaxis, platform crossings for probe trials. |
| Open Field | Distance, mean speed, center time, periphery time, immobility. |
| EPM | Open arm time, closed arm time, open arm entries, closed arm entries. |
| Rotarod | Trial duration and fall candidate events, with manual review at first. |

## Step 7 - Quality control and review

- Tracking confidence, dropped frame ratio, calibration error, occlusion ratio, out-of-bounds ratio, lighting warning and contrast warning.
- QC statuses: `PASS`, `WARN`, `REVIEW_REQUIRED`, `FAIL`.
- QC is separate from behavioral `passed`.
- Manual review screen with video, overlay, trajectory and metric summary.
- Study exports filter low-quality runs by default.

## Step 8 - Analysis and reporting

- Scenario dashboards aggregating their tests; compare subject groups (via `Subject.groupName`).
- Per-subject history across tests.
- MWM acquisition curves and probe summaries.
- Open Field and EPM summaries.
- Rotarod repeated-trial curves.
- CSV and JSON exports with metric definitions and QC status.

## Step 9 - Advanced behavior modules

- Optional pose-estimation adapter: DeepLabCut, SLEAP or another open source model.
- Behavior classifiers for rearing, grooming, freezing, risk assessment and head direction.
- Improved Rotarod fall detection.
- Multi-animal support after single-animal workflows are stable.

## Step 10 - Live monitoring and sources

- CV to Vue live panel through WebSocket or SSE.
- RTSP and HTTP adapters.
- Phone `getUserMedia` push adapter.
- Live QC warnings.

## Step 11 - Open source deployment

- Final license strategy for Mişko and CV service.
- Docker Compose profile for the full local stack.
- GHCR image publishing.
- GitHub Pages docs deploy.
- Example datasets, demo videos and sample apparatus definitions.

## Non-goals

- No sensor fusion in the core architecture.
- No RFID, accelerometer, load cell or IR beam dependency.
- No raw frame telemetry in Mişko PostgreSQL.
- No editable DB rows for scientific paradigm definitions.
- No cross-lab multi-tenant deployment in the first architecture.
