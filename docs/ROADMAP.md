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
| Mişko | Lab workflow, users, studies, subjects, apparatuses, calibrations, paradigm specs, metric dictionary, summary results. |
| CV service | Video capture, frame processing, detection, tracking, telemetry, artifacts, metric computation. |
| Object storage | Video, trajectory files, heatmaps, calibration images and debug overlays. |

## Step 0 - Identity and internal SaaS model ✅

Mişko is an internal, org-only, on-prem product. No public sign-up.

- Superadmin bootstrap at Docker startup with `ADMIN_EMAIL`.
- System-generated strong password printed once.
- Sign-up removed from frontend and backend.
- Internal user management through `/api/users`.

## Step 1 - Lab foundation and permissions

Goal: establish the operational shell before science data grows.

- `Laboratory` singleton: one lab per installation.
- Installation wizard creates the laboratory and `SUPERADMIN` together.
- Singleton guard refuses a second laboratory.
- Five roles: `SUPERADMIN`, `LAB_MANAGER`, `RESEARCHER`, `TECHNICIAN`, `VIEWER`.
- Code-defined permission matrix with `requirePermission(...)`.
- Migrate existing `ADMIN` and `OPERATOR` users to the new roles.
- `LabParadigm` lets admins enable or disable paradigms for the lab.

Exit criteria:

- A fresh install creates one lab and one superadmin.
- Every route is gated by permissions, not ad-hoc role checks.
- UI only shows enabled paradigms.

## Step 2 - Scientific contract before CV

Goal: define what the system means by each paradigm and each measurement before
real AI starts producing data.

- Add code-backed `ParadigmSpec` registry:
  - `MWM`
  - `OPEN_FIELD`
  - `EPM`
  - `ROTAROD`
- Add code-backed `MetricDefinition` registry from `docs/MEASUREMENTS.md`.
- Lock canonical units:
  - position and distance: `cm`
  - speed: `cm_s`
  - duration: `s`
  - ratios: `0..1`
  - counts: `count`
- Define per-paradigm:
  - apparatus parameters
  - session parameters
  - zones
  - metrics
  - acceptance criteria
  - QC requirements
  - artifact expectations
- Define result schema versioning with `schemaVersion` and `protocolVersion`.

Exit criteria:

- Unknown metric keys are not accepted.
- Every metric has a unit, definition, input list and aggregation behavior.
- Researchers can inspect paradigm detail pages before running a test.

## Step 3 - Research-grade domain model

Goal: replace the starter schema with the model needed for real studies.

- Rich `Subject`: strain, line, genotype, zygosity, sex, birth date, coat color,
  cage, health status and lifecycle.
- `WeightLog` time series.
- `DiseaseModel` catalog with subject joins.
- `Treatment` catalog with subject joins, dose, route and schedule.
- `Study -> Group` for longitudinal experiments.
- `Apparatus` as the physical rig for a paradigm.
- `Calibration` as fixed apparatus default or per-session override.
- `Test` references paradigm, apparatus, subject, operator, device, study,
  timepoint and calibration.
- Change `Test.result` from stringified JSON to structured Prisma `Json`.

Exit criteria:

- A study can contain groups and repeated tests across timepoints.
- A test knows which apparatus and calibration were used.
- Results can store structured metrics, QC and artifact URLs.

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
- Mişko stores metrics, QC status, passed value and artifact URLs.
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
- Keep QC separate from behavioral `passed`.
- Add manual review screen with video, overlay, trajectory and metric summary.
- Default study exports exclude `REVIEW_REQUIRED` and `FAIL` unless explicitly
  included.

Exit criteria:

- Bad tracking is visible and does not silently enter study summaries.
- Reviewers can inspect artifacts and decide whether a test is usable.
- Every exported result carries QC status and protocol version.

## Step 8 - Analysis and reporting

Goal: turn individual tests into study-level evidence.

- Study dashboards by group and timepoint.
- Longitudinal subject view.
- MWM acquisition curves.
- Probe trial summaries.
- Open Field center/periphery summaries.
- EPM open-arm summaries.
- Rotarod repeated-trial learning curves.
- Export CSV and JSON with metric definitions and QC status.

Exit criteria:

- Researchers can compare groups within a study.
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
- Reproducible seed data and sample apparatus definitions.

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
