# Mişko Roadmap

> Philosophy: build the scientific contract and the video-only backbone before
> adding real AI. Mişko and the CV service stay independent from the start. Their
> contact point is the contract in `docs/INTEGRATION.md`. The domain model is in
> `docs/DOMAIN.md`; measurement rules are in `docs/MEASUREMENTS.md`.

## Architecture north star

Mişko starts as a **video-only behavioral test platform**. There are no sensors
in the core roadmap: no RFID, no accelerometer, no load cell, no IR beam. All
measurements are derived from camera frames, calibration, apparatus geometry and
the code-backed paradigm specs.

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

The core architecture is intentionally split:

| Layer | Responsibility |
|---|---|
| Mişko | Lab workflow, users, subjects, environments, scenarios, paradigm specs, metric dictionary, summary results. |
| CV service | Video capture, frame processing, detection, tracking, telemetry, artifacts, metric computation. |
| Object storage | Video, trajectory files, heatmaps, calibration images and debug overlays. |

## Step 0 - Identity and internal SaaS model ✅

Mişko is an internal, org-only, on-prem product. No public sign-up.

- Superadmin bootstrap at Docker startup with `ADMIN_EMAIL`.
- System-generated strong password printed once.
- Sign-up removed from frontend and backend.
- Internal user management through `/api/users`.

## Step 1 - Lab foundation and permissions ✅

Goal: establish the operational shell before science data grows.

- `Laboratory` singleton: one lab per installation. (done, `Laboratory` model)
- Installation wizard creates the laboratory and `SUPERADMIN` together. (done, CLI in `backend/prisma/bootstrap-admin.js` at Docker startup)
- Singleton guard refuses a second laboratory. (done, bootstrap and `lab.service.js` never create a second lab)
- Five roles: `SUPERADMIN`, `LAB_MANAGER`, `RESEARCHER`, `TECHNICIAN`, `VIEWER`. (done)
- Code-defined permission matrix with `requirePermission(...)`. (done)
- Migrate existing `ADMIN` and `OPERATOR` users to the new roles. (done)
- `Environment` instances: named, persisted test setups created from a paradigm template. (done)
  - `GET/PATCH /api/lab` (lab:configure) reads/updates the singleton.
  - Paradigms are read-only, code-owned templates: `GET /api/paradigms` and
    `GET /api/paradigms/:key` carry no per-lab state.
  - An environment stores a self-contained snapshot (`{ paradigmKey, schemaVersion,
    apparatus, zones }`); apparatus values are validated against the code-fixed
    parameter ranges and stay locked at test time.
  - Environment CRUD: `GET /api/environments`, `GET /api/environments/:id`,
    `POST /api/environments`, `PATCH /api/environments/:id`, `DELETE /api/environments/:id`
    (writes need `apparatus:write`).
  - A lab can hold several environments per paradigm (e.g. two distinct Morris water tanks).

Exit criteria:

- A fresh install creates one lab and one superadmin. (done)
- Every route is gated by permissions, not ad-hoc role checks. (done)
- Paradigms are a read-only catalog; physical setups live as named environments. (done; the frontend has an Environments menu and each paradigm detail page can spawn one)

## Step 2 - Scientific contract before CV

Goal: define what the system means by each paradigm and each measurement before
real AI starts producing data.

- Add code-backed `ParadigmSpec` registry: (done, 11 paradigms in `backend/src/config/paradigms.js`)
  - `MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`
  - `Y_MAZE`, `NOVEL_OBJECT`, `BARNES_MAZE` (learning and memory)
  - `THREE_CHAMBER` (social)
  - `LIGHT_DARK` (anxiety)
  - `POLE`, `TREADMILL` (motor)
- Add code-backed `MetricDefinition` registry from `docs/MEASUREMENTS.md`. (done, `backend/src/config/metrics.js`)
- Lock canonical units: (done, `backend/src/config/units.js`)
  - position and distance: `cm`
  - speed: `cm_s`
  - duration: `s`
  - ratios: `ratio` (0..1) and `percent`
  - counts: `count`
  - angle: `deg`
  - rotarod speed: `rpm`
  - subject weight: `g`
  - event flags: `boolean`
  - apparatus parameters only: `mm`, `c`
- Define per-paradigm: (done)
  - apparatus parameters
  - session parameters
  - zones
  - metrics (each with `valueType`, `validRange`, `templated`)
  - QC requirements
  - artifact expectations
- Per-metric validation contract (`validateMetrics`): the single contract both
  manual entry and the CV service validate result values against. (done)
- Define result schema versioning with `schemaVersion`. (done, `RESULT_SCHEMA_VERSION`)
- Expose a read-only inspection API for researchers and the CV service. (done)
  - `GET /api/paradigms`, `GET /api/paradigms/:key`
  - `GET /api/paradigms/metrics`, `GET /api/paradigms/units`

> Note: there is **no pass/fail acceptance**. A behavioral result is data, not a
> verdict; interpretation/comparison is the analysis layer's job.

Exit criteria:

- Unknown metric keys are not accepted. (`validateMetrics(...)` enforced on result entry)
- Every metric has a unit, definition, input list and aggregation behavior. (done)
- Researchers can inspect paradigm detail pages before running a test. (done; `frontend/src/views/Paradigms.vue`)

## Step 3 - Scenario: the central experiment definition

Goal: make a `Scenario` the complete, reusable definition of an experiment, so
that running a test is just "pick a subject and go".

The domain is intentionally small:

- **Subject** - the mouse (kept simple: code, sex, group, birth date, notes).
- **Paradigm** - read-only code catalog (Step 2).
- **Environment** - a named instance of a paradigm: the physical setup snapshot
  (apparatus + zones). (Step 1)
- **Scenario** - the central object: a named bundle of one or more environments.
- **Test** - a `Subject` measured against a `Scenario`, run environment by
  environment.

A `Scenario` references one or more `Environment`s (N-N), so it can span one or
more paradigms; each environment's paradigm fixes its metrics. A scenario carries
**no** pass/fail criteria. A `Test` = `Subject + Scenario`, run environment by
environment (Start -> record metric results -> Finish). Results are stored per
environment and are **data, not a verdict**.

Separate the three layers: **signals** (raw, real-time, CV-side - never in
Mişko), **metrics** (paradigm definitions), **metric results** (the values for a
run; CV computes from signals, manual entry records the observable ones).

Work items:

- Redesign the starter `Scenario` (free `POOL | MAZE | STICK | PATH` type) into a
  named bundle of N-N `environments` (+ optional `sessionParams`). (done)
- `Test = Subject + Scenario`; `Test.result` is structured per-environment JSON:
  `{ environments: { [envId]: { status, startedAt, endedAt, metrics } } }`.
  **No `passed`/verdict.** (done)
- Per-environment manual result entry: a form auto-generated from each
  environment's paradigm metric dictionary, validated by `validateMetrics`. (done)
- Keep `Subject` as the simple starter model. (done)

Exit criteria:

- A scenario fully defines an experiment (its environments); built once.
- Running a test = pick a subject + scenario, then enter each environment's
  metric results (manual now, CV later - same shape).

Explicitly out of scope (dropped from the earlier research-grade plan): rich
`Subject`, `WeightLog`, `DiseaseModel`/`Treatment`, `Study -> Group`, a separate
`Apparatus` model. The physical rig is the `Environment`; calibration stays in
Step 5.

## Step 4 - Video-only boundary with fake CV

Goal: prove the end-to-end contract without real vision.

- Add `POST /api/tests/:id/result`.
- Authenticate service-to-service calls with `X-Service-Key`.
- Make result submission idempotent through `captureSessionId`.
- Validate result JSON against the active `ParadigmSpec` and metric dictionary.
- Add `cv-service/` skeleton with FastAPI, its own PostgreSQL and `/health`.
- Stub CV result push with fake but valid metrics.
- Add MinIO for video and artifact URLs.

Exit criteria:

- Operator creates and finishes a test in Mişko.
- Fake CV pushes a valid result.
- Mişko stores per-environment metrics, QC status and artifact URLs (no verdict).
- Duplicate `captureSessionId` submissions are no-ops.

## Step 5 - Apparatus geometry and calibration workflow

Goal: make every lab's physical setup analyzable, especially different MWM
tanks.

- Apparatus geometry editor:
  - circle, rectangle, plus, custom polygon
  - dimensions in cm
  - surface color and material
  - paradigm-specific config
- Zone editor:
  - platform
  - center
  - periphery
  - quadrants
  - wall annulus
  - open and closed arms
- Calibration workflow:
  - upload or capture reference frame
  - mark reference points
  - compute pixel-to-cm transform
  - store reprojection error
  - allow fixed apparatus default and per-test override
- MWM normalization:
  - tank-centered coordinates
  - raw cm values
  - normalized coordinates and distances
  - protocol version compatibility checks

Exit criteria:

- A user can define a Morris tank with its real diameter and platform location.
- CV can receive geometry and calibration without guessing zones from video.
- Cross-lab MWM results can be compared through normalized metrics.

## Step 6 - Real video CV MVP

Goal: compute the first real measurements from camera video only.

- Camera adapters:
  - `local_usb`
  - uploaded video file for offline tests
  - phone camera stream later in this step if needed
- Detection or segmentation model for mouse localization.
- ByteTrack or equivalent tracker for trajectory continuity.
- OpenCV pipeline for preprocessing and homography.
- Per-frame telemetry stored in CV service, not in Mişko.
- Summary metrics pushed to Mişko at test end.

Initial metric targets:

| Paradigm | MVP metrics |
|---|---|
| MWM | Escape latency, path length, swim speed, quadrant time, thigmotaxis, platform crossings for probe trials. |
| Open Field | Distance, mean speed, center time, periphery time, immobility. |
| EPM | Open arm time, closed arm time, open arm entries, closed arm entries. |
| Rotarod | Trial duration and fall candidate events, with manual review at first. |

Exit criteria:

- CV processes a real video and returns valid metrics.
- Debug overlay shows detected animal, trajectory, zones and calibration.
- MWM, Open Field and EPM can run with centroid tracking.
- Rotarod is marked as experimental until fall detection is validated.

## Step 7 - Quality control and review

Goal: make results scientifically usable, not just numerically populated.

- Compute QC metrics:
  - tracking confidence
  - dropped frame ratio
  - calibration error
  - occlusion ratio
  - out-of-bounds ratio
  - lighting warning
  - contrast warning
- Add QC statuses:
  - `PASS`
  - `WARN`
  - `REVIEW_REQUIRED`
  - `FAIL`
- QC qualifies the metric data; it is not a pass/fail verdict.
- Add manual review screen with video, overlay, trajectory and metric summary.
- Default scenario exports exclude `REVIEW_REQUIRED` and `FAIL` unless explicitly
  included.

Exit criteria:

- Bad tracking is visible and does not silently enter scenario summaries.
- Reviewers can inspect artifacts and decide whether a test is usable.
- Every exported result carries QC status and protocol version.

## Step 8 - Analysis and reporting

Goal: turn individual tests into scenario-level evidence.

- Scenario dashboards: aggregate the tests run under a scenario.
- Compare subject groups (via `Subject.groupName`) within a scenario.
- Per-subject history across tests.
- MWM acquisition curves.
- Probe trial summaries.
- Open Field center/periphery summaries.
- EPM open-arm summaries.
- Rotarod repeated-trial learning curves.
- Export CSV and JSON with metric definitions and QC status.

Exit criteria:

- Researchers can compare subject groups within a scenario.
- Reports include units, normalization, protocol version and QC filtering.
- Raw video stays outside Mişko, but artifacts are linked.

## Step 9 - Advanced behavior modules

Goal: add pose or behavior-specific models only where centroid tracking is not
enough.

- Optional pose-estimation adapter:
  - DeepLabCut
  - SLEAP
  - another open source model if licensing and performance fit better
- Behavior classifiers:
  - rearing
  - grooming
  - freezing
  - risk assessment
  - head direction
- Improve Rotarod fall detection.
- Add multi-animal support only after single-animal workflows are stable.

Exit criteria:

- Advanced behaviors are versioned modules, not hidden changes to core metrics.
- Paradigms declare whether they need centroid, bounding box, segmentation or
  pose.

## Step 10 - Live monitoring and additional video sources

Goal: improve operations without changing the scientific contract.

- CV to Vue live panel through WebSocket or SSE.
- RTSP and HTTP camera adapters.
- Phone `getUserMedia` push adapter.
- Live QC warnings.
- Session start and stop controls.

Exit criteria:

- Operators can monitor a running test.
- Live display is operational only; final results still come through the
  validated result contract.

## Step 11 - Open source deployment

Goal: make the project installable and reproducible by other labs.

- Decide final license strategy for Mişko and CV service.
- Docker Compose profile for full local stack.
- GHCR image publishing.
- GitHub Pages docs deploy.
- Example datasets and demo videos.
- Reproducible seed data and sample scenario and environment definitions.

Exit criteria:

- A lab can install the stack from docs.
- Demo videos reproduce the documented metrics.
- Open source license and third-party model licenses are documented.

## Explicit non-goals for the core roadmap

- No sensor fusion in the core architecture.
- No RFID, accelerometer, load cell or IR beam dependency.
- No raw frame telemetry in Mişko PostgreSQL.
- No editable DB rows for scientific paradigm definitions.
- No cross-lab multi-tenant deployment in the first architecture.

Sensor adapters can be added later as optional CV-side modules, but the product
must be scientifically useful with camera input alone.
