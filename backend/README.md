# Go backend

The Go rebuild is incomplete; do not release it.

The study model was rebuilt on a fresh schema (`schema/001`-`012`) following
[DOMAIN_REDESIGN.md](../docs/plans/DOMAIN_REDESIGN.md): an experiment is
designed as groups with a role, a disease model and a regimen, animals are
allocated to one group, and a test is one filmed session of one or more animals.
Phases, enrollments, dated group assignments, intervention plans, subject
conditions and per-recording calibration no longer exist.

| Issue | Implemented |
|---|---|
| [#124](https://github.com/HappyHackingSpace/Misko/issues/124) | Liveness/readiness, graceful shutdown, schema installation, architecture checks |
| [#125](https://github.com/HappyHackingSpace/Misko/issues/125) | Login, current user, own password change, user management, laboratory singleton, RBAC, idempotent setup |
| [#126](https://github.com/HappyHackingSpace/Misko/issues/126), [#127](https://github.com/HappyHackingSpace/Misko/issues/127) | Rebuilt as: catalog (substances, disease models), subjects and weights, experiments with groups, regimens, timepoints, endpoints and allocations, dosing and induction outcomes |
| [#128](https://github.com/HappyHackingSpace/Misko/issues/128) | Read-only versioned paradigm catalog and its first metric engine (since trimmed to the four paradigms below) |
| [#129](https://github.com/HappyHackingSpace/Misko/issues/129) | The remaining paradigm contracts and metric engines, calibrated zones and pinned manifests (observer scoring was later removed: every paradigm is tracked) |
| [#130](https://github.com/HappyHackingSpace/Misko/issues/130) | Environments with immutable measurement revisions and experiment test protocols with immutable versions |
| [#131](https://github.com/HappyHackingSpace/Misko/issues/131) | Tests as sessions with one or more animals in arenas, their lifecycle, trials per animal and test comments |
| [#132](https://github.com/HappyHackingSpace/Misko/issues/132) | Video assets in private Google Cloud Storage with one current verified video per test: signed resumable uploads, server-side verification, pinned read URLs (live GCS smoke test pending) |
| [#133](https://github.com/HappyHackingSpace/Misko/issues/133) | Calibration per arena (environment revision) with manual overrides per animal of a test: normalized DLT homography from FIT points, independent CHECK validation, correction chains and the waiting-for-calibration status |
| [#134](https://github.com/HappyHackingSpace/Misko/issues/134) | Analysis workers, analysis runs per animal with leases and fencing attempts, the scheduler process, verified output publication, the original/analyzed video pair and human review of every result |
| [#137](https://github.com/HappyHackingSpace/Misko/issues/137) | The study report per endpoint with role-based comparisons, CSV export and the dashboard summary |

The water maze worker in [`worker/`](../worker/README.md) is tested only on
synthetic video. The Vue panel is being moved to this API; its screens for the
study model are scaffolded but not complete. There is no legacy API adapter and
no data migration from earlier installations.

## Layout and boundaries

- `cmd/api`: process entry, signals, exit status.
- `cmd/bootstrap`: `migrate` installs the schema into an empty database or applies pending migrations; `setup` creates the laboratory and first administrator.
- `cmd/jobs`: the scheduler loop (lease expiry, automatic jobs of animals of tests and of paradigm trials); it serves no HTTP.
- `internal/bootstrap`: composition root; constructor wiring of concrete adapters.
- `internal/access/domain`: the shared RBAC kernel (roles, permission matrix, authenticated actor).
- `internal/identity`: users, passwords, tokens and user administration.
- `internal/laboratory`: the laboratory singleton.
- `internal/catalog`: laboratory-wide substances and disease models.
- `internal/subjects`: laboratory animals and their body weights.
- `internal/experiments`: experiments and their lifecycle, groups, regimen items, timepoints, endpoints and allocations (with exclusion).
- `internal/dosing`: actual administrations, the daily dosing schedule derived from regimens, and induction outcomes.
- `internal/paradigms`: the hardcoded, versioned paradigm catalog and pure metric engine.
- `internal/environments`: physical apparatus (arenas) and their immutable measurement revisions.
- `internal/protocols`: test protocols of an experiment with immutable versions of ordered paradigm steps.
- `internal/tests`: tests (sessions), the animals in them (test subjects) with their arenas, trials per animal and comments.
- `internal/media`: video assets and the videos of tests, and an asset port for videos outside tests (paradigm trials) and source videos (analysis); `adapters/gcs` is the only package that imports the Cloud Storage client.
- `internal/calibration`: pixel-to-centimeter calibrations owned by an environment revision (the default), a test subject (an override on its test's current video) or a paradigm trial (on its own video), and the status that gates analysis.
- `internal/analysis`: the analysis service. Jobs (an immutable input snapshot and its execution state), workers and capabilities, leases and attempts, result validation, publication of outputs, metrics, events and QC. It knows nothing about experiments or trials.
- `internal/subjectanalysis`: the experiment side of analysis: which job analyses which animal of a test (`test_subject_analyses`), the orchestrator that submits a job when an animal is ready, reanalysis and the review of results (`subject_results`).
- `internal/trials`: paradigm trials: a paradigm version tried on one video in one arena outside any experiment, with its own uploads, calibration and analyses.
- `internal/reports`: the read-only study report and its CSV export over the reporting views.
- `internal/dashboard`: read-only counts and short lists for the start screen.
- `internal/health`: readiness use case and probes.
- `internal/platform`: configuration, HTTP server lifecycle, JSON helpers, PostgreSQL transaction and error helpers, the in-memory rate limiter and the integration-test database helper. No business rules.
- `schema`: embedded, numbered SQL migrations recorded in `misko.schema_migrations`; never run by the API (see Migrations).
- `sqlc.yaml`, `internal/*/adapters/postgres/queries.sql`: SQL sources for the generated `sqlcgen` packages. Do not edit generated code; run `make generate`.
- `tests/architecture`: source import checks with positive and negative policy fixtures.

Every module (a bounded context) has its own domain, application and adapters
and its own tables. Domain packages allow pure standard-library dependencies
and their own subpackages. Application packages add context/synchronization and
their own domain. `internal/access/domain` is the only domain package other
modules may import, so every use case authorizes against the same matrix.
Adapters import only their own module, the platform and the RBAC kernel; HTTP
adapters resolve the caller through an `authenticate` function wired in the
composition root. Modules call each other only through small interfaces the
consumer declares in its application package (ports), implemented by adapters
in `internal/bootstrap/ports.go` and `analysis.go`: the analysis service's port
(`Submit`, `Get`, `Jobs`, `ByRequester`, `Results`), the media asset port, the
calibration port (`Snapshot`, `TrialCalibration`) and the environment revision
port. Relationships between modules, such as an allocation's subject, are
enforced with foreign keys rather than imports. The paradigm catalog
(`internal/paradigms/domain`) is a shared kernel read only by each module's
small `adapters/catalog` package. The architecture test also states that
`analysis` never imports the experiment side (`experiments`, `tests`,
`protocols`, `dosing`, `subjects`, `subjectanalysis`, `reports`, `dashboard`) or
`trials`, and that `trials` never imports the experiment side.
`net/http`, `database/sql`, JWT, bcrypt, pgx and GCP packages cannot enter inner
layers. The architecture test scans every Go file under `internal`, including
inactive build tags.

## Roles and permissions

The five roles and their permissions are static code, not database rows.

| Permission | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|---|---|---|---|---|
| `user:manage` | Yes | Yes | No | No | No |
| `lab:configure` | Yes | Yes | No | No | No |
| `study:write` | Yes | Yes | Yes | No | No |
| `subject:write` | Yes | Yes | Yes | No | No |
| `weight:write` | Yes | Yes | Yes | Yes | No |
| `apparatus:write` | Yes | Yes | Yes | No | No |
| `test:write` | Yes | Yes | Yes | No | No |
| `test:run` | Yes | Yes | Yes | Yes | No |
| `trial:run` | Yes | Yes | Yes | Yes | No |
| `*:read` | Yes | Yes | Yes | Yes | Yes |

What each permission covers:

| Permission | Covers |
|---|---|
| `study:write` | Catalog (substances, disease models), experiment design and lifecycle, allocation and exclusion, protocols |
| `apparatus:write` | Environments, their revisions and arena calibrations |
| `subject:write` | Subjects |
| `weight:write` | Body weights |
| `test:write` | Planning tests: creating them, adding or removing animals, cancelling |
| `test:run` | Running tests and trials, videos, per-animal calibration overrides, doses, induction outcomes, manual analysis of an animal and result review |
| `trial:run` | Paradigm trials: creating, uploading their video, calibrating on it, analysing and deleting |
| `*:read` | Reading everything |

Route overview (`{e}` is `/api/experiments/{id}`):

| Route | Required access |
|---|---|
| `GET /api/health`, `GET /api/ready`, `GET /api/meta`, `POST /api/auth/login` | Public |
| `GET /api/auth/me`, `POST /api/auth/password` | Any signed-in user |
| `GET, POST /api/users`; `GET, PATCH, DELETE /api/users/{id}`; `POST /api/users/{id}/reset-password` | `user:manage` |
| `GET /api/lab`; `GET /api/paradigms`, `/api/paradigms/{key}`, `/api/paradigms/{key}/versions/{version}` | `*:read` |
| `PATCH /api/lab` | `lab:configure` |
| `GET /api/catalog/substances`, `/api/catalog/disease-models` and `/{id}` of each | `*:read` |
| `POST`, `PATCH /{id}`, `DELETE /{id}` on `/api/catalog/substances` and `/api/catalog/disease-models` | `study:write` |
| `GET /api/subjects`, `GET /api/subjects/{id}`, `GET /api/subjects/{id}/weights` | `*:read` |
| `POST /api/subjects`, `PATCH, DELETE /api/subjects/{id}` | `subject:write` |
| `POST /api/subjects/{id}/weights` | `weight:write` |
| `GET /api/experiments`, `GET {e}`, `{e}/readiness`, `{e}/metric-options`, `{e}/groups`, `{e}/regimen-items`, `{e}/timepoints`, `{e}/endpoints`, `{e}/allocations` | `*:read` |
| `POST /api/experiments`, `PATCH {e}`, `POST {e}/start`, `POST {e}/complete`; `POST`, `PATCH`, `DELETE` on groups, regimen items, timepoints and endpoints; `POST {e}/allocations`, `POST {e}/allocations/randomise`, `PATCH, DELETE {e}/allocations/{allocationId}`, `POST {e}/allocations/{allocationId}/exclusion` | `study:write` |
| `GET {e}/administrations`, `GET {e}/dosing?date=`, `GET {e}/induction-outcomes`, `GET /api/subjects/{id}/administrations` | `*:read` |
| `POST {e}/allocations/{allocationId}/administrations`, `POST {e}/allocations/{allocationId}/induction-outcome` | `test:run` |
| `GET /api/environments`, `GET /api/environments/{id}`, its `revisions` and `revisions/{number}`, `GET /api/environment-revisions` | `*:read` |
| `POST, PATCH` environments; `POST` environment revisions | `apparatus:write` |
| `GET {e}/protocols`, `protocols/{protocolId}`, its `versions` and `versions/{number}` | `*:read` |
| `POST, PATCH` protocols; `POST` protocol versions | `study:write` |
| `GET /api/tests` (worklist across experiments), `GET {e}/tests`, `GET /api/tests/{id}`, `GET /api/tests/{id}/subjects/{subjectId}/trials` | `*:read` |
| `POST {e}/tests`, `POST /api/tests/{id}/subjects`, `DELETE /api/tests/{id}/subjects/{subjectId}`, `POST /api/tests/{id}/cancel` | `test:write` |
| `POST /api/tests/{id}/start`, `POST /api/tests/{id}/complete`, `POST /api/tests/{id}/subjects/{subjectId}/trials` | `test:run` |
| `GET, POST /api/tests/{id}/comments` | Any signed-in user |
| `PATCH /api/tests/{id}/comments/{commentId}` | The comment's author |
| `DELETE /api/tests/{id}/comments/{commentId}` | The comment's author, SUPERADMIN or LAB_MANAGER |
| `GET /api/tests/{id}/recordings`, `GET /api/tests/{id}/recordings/current`, `GET /api/tests/{id}/recordings/{recordingId}/read-url` | `*:read` |
| `POST /api/tests/{id}/recordings` | `test:run` |
| `POST /api/tests/{id}/recordings/{recordingId}/finalize` | `test:run`, and the uploader, SUPERADMIN or LAB_MANAGER |
| `GET /api/tests/{id}/calibration-status`, `GET /api/tests/{id}/subjects/{testSubjectId}/calibrations` and `calibration-status` | `*:read` |
| `POST /api/tests/{id}/subjects/{testSubjectId}/calibrations` (manual override for one animal on the current video) | `test:run` |
| `GET /api/environments/{id}/revisions/{number}/calibrations` and `calibration-status` | `*:read` |
| `POST /api/environments/{id}/revisions/{number}/calibrations` | `apparatus:write` |
| `GET /api/analysis/capabilities`, `GET /api/analysis/jobs/{jobId}`, its `video-pair` and `outputs/{kind}/read-url` (and the deprecated `GET /api/analysis-runs/{jobId}` and `video-pair`), `GET /api/tests/{id}/analysis-runs`, `GET /api/analysis/results`, `GET /api/analysis/results/{testSubjectId}` | `*:read` |
| `POST /api/analysis/test-subjects/{testSubjectId}/runs`; `POST /api/analysis/results/{testSubjectId}/accept`, `reject`, `run`; `POST /api/analysis/results/bulk-accept` | `test:run` |
| `GET, POST /api/analysis/workers`, `POST /api/analysis/workers/{id}/disable` | `lab:configure` |
| `GET /api/trials`, `GET /api/trials/{id}`, `GET /api/trials/{id}/video/read-url`, `GET /api/trials/{id}/calibrations` and `calibration-status` | `*:read` |
| `POST /api/trials`, `DELETE /api/trials/{id}`, `POST /api/trials/{id}/video`, `video/finalize`, `analyses`, `calibrations` | `trial:run` |
| `POST /api/worker/claim`, `POST /api/worker/runs/{runId}/heartbeat`, `source-url`, `outputs`, `result`, `failure` | A worker token |
| `GET {e}/report`, `GET {e}/report/export`, `GET /api/dashboard/summary` | `*:read` |

Use cases check the actor and permission themselves; HTTP handlers only translate.
Rules enforced by tests:

- Tokens are HS256 JWTs with issuer, audience, issued-at and expiry, carrying the
  user id and a session version but no role. Other algorithms are rejected.
- The current role is loaded from PostgreSQL on every request, so a role change
  applies to the next request and deleted users get 401 immediately.
- Resetting or changing a password increments the session version and revokes
  earlier tokens. `POST /api/auth/password` returns a replacement token.
- 401 means missing, malformed, expired or revoked credentials; 403 means the
  current role lacks the permission.
- Users cannot delete themselves. The last SUPERADMIN or LAB_MANAGER cannot be
  deleted or demoted to an unprivileged role; checks run in a SERIALIZABLE
  transaction with bounded retries, verified with concurrent requests.
- The SUPERADMIN account is the lab's founding account, created once by the
  setup command and never again: no request may create a second one, and none
  may rename, role-change, reset the password of, or delete the existing one,
  whether made by a LAB_MANAGER (despite otherwise holding identical
  permissions) or by that account acting on itself through these routes. It
  changes its own password only through `POST /api/auth/password`, which proves
  the current one. Every such attempt returns 403 `user.superAdminProtected`.
- Passwords need at least 8 characters and at most 72 UTF-8 bytes (the bcrypt
  limit; longer input is rejected, never truncated). They are compared byte for
  byte without normalization. Generated passwords are 26 random base32 characters.
- Unknown accounts and wrong passwords return the same 401 after a bcrypt comparison.
- There is no signup route. Setup creates an administrator only when no
  privileged user exists, so repeated runs create nothing.
- Unexpected errors log the route pattern and error only, never headers, bodies,
  tokens or passwords. Clients receive a stable `code` and a generic message.

## Catalog

Substances and disease models are laboratory-wide and reused across studies.
Names are unique ignoring case.

- **Substance**: name, `kind` (`COMPOUND`, `REFERENCE_DRUG`, `VEHICLE`,
  `INDUCER`) and description. The kind only guides pickers; what a substance
  does in a study comes from the regimen item that uses it.
- **Disease model**: name, `inductionType` (`CHEMICAL`, `SURGICAL`, `DIET`,
  `STRESS`, `OTHER`), an optional free-text `confirmationCriterion` and a
  description. Every model is induced; the inducing agent and its dose are part
  of each study's regimen.
- Lists filter by `search`, substances by `kind` and disease models by
  `inductionType`. Deleting a substance that a regimen item or administration
  uses returns 409 `substance.inUse`; deleting a disease model that a group
  uses returns 409 `diseaseModel.inUse`.

## Subjects and weights

- **Subject**: code (unique ignoring case), species (`MOUSE`, `RAT`), sex
  (`FEMALE`, `MALE`, `UNKNOWN`). An animal can be registered before it joins a
  study. The subject JSON carries a read-only `allocation` summary (experiment,
  group code and role, excluded) or null. A subject that is allocated or has
  weights cannot be deleted (`subject.inUse`).
- **Weight**: body weight in grams with up to 3 decimals and at most 5000 g,
  with the time it was measured. Weights belong to the animal, not to a study,
  and never change.
- `GET /api/subjects` filters by `search`, `species`, `sex` and
  `unallocated=true`, and sorts by `code`, `species`, `sex` or `createdAt`.

## Experiments

An experiment is designed before any animal is touched, then runs, then
completes.

- **Experiment**: code (unique ignoring case), title, description,
  `hypothesis`, `randomisation` (text) and `blinded`. `status` is `DRAFT`,
  `RUNNING` or `COMPLETED` and moves only forward. Code, title and description
  can be edited until completion; hypothesis, randomisation and blinding only in
  a draft.
- **Group**: code (such as `G1`), name, optional `targetSize` and a `role`:
  `HEALTHY_CONTROL`, `SHAM`, `DISEASE_CONTROL`, `POSITIVE_CONTROL` or
  `TREATMENT`. Every group except a healthy control names a disease model; a
  healthy control has none (database check). The group list reports how many
  animals are allocated against the target.
- **Regimen item**: what a group receives. `purpose` (`INDUCTION`, `VEHICLE`,
  `TREATMENT`), substance, dose (decimal text, stored exactly in millionths),
  unit, route, `frequency` (`ONCE`, `DAILY`, `TWICE_DAILY`, `WEEKLY`,
  `EVERY_N_DAYS` with `intervalDays`) and `startDay`/`endDay` in study days
  relative to day 0. A group has at most one `TREATMENT` item, so a treatment
  group has one substance at one dose level; dose response is the set of
  treatment groups that share a substance.
- **Timepoint**: name, `studyDay` (baseline days are negative or zero) and
  position, both unique in the experiment.
- **Endpoint**: what the study is judged on: one metric of one paradigm at one
  timepoint, `kind` `PRIMARY` or `SECONDARY` (at most one primary), `direction`
  (`HIGHER_IS_BETTER`, `LOWER_IS_BETTER`) and an optional baseline timepoint for
  change from baseline. `GET {e}/metric-options` lists the metrics of the
  paradigm versions the experiment's protocol steps use; an endpoint must name
  one of them.

Lifecycle:

- `GET {e}/readiness` lists the start rules the design breaks, each with a
  stable code (`readiness.noTimepoints`, `readiness.noGroups`,
  `readiness.targetSizeMissing`, `readiness.primaryEndpointMissing`,
  `readiness.endpointMetricUnavailable`, `readiness.treatmentItemMissing`,
  `readiness.treatmentItemNotAllowed`, `readiness.inductionItemMissing`,
  `readiness.inductionItemNotAllowed`, `readiness.vehicleItemMissing`,
  `readiness.diseaseControlMissing`) and the group or endpoint it concerns.
  The rules: at least one timepoint and one group, a target size on every
  group, one primary endpoint, every endpoint metric available; treatment and
  positive control groups have exactly one `TREATMENT` item and an `INDUCTION`
  item; a disease control has an `INDUCTION` and a `VEHICLE` item and no
  treatment; a sham has a `VEHICLE` item and no induction or treatment; a
  healthy control has no induction or treatment; every sham, treatment and
  positive control group has a disease control with the same disease model.
- `POST {e}/start` with `dayZero` (a date) runs the same check and answers 422
  `experiment.notReady` with the unmet rules, or moves the experiment to
  `RUNNING`. Day 0 never changes afterwards. Starting freezes groups, regimen
  items, timepoints and endpoints: database triggers reject changes with
  `*_frozen` constraints, answered as 409 `experiment.designFrozen`.
- `POST {e}/complete` needs every test of the experiment completed or cancelled
  (`experiment.testsOpen`). A completed experiment accepts no new rows anywhere,
  and analysis decisions return `analysis.experimentClosed`.

Allocation and exclusion:

- An animal belongs to exactly one group, so to one experiment, ever
  (`UNIQUE (subject_id)` on allocations). `POST {e}/allocations` takes
  `subjectId`, `groupId`, optional `cage` and `allocatedAt`.
  `POST {e}/allocations/randomise` takes up to 1000 `subjectIds`, optional
  `groupIds` and a cage, and distributes the animals at random over the groups
  up to each group's target size, filling them evenly; either every animal is
  allocated or none (`allocation.notEnoughPlaces`). Groups without a target take
  no animals.
- Animals can be allocated in `DRAFT` and `RUNNING`. In a draft an allocation
  can move to another group or be removed. Once the study runs the group is
  fixed: only the cage can change, and the animal can be excluded once with
  `POST .../exclusion` (`reason`: `DIED`, `HUMANE_ENDPOINT`,
  `INDUCTION_FAILED`, `TECHNICAL`, `OTHER`, optional note and time). Excluded
  animals stay allocated, so the report shows them; they join no new test and
  get no new analysis run.
- `GET {e}/allocations` filters by `groupId` and `excluded` and also returns
  every group's fill against its target.

Experiments are listed with `search`, `status`, `sort`, `order`, `page` and
`pageSize`. Routes under `/api/experiments/{id}` return 404 for groups,
timepoints, allocations and other rows of another experiment, and composite
foreign keys enforce the same scoping in the schema. Experiments cannot be
deleted through the API.

## Dosing

A regimen item is a plan; only an administration records that a dose was given.
Records need a running experiment.

- **Administration**: `POST {e}/allocations/{allocationId}/administrations`
  (`test:run`). With `regimenItemId` the purpose, substance, dose and route
  default to the item's, and a different actual amount or route may be given;
  the item must belong to the animal's own group. An unplanned dose names all of
  them. The time cannot lie in the future (5 minutes of clock skew are
  tolerated), before the allocation or after an exclusion.
- Per-kilogram units (`mg/kg`, `ug/kg`, `mL/kg`, `IU/kg`) need a weight of the
  same animal measured within the 7 days before the dose: the named
  `weightMeasurementId`, or by default the latest one. The response includes
  `absoluteDose` (dose times body mass, rounded half up to 6 decimals).
  Records copy the substance name, dose, route and body weight, so later edits
  of the catalog do not change history. Administrations never change.
- `GET {e}/dosing?date=YYYY-MM-DD` (default: today in the laboratory's time
  zone) lists every dose due that day from the regimen items and day 0, per
  animal, with how many were given and the absolute dose for the latest weight.
  `TWICE_DAILY` is due twice a day; `WEEKLY` and `EVERY_N_DAYS` are due on the
  start day and every 7 or `intervalDays` days after.
- `GET {e}/administrations` and `GET /api/subjects/{id}/administrations` filter
  by animal, substance, purpose and time range.
- **Induction outcome**: `POST {e}/allocations/{allocationId}/induction-outcome`
  records once whether the animal's disease model was `CONFIRMED` or
  `NOT_CONFIRMED`, with the assessment time and a note. The induction itself is
  an administration with purpose `INDUCTION`. `GET {e}/induction-outcomes` lists
  them.

Units: `mg`, `ug`, `g`, `mL`, `uL`, `IU`, `mg/kg`, `ug/kg`, `mL/kg`, `IU/kg`.
Routes: `ORAL`, `INTRAPERITONEAL`, `SUBCUTANEOUS`, `INTRAVENOUS`,
`INTRAMUSCULAR`, `INTRANASAL`, `INTRACEREBROVENTRICULAR`, `TOPICAL`,
`INHALATION`. Composite foreign keys tie an administration's group and subject
to its allocation, its regimen item to the same group and its weight to the same
animal, and a check constraint requires a weight for per-kilogram doses.

## Paradigm catalog and metric engine

Paradigm, metric, unit, event and calculation definitions are Go code in
`internal/paradigms/domain`. There are no create, update or delete routes; a
change means a new published version, and callers only receive copies.

- `ParadigmVersion` identifies one paradigm's contract, `MetricEngineVersion`
  the calculations and `ResultSchemaVersion` the shape of a result. Every result
  and manifest carries all three.
- `GET /api/paradigms/{key}/versions/{version}` is the manifest a worker
  consumes: parameters with units, ranges, defaults and value type, zones
  (derived from parameters or calibrated per video), the interval and point
  events it derives (with their labels and whether they occur at most once),
  metrics with definition, formula, inputs, missing-data rule and tolerance, and
  QC rules. `inputEvents` is always empty and only kept so published manifests
  never change.

The catalog has four paradigms, all at version 1, in two categories: `MWM`
(Morris water maze) and `NOVEL_OBJECT` (LEARNING_MEMORY), `POLE` and
`TREADMILL` (MOTOR). Every paradigm is analysed automatically: a worker tracks
the subject's centroid in the calibration's centimetre frame, and the Go engine
derives every event and metric from the samples and the zones; a person only
reviews (accepts or rejects) the result. There is no observer scoring. Units
are `cm`, `cm_s`, `s`, `count`, `ratio` and `boolean`.

Every paradigm takes the same input: parameters, samples (time in microseconds,
x and y in centimetres, tracked or lost), calibrated zone shapes and the
recorded duration. Calibrated zones are circles or polygons with at least three
vertices, in the same centimetre coordinates as the samples, drawn once with
the environment's calibration (a per subject override on the test video can
change them, for example a novel object on the other side); a zone is the
union of its shapes and includes its boundary. Unknown, missing required or
malformed zones are rejected. A counted interval is a pair of consecutive
tracked samples with 0 < dt <= `max_sample_gap_s`; time and zone membership
come from the earlier sample, and an occupancy run is a sequence of contiguous
counted intervals whose earlier sample lies in a zone. Every rule threshold is
an ANALYSIS parameter with a standard default, so a lab can tune it per
environment.

| Paradigm | Frame and zones | Analysis parameters (default) | Derived events | Metrics |
|---|---|---|---|---|
| `MWM` | Tank-centred, x east, y north; samples outside the tank are lost. Platform, target quadrant and wall annulus derived from parameters | `wall_annulus_width_cm` (12), `min_platform_dwell_s` (1), `max_sample_gap_s` (0.5) | `in_platform` intervals, `platform_entry` points | escape latency (start of the first platform dwell of at least `min_platform_dwell_s`), path length and mean swim speed up to the escape, target quadrant and thigmotaxis time ratios, platform crossings, time-weighted mean distance to the platform |
| `NOVEL_OBJECT` | Top view, origin at a corner of the arena floor, x right, y down the image (calibration points within the arena). Calibrated `novel_object` and `familiar_object`: the object plus the exploration distance (about 2 cm) | `min_bout_s` (0.3), `min_total_exploration_s` (0), `max_sample_gap_s` (0.5) | `exploration` intervals labelled `novel` or `familiar`: occupancy runs of an object zone lasting at least `min_bout_s` | novel, familiar and total exploration time (sum of the bouts), discrimination index |
| `POLE` | Side view, x horizontal, y the height above the base (base y = 0, top y = `pole_length_cm`). Calibrated `pole` (where the subject climbs) and `base` (floor at the foot of the pole) | `turn_descent_cm` (2), `fall_speed_cm_s` (80), `max_sample_gap_s` (0.5); session `max_trial_duration_s` (60) | single `turn_complete`, `base_reached` and `fall` points | time to turn, total time, descent time and speed, fall detected |
| `TREADMILL` | Side view, x along the lane from the shock grid end (x = 0) to the front (x = `lane_length_cm`), y vertical. Calibrated `shock_grid` | `exhaustion_dwell_s` (5), `min_shock_dwell_s` (0.2), `max_sample_gap_s` (0.5); session belt protocol and `max_trial_duration_s` (1800) | `shock` points, a single `exhaustion` point | latency to exhaustion, run time, run distance (integral of the belt speed), shocks strictly before the run ends |

Event rules, in the order the engine applies them:

- `NOVEL_OBJECT`: exploration is approximated by the body centre inside the
  zone, not by nose direction, so sitting on the object counts as exploration.
  A lost frame or a gap longer than `max_sample_gap_s` ends a bout.
- `POLE`: samples after `max_trial_duration_s` are ignored. A sample is on the
  pole when it lies in `pole` and not in `base`. `turn_complete` is the first
  tracked sample on the pole at least `turn_descent_cm` below the highest
  sample on the pole so far. `fall` is a pair of consecutive tracked samples at
  most `max_sample_gap_s` apart, from on the pole to the base or off the pole,
  with a downward speed above `fall_speed_cm_s`, placed at the later sample.
  `base_reached` is the first tracked sample in `base` after a sample on the
  pole, unless the subject fell first; the search ends at the base or the fall.
- `TREADMILL`: samples after `max_trial_duration_s` are ignored, so a stay is
  cut at the cut-off. Every occupancy run of `shock_grid` lasting at least
  `min_shock_dwell_s` is a `shock` at its start; the first lasting
  `exhaustion_dwell_s` is the `exhaustion` at its start (a lost frame breaks
  the stay). The run ends at exhaustion, or without it at the end of the
  recording, censored at `max_trial_duration_s`.

With no counted interval, every metric is reported missing with reason
`NO_VALID_INTERVALS`, never as zero. Tracked samples need finite coordinates,
timestamps must be non-negative and strictly increasing, the recorded duration
must not be negative, and unknown or out-of-range parameters are rejected.
Tolerances are 1e-6 in the metric unit for times, distances and speeds, and
1e-9 for ratios, counts and booleans. QC rules are the same for every paradigm:
tracking confidence at least 0.7, at most 10% lost frames and a calibration
error of at most 2 cm.

Missing metrics carry a reason instead of zero: `NO_VALID_INTERVALS`,
`EVENT_NOT_OBSERVED` (for example no escape in a probe trial, no turn or base
by the cut-off), `ZERO_DENOMINATOR`, `INCOMPLETE_RECORDING` (no exhaustion and
a recording shorter than the cut-off) and `BELOW_EXPLORATION_CRITERION`. Rules
across parameters reject a platform outside the tank, a wall annulus as wide as
the tank radius, and belt end speeds below start speeds.

Every published manifest is pinned to a golden file in
`internal/paradigms/adapters/http/testdata/manifests`, and the test fails when a
published contract changes or disappears. Change a contract by publishing a new
version; `go test ./internal/paradigms/adapters/http -update` only writes
goldens for versions that do not have one yet.

## Environments and protocols

An environment is a physical apparatus set up for one paradigm, such as a
particular water tank or one of three pools filmed by one camera. In
a test, each animal is placed in an arena, which is one environment revision.
Its measurements are stored as numbered revisions.

- Creating an environment stores revision 1; `POST /api/environments/{id}/revisions`
  adds the next number. The environment row is locked while a revision is
  numbered, so concurrent revisions get consecutive numbers.
- A revision's `parameters` give every apparatus parameter of its paradigm
  version, because no physical measurement is defaulted, plus optional analysis
  parameters, which default. Values must be within range and satisfy the
  version's rules across parameters, such as an MWM platform inside the tank.
- Revisions are immutable: a trigger rejects `UPDATE` and `DELETE`, and a
  composite foreign key stops the environment's paradigm from changing. Only an
  environment's name and notes can be edited. `GET /api/environment-revisions`
  lists every revision with its environment name, for arena pickers.

A protocol belongs to one experiment and has numbered versions. A version is an
ordered list of 1 to 50 steps, and the steps may use different paradigms. It is
the test battery of the experiment; its steps name no apparatus, because the
arena is chosen per animal in each test.

- A step names a paradigm key and version, a trial type defined by that version,
  1 to 1000 trials, an inter-trial interval of 0 to 86400 seconds and session
  parameter values. Positions run from 1 without gaps or duplicates.
- Session values are validated against the paradigm version and stored with
  defaults filled in.
- Versions and steps are immutable. Triggers reject `UPDATE` and `DELETE`, and a
  deferred constraint trigger rejects steps added to a version after it was
  committed. Protocols and versions can be added while the experiment is a draft
  or running, not after it completed (`protocol.experimentClosed`).
- Protocol routes are scoped to their experiment, so another experiment's
  protocol id returns 404.

## Tests, trials and comments

A test is one session of a running experiment: one timepoint, one step of a
protocol version, one video, and one or more animals (test subjects), each in
its own arena. Usually one animal; sometimes several arenas share the camera
frame. Animals of different groups may share a session. The group of a
measurement always comes from the animal's allocation; nothing about the group
is copied onto the test.

- `POST {e}/tests` takes the timepoint, protocol version, step position,
  optional `scheduledAt` and `notes`, and `subjects`: 1 to 12 pairs of
  `allocationId` and `environmentRevisionId`. The paradigm, its version and the
  planned trials come from the step. `scheduledAt` defaults to day 0 plus the
  timepoint's study day at midnight in the laboratory's time zone.
- Every arena must run the step's paradigm version, each animal and each arena
  appears once per test, an animal must be allocated in the experiment and not
  excluded, and an animal cannot be in another non-cancelled test of the same
  timepoint and step (`test.animalAlreadyScheduled`). Composite foreign keys and
  unique keys enforce the same in the schema.
- While a test is `PLANNED`, `POST /api/tests/{id}/subjects` adds an animal and
  `DELETE /api/tests/{id}/subjects/{subjectId}` removes one; the last animal
  stays (`test.lastSubject`).
- Status moves only forward: `PLANNED` to `IN_PROGRESS` (start) or `CANCELLED`,
  and `IN_PROGRESS` to `COMPLETED` or `CANCELLED` (with a reason). Transitions
  lock the test row, so concurrent requests get one success and
  `test.invalidTransition` (409) for the rest. A trigger rejects other status
  changes, edits of the context columns and deletes. A test cannot start while
  one of its animals is excluded; remove it first.
- Trials are recorded per animal while the test is in progress, with
  `POST /api/tests/{id}/subjects/{subjectId}/trials`. `repetition` runs from 1 to
  the planned trials; recording a repetition again adds the next `attempt` and
  never replaces an earlier trial. Trials are immutable. Completing a test needs
  at least one trial for every animal and a time after its start and every trial.
- `GET /api/tests` is the worklist across experiments, filtered by
  `experimentId`, `status`, `timepointId` and a `from`/`to` date range in the
  laboratory's time zone. Test status is the session's lifecycle only; analysis
  has its own status per animal.

Comments belong to a test and are plain text of 1 to 2000 characters; control
characters other than tab and line breaks are removed. Every signed-in role,
including VIEWER, can read and write comments. Only the author edits, and the
author, SUPERADMIN or LAB_MANAGER deletes. A comment id is accepted only under
its own test. One user may create or edit at most 20 comments per minute
(`comment.rateLimited`, 429); the counter lives in the API process, so each
instance counts separately.

## Video storage

Original videos go straight from the browser to a private Google Cloud Storage
bucket; the API never proxies video bytes and never stores signed URLs or upload
session URIs.

A test has one current video. Videos attach only to a test that is
`IN_PROGRESS` or `COMPLETED` (`media.testNotStarted` otherwise); a new upload
supersedes the current recording at once and the old one stays readable for
provenance (`supersedesId`, `current` on every recording,
`GET /api/tests/{id}/recordings/current`). The test row is locked while a
video is attached, so concurrent uploads form one linear chain.

1. `POST /api/tests/{id}/recordings` declares file name, content type
   (`video/mp4`, `video/quicktime` or `video/webm`), size, base64 CRC32C and the
   clip range, and stores a `PENDING` video with a new object name
   `tests/{testId}/originals/{videoId}`. The response carries a V4-signed `POST`
   with `x-goog-resumable: start`, `x-goog-if-generation-match: 0`, so the
   upload cannot overwrite an existing object, and
   `x-goog-content-length-range: 0,<size>`, so storage refuses more bytes than
   declared. The client sends all of them, uploads to the returned session URI
   in chunks (a `308` without `Range` means storage kept nothing and the upload
   restarts at 0) and keeps that URI to itself.
2. `POST .../recordings/{recordingId}/finalize` (the uploader, LAB_MANAGER or
   SUPERADMIN) reads the object's attributes from GCS first and only then locks
   the database row. A missing object returns `media.uploadIncomplete`; a size,
   CRC32C, content type or maximum-size mismatch stores `REJECTED` with a reason
   and nothing is deleted. A match stores `VERIFIED` with the object generation.
   Finalizing again returns the stored result, and a trigger keeps verified and
   rejected videos unchanged.
3. `GET .../recordings/{recordingId}/read-url` signs a `GET` for the verified
   generation, valid for `READ_URL_TTL`. GCS serves HTTP range requests on it, so
   players can seek.

Media metadata such as duration, codec and time base is not probed yet; analysis
workers read it later.

Required GCP configuration, supplied by the operator; the API creates or deletes
no cloud resources:

- A private bucket with uniform bucket-level access and public access
  prevention, for example
  `gcloud storage buckets create gs://BUCKET --location=REGION --uniform-bucket-level-access --public-access-prevention`.
- A CORS policy for the UI origin:
  `gcloud storage buckets update gs://BUCKET --cors-file=cors.json` with
  `[{"origin":["https://UI_ORIGIN"],"method":["GET","HEAD","POST","PUT","OPTIONS"],"responseHeader":["Content-Type","Content-Range","Range","Location","ETag","x-goog-resumable","x-goog-content-length-range"],"maxAgeSeconds":3600}]`.
- Application Default Credentials for the API (workload identity on GCP; no
  service account JSON in the repository). The identity needs object create and
  get permissions on the bucket, for example `roles/storage.objectUser`. When the
  credentials cannot sign (workload identity), set `GCS_SIGNER_EMAIL` to a service
  account that grants the API `roles/iam.serviceAccountTokenCreator` so URLs are
  signed through the IAM Credentials API.

| Variable | Default | Meaning |
|---|---|---|
| `GCS_BUCKET` | empty | Bucket name; takes precedence over `LOCAL_STORAGE_DIR`. With neither, video routes return 503 `media.storageNotConfigured` |
| `LOCAL_STORAGE_DIR` | empty | Keep objects as files in this directory instead of GCS (development, single machine) |
| `GCS_SIGNER_EMAIL` | empty | Service account used to sign URLs through IAM |
| `VIDEO_MAX_BYTES` | 21474836480 | Largest accepted video, 1 MiB to 5 TiB |
| `UPLOAD_URL_TTL` | 1h | Validity of the signed upload start, 1m to 168h |
| `READ_URL_TTL` | 15m | Validity of read URLs, 1m to 12h |

Local storage (`internal/media/adapters/localfs`) keeps each object under
`LOCAL_STORAGE_DIR/objects` with its generation, size, CRC32C and content type,
and never overwrites one (`if-generation-match: 0`). The API serves its signed
URLs under `/api/local-storage/`: the same resumable upload (POST start, PUT
chunks answered by 308 with `Range`, a 256 KiB granularity) and generation
pinned ranged reads as a bucket, so the panel and the worker need no changes.
The URLs are relative, HMAC-SHA256 signed with a key derived from `JWT_SECRET`,
and expire after `UPLOAD_URL_TTL` or `READ_URL_TTL`; the worker prefixes
`MISKO_API_URL`.

The unit and integration tests use a fake object store and an offline signer;
they do not prove real GCS behavior. The live smoke test uploads a 3 MiB object
under `smoke/` with a resumable session, checks CRC32C, the overwrite
precondition, a range read, CORS and URL expiry, and deletes nothing:

```sh
GCS_LIVE_BUCKET=BUCKET GCS_LIVE_ORIGIN=https://UI_ORIGIN go test -tags=gcslive -run TestLive ./internal/media/adapters/gcs/
```

## Calibration

Physical metrics need a validated calibration. Paradigms whose QC contract has
`max_calibration_error_cm` require one; every published paradigm does. A
paradigm without that rule would report `NOT_REQUIRED`.

- `POST /api/tests/{id}/subjects/{testSubjectId}/calibrations` (`test:run`)
  takes the camera id, frame size, crop, the video time of the reference frame,
  the measurement plane (`ARENA_FLOOR`, `WATER_SURFACE`, `APPARATUS_TOP`, or
  `APPARATUS_SIDE` for rigs filmed from the side such as the pole and the
  treadmill lane, migration `017_side_view_plane.sql`), 4 to
  100 FIT points and 3 to 100 CHECK points, each a full-frame pixel with plane
  coordinates in centimeters. It needs the test's current video verified
  (`calibration.noVideo`, `calibration.videoNotVerified`).
- The transform is a homography fitted with the normalized direct linear
  transform: both point sets are centered and scaled, the 8 unknowns are solved by
  least squares with partial pivoting, and a negligible pivot (collinear or
  repeated points) is rejected as `calibration.degenerateFit`. The algorithm
  version `homography-dlt-normalized-v1` is stored with the result.
- Points must lie inside the crop, the crop inside the frame, CHECK points at
  least 1 pixel away from every FIT point, and plane coordinates inside the
  environment revision's geometry plus the tolerance where the paradigm fixes
  it: the MWM tank and the NOVEL_OBJECT arena floor (0 to width, 0 to depth).
  The side views of the pole and the treadmill choose their own frame.
- The stored calibration is `VALID` when the largest CHECK error is within the
  paradigm's `max_calibration_error_cm`, otherwise `REJECTED` with
  `EXCESSIVE_CHECK_ERROR`. FIT error alone never validates a calibration.
- Calibrations are immutable. A correction names `supersedesId`, which must be
  the latest calibration of its chain; within one video it keeps the camera id
  and frame size, and the crop may change. The chain's row (environment
  revision or test subject) is locked, a unique index allows one first
  calibration, and a unique `supersedes_id` keeps one linear chain.
- A calibration has a scope. An `ENVIRONMENT` calibration is the default of one
  environment revision (one arena), entered once when the rig is set up with
  `POST /api/environments/{id}/revisions/{number}/calibrations`
  (`apparatus:write`, no video, so no reference frame); its crop is that arena's
  region of the fixed camera frame, so arenas filmed together each have their
  own. An `OVERRIDE` is a manual calibration of one animal of a test (a test
  subject) on the test's current video, for example when the camera or arena
  moved; it must use the animal's arena and a video of its test. A `TRIAL`
  calibration belongs to a paradigm trial and its current video
  (`POST /api/trials/{id}/calibrations`, `trial:run`); it never changes the
  environment. There is one chain per revision, one per test subject spanning
  the test's videos and one per trial spanning the trial's videos: the first
  calibration on a replacement video supersedes the latest one of the previous
  video and may use another camera.
- An animal is analyzed with its own latest override on the current video when
  there is one, and a `REJECTED` one leaves it waiting instead of falling back to
  the arena default. Otherwise it uses the latest default of its arena, so a
  replacement video starts from the defaults again. The
  `misko.effective_calibrations` view decides this, and the status endpoints,
  analysis and the dashboard all read it.
- `GET /api/tests/{id}/calibration-status` lists every animal of a test and
  `GET .../subjects/{testSubjectId}/calibration-status` one of them:
  `WAITING_FOR_VIDEO` while the test has no current verified video,
  `WAITING_FOR_CALIBRATION` until a valid calibration applies, then `CALIBRATED`
  with that calibration. The response also carries `source` (`ENVIRONMENT` or
  `OVERRIDE`), `drift` (`UNCHECKED`: nothing checks camera or arena movement
  yet, so a calibration taken from the environment is trusted as entered), the
  arena and the current recording.
- A trial is analysed with its own latest calibration on its current video when
  there is one (a `REJECTED` one leaves it waiting), otherwise the latest
  default of its arena (`misko.trial_effective_calibrations`).
  `GET /api/trials/{id}/calibration-status` reports it with source `TRIAL` or
  `ENVIRONMENT`.
- An analysis job copies the calibration it uses (frame, crop, plane,
  transform and zones in pixels and centimetres) and keeps its id, so older
  jobs keep their inputs after a correction.

## The analysis service

Computer vision runs outside the API in analysis workers. The analysis service
(`internal/analysis`) queues jobs, leases them to workers, verifies what they
upload and publishes it; it never reads video bytes. Workers measure positions
only: the Go metric engine computes every published metric and event from the
stored trajectory. The worker in [`worker/`](../worker/README.md), tested only
on synthetic video so far, tracks the subject's centroid for every paradigm of
the catalog and refuses a paradigm version it does not support with
`UNSUPPORTED_PARADIGM`.

A job analyses "this video, with this paradigm version, these parameters and
this calibration":

- **Input snapshot** (never changes): the requester (`TRIAL` or `TEST_SUBJECT`
  and its id, an opaque reference the service never joins), the paradigm key
  and version with the metric engine and result schema versions, the resolved
  parameters (apparatus, analysis and session values), the source video asset
  with its GCS generation, CRC32C and clip, the trigger (`AUTOMATIC` or
  `MANUAL`), an optional idempotency key and, when the paradigm needs one, a
  copy of the calibration (frame size, crop, measurement plane, homography and
  zones in pixels and centimetres) with the id it was copied from.
- **Execution state**: status `QUEUED`, `RUNNING`, `SUCCEEDED` or `FAILED`,
  attempts and the lease, the worker and model version, the outputs
  (`analysis_job_uploads`, `analysis_job_outputs`, the video pair), the metric
  results, the derived events and the QC of the trajectory
  (`analysis_job_qc`: samples, tracked, inferred, lost and inferred ratios, mean
  confidence and whether the rules passed).

Other modules use the in-process port; they authorize their own users:

- `Submit(request)` validates the request, reads the paradigm contract, the
  source video through the media asset port (it must be `VERIFIED`) and, when
  the paradigm needs one, the calibration through the calibration port (it
  must be `VALID` with a shape for every required zone), and stores a `QUEUED`
  job. A request with an idempotency key seen before returns the first job.
  Refused inputs are `ErrInvalidRequest`, `ErrNotAnalysable`,
  `ErrSourceNotVerified`, `ErrCalibrationRequired`, `ErrMissingZones` and
  `ErrInvalidCalibration`; the requesters answer them as "not ready". A job
  waits in the queue until a worker with the capability claims it.
- `Get(id)`, `Jobs(ids)`, `ByRequester(kind, ids)` (newest first) and
  `Results(ids)` (with metrics, events, outputs, pair and QC); at most 500 ids
  per call.
- A `Listener` hears about every success after it is stored; bootstrap tells
  the experiment side about jobs of animals of tests.

Workers are service identities, not users:

- `POST /api/analysis/workers` (`lab:configure`) registers a name, model version
  and capabilities (paradigm key and version from the catalog). A capability must
  be computable from a trajectory and the pinned calibration's zones; versions
  outside the catalog return `worker.unknownCapability`. The response
  shows the token (`mw_...`) once; only its SHA-256 hash is stored.
  `GET /api/analysis/workers` lists workers and
  `POST /api/analysis/workers/{id}/disable` revokes a token; its running lease
  then expires normally. `GET /api/analysis/capabilities` (`*:read`) lists what
  enabled workers can analyze.
- Worker routes under `/api/worker` accept only `Authorization: Worker TOKEN`.
  A user bearer token there returns 401 `worker.unauthenticated`, and a worker
  token on a user route returns 401 `auth.unauthenticated`. The routes keep the
  name "runs": a run id is a job id.

The worker protocol:

- `cmd/jobs` ticks every `JOB_INTERVAL`. The analysis service's part of a tick
  returns jobs whose lease expired to the queue, or fails them with
  `LEASE_EXPIRED` after the last attempt (`FOR UPDATE SKIP LOCKED`, so several
  schedulers may run). The same tick runs the experiment side's orchestrator
  and the trials' automatic analyses (below).
- `POST /api/worker/claim` leases the oldest available queued job the worker can
  analyze (`FOR UPDATE SKIP LOCKED`), increments `attempt` and returns `run` and
  `job` (the same job: `run` is the name workers read), `requester`,
  `testSubject` (`{id}` for a job of an animal of a test, for logs; null for a
  trial), the output prefix `jobs/{jobId}/attempts/{attempt}/`, the trajectory
  schema `misko.trajectory.v1`, the contract (the required zones, the metrics,
  event types, missing reasons and QC rules the engine will apply) and the
  job's calibration snapshot (frame size, crop, plane, the pixel to centimeter
  homography and its zones, each in pixels and centimetres, or null). With
  nothing to do it returns 204. A lease lasts 5 minutes;
  `.../runs/{jobId}/heartbeat` extends it. A job has 3 attempts.
- The attempt is a fencing token. Heartbeat, `source-url`, `outputs`, `result`
  and `failure` take the attempt and return 409 `analysis.staleAttempt` unless the
  caller holds the current lease. Database triggers accept output rows only for
  the current attempt of a `RUNNING` job, move the status and attempt only
  forward, and keep the snapshot, finished jobs and published rows unchanged.
- `.../source-url` signs a read of the pinned source generation. `.../outputs`
  records an output of the attempt (`ANALYZED_VIDEO` as MP4 or WebM, `TRAJECTORY`
  as `misko.trajectory.v1` JSON, `THUMBNAIL` as JPEG or PNG) with size and CRC32C,
  and signs a resumable upload that cannot overwrite an existing object.
- `.../result` names the output objects and the video pair, and the recording
  duration when the clip has no end; metrics and events are not accepted. The API
  rejects outputs outside the attempt prefix, anything but exactly one analyzed
  video and one trajectory, and a pair whose source offset is not the clip start.
  It then reads every output object's attributes from GCS and requires the
  declared size, CRC32C and content type (`analysis.outputNotVerified` otherwise).
- It reads the verified trajectory generation (at most 64 MiB) and requires the
  job id (as `runId`), attempt and recording duration, equal-length `tUs`, `xCm`,
  `yCm`, `tracked` and `confidence` arrays, strictly increasing times within the
  recording, confidences from 0 to 1 and coordinates for tracked samples
  (`analysis.invalidTrajectory`). An optional `inferred` array of the same
  length marks tracked samples whose position the worker placed without seeing
  it (the current worker never sets it); an inferred sample must be tracked. QC
  rules the trajectory can measure are applied: `max_lost_frame_ratio` against
  untracked samples (inferred ones are tracked), `min_tracking_confidence`
  against the mean confidence of observed samples and, when a paradigm sets it,
  `max_inferred_ratio` against inferred samples. A failure stores the QC, marks
  the job `FAILED` with a `QC_FAILED` reason, without retry, and publishes
  nothing else.
- The Go metric engine then computes the metrics and events from the samples
  with the job's parameters, the snapshot's zones and the duration; an event's
  confidence is the mean confidence of the tracked samples it covers. Outputs,
  metrics, events, the pair and the QC are written with the `SUCCEEDED` status in
  one transaction; a deferred trigger refuses success without the pair and its
  analysed video. Delivering the accepted result again returns the job (and
  tells the listener again).
- `.../failure` records a reason. A retryable failure returns the job to the
  queue while attempts remain; otherwise the job is `FAILED`.

Users read jobs with `*:read`: `GET /api/analysis/jobs/{jobId}` (the snapshot
and the stored result: metrics, events, outputs, pair, QC),
`GET /api/analysis/jobs/{jobId}/video-pair`, which signs reads of the pinned
original generation and the job's own analysed video with the time offsets and
mapping version for synchronized playback, and
`GET /api/analysis/jobs/{jobId}/outputs/{kind}/read-url`, which signs a read of
one output, such as the trajectory for drawing the track.
`GET /api/analysis-runs/{jobId}` and its `video-pair` are deprecated names of
the first two.

## The experiment side of analysis

`internal/subjectanalysis` owns `test_subject_analyses` (which job analyses
which animal of a test, from which recording, with which effective calibration
and why) and `subject_results` (the review). It reads the study tables to decide
readiness and the jobs only through the port and the `test_subject_jobs` view.

- The orchestrator (in every scheduler tick) submits one `AUTOMATIC` job for
  every animal of a test that is not cancelled, in an experiment that is
  running or completed, not excluded, whose test's current video is
  `VERIFIED`, with a valid calibration of its arena (its own override for this
  video, else the arena default) with every required zone when the paradigm
  requires it. The parameters are the arena's merged with the step's session
  values. The query returns only candidates it can plan (at most 500, oldest
  video first), so animals waiting for a calibration never starve newer ones;
  a candidate that fails is logged and skipped while the others are submitted.
  The job's idempotency key and a partial unique index on the link (animal,
  recording, calibration) make concurrent ticks submit each automatic job
  once; a new override of the animal submits a new job, a corrected arena
  default does not.
- `POST /api/analysis/test-subjects/{testSubjectId}/runs` (`test:run`) submits
  a `MANUAL` job of one animal with the current inputs, or returns 409
  `analysis.notReady`, and 409 `analysis.runPending` while a job of the animal
  is queued or running. `GET /api/tests/{id}/analysis-runs` (`*:read`,
  `?testSubjectId=` for one animal) lists the linked jobs as runs (`id` is the
  job id).
- The first successful job of an animal opens its result as `PENDING`: the
  analysis service's listener tells this module right after the success, and
  the tick opens any result it missed.

Every result is approved by a person before it counts:

- `GET /api/analysis/results` (`*:read`) lists results for review, `PENDING` by
  default (`status=ACCEPTED|REJECTED|ALL`, `experimentId`, `page`, `pageSize` up
  to 200), oldest first, with the animal, its arena, the test, the job under
  review with its metrics (read through the port) and whether a newer succeeded
  job exists. In a `blinded` experiment `group` is null: the reviewer sees no
  group, disease, substance or dose. `GET /api/analysis/results/{testSubjectId}`
  adds every succeeded job of the animal.
- `POST .../{testSubjectId}/accept` (`test:run`, optional `note`) and
  `POST .../{testSubjectId}/reject` (`reason`: `TRACKING_LOST`, `WRONG_ANIMAL`,
  `VIDEO_DEFECT`, `PROTOCOL_DEVIATION`, `OTHER`, optional `note`) record the
  decision and the reviewer; a decision can be revised while the experiment
  runs. `POST .../{testSubjectId}/run` with `jobId` (or `runId`) puts another
  succeeded job of the same animal under review and sends the result back to
  `PENDING`; a job linked to no animal, such as a trial's, returns
  `analysis.runNotFound`. `POST /api/analysis/results/bulk-accept` accepts 1 to
  100 results at once, all or none. A newer job never replaces the reviewed one
  by itself, and once the experiment is completed every decision returns 409
  `analysis.experimentClosed`.
- Reports and the dashboard read accepted metrics through the
  `report_accepted_metrics` and `test_subject_jobs` views, which only reach
  jobs linked to animals of tests: a paradigm trial never enters them.

## Paradigm trials

`internal/trials` lets a lab try a paradigm version on one video in one arena
outside any experiment, to see what the analysis measures and how its ANALYSIS
thresholds change the result. A trial has no groups, no review and no report.
It owns `paradigm_trials` and `paradigm_trial_jobs` (which job analysed which
trial video, with which calibration and which overrides).

- `POST /api/trials` (`trial:run`) with `name`, `paradigmKey`,
  `paradigmVersion`, `environmentRevisionId` and optional `notes`; the arena
  must run that version (400 `trial.paradigmMismatch`). `GET /api/trials`
  (`*:read`, `search` in the name, `paradigmKey`, `page`, `pageSize` up to 200)
  lists trials newest first with their arena and newest job;
  `GET /api/trials/{id}` returns the trial, its arena, its video, the
  calibration it would be analysed with now and every analysis, newest first,
  with the job's state, metrics and QC.
- `POST /api/trials/{id}/video` (`trial:run`, `fileName`, `contentType`,
  `sizeBytes`, `crc32c`, optional `clipStartUs`, `clipEndUs`) declares the
  trial's video through the media asset port (stored under
  `trials/{id}/originals/`) and signs its resumable upload; a replacement
  takes the place of the current video and earlier analyses keep theirs.
  `POST /api/trials/{id}/video/finalize` verifies the stored object and, once
  verified, submits the automatic job when the trial is ready.
  `GET /api/trials/{id}/video/read-url` signs a read for the calibrator.
- A trial whose paradigm needs a calibration is ready once its own calibration
  on its video, or else its arena's default, is valid with every required
  zone. A trial calibrated after its upload gets its automatic job in the next
  tick; each trial video gets one automatic job.
- `POST /api/trials/{id}/analyses` (`trial:run`, optional `overrides`)
  analyses the current video again with the arena's parameters and the given
  ANALYSIS values; any other key or a value out of range returns 400
  `trial.invalidOverrides`. Earlier jobs stay for comparison.
- `DELETE /api/trials/{id}` (`trial:run`) hides the trial: its jobs never
  change and stay readable through `GET /api/analysis/jobs/{jobId}`, so the
  trial row is kept with `deleted_at`.

The integration tests cover the job queue (idempotency, snapshot, claims,
fencing, deferred publication, expired leases), the experiment flow through the
port (orchestrator, review, reanalysis, report and dashboard), the arena
calibration rule, a trial end to end (upload, calibration on the trial video,
automatic job, result, a second analysis with a changed threshold, delete) and
that a trial job never reaches review, reports or the dashboard, with a fake
object store. They do not prove real GCS behavior or any tracking accuracy.

## Reports and exports

The study report reads the design tables and the read-only views
`report_test_subjects` and `report_accepted_metrics` (`schema/012_reports.sql`,
redefined over analysis jobs in `018_analysis_jobs.sql`);
it never writes. Every role has `*:read`.

- `GET /api/experiments/{id}/report` (`?endpointId=` for one endpoint) returns,
  for every endpoint (the primary first), the groups at the endpoint's timepoint
  and a time course over every timepoint with measurements. The experimental
  unit is the animal: each animal contributes one value per timepoint, the mean
  of its accepted results there. Only `ACCEPTED` results of animals that are not
  excluded count. Per group: `n`, mean, sample SD, SEM, minimum, maximum, the
  change from the endpoint's baseline timepoint per animal, and how many animals
  were tested, are pending review, wait for analysis, were rejected, have an
  accepted result without the metric, and are excluded.
- Comparisons come from the group roles: model validity (disease control vs
  healthy control, and vs sham of the same disease model), assay sensitivity
  (positive control vs disease control) and efficacy (each treatment vs the
  disease control of its disease model). Each gives the difference, the percent
  difference and the effect signed by the endpoint's direction; assay
  sensitivity and efficacy add rescue % = (group - disease control) /
  (healthy reference - disease control) x 100, where the healthy reference is a
  healthy control, else a sham of the same disease model. Dose response lists
  the treatment groups of one substance and unit by dose.
- Values of different paradigm versions, metric engine versions or units are
  never pooled: the section lists them in `versions`, sets `comparable` to
  false and computes no means or comparisons. Statistical tests come later.
- `GET /api/experiments/{id}/report/export` returns CSV: `level=animals`
  (default) one row per animal and test at each endpoint's timepoint and
  baseline with the review status and value, `level=groups` the group
  statistics of each endpoint at every timepoint. `excel=1` writes UTF-16 LE
  with tab separators. CSV text cells that start with `=`, `+`, `-`, `@`, tab or
  carriage return get a leading apostrophe. One endpoint reads at most 100,000
  measurements (`report.exportTooLarge` otherwise).

`GET /api/dashboard/summary` (`*:read`) counts experiments and tests by status,
queued, running and failed runs, results by review status and animals waiting
for a calibration of their arena, and lists the five latest published results
and the five soonest planned sessions with their animals.

## Local container run

From the repository root:

```sh
docker compose up -d db
docker compose --profile setup run --rm migrate
docker compose --profile setup run --rm setup
docker compose up --build -d backend jobs
curl --fail http://127.0.0.1:4000/api/ready
```

`jobs` runs the analysis scheduler against the same database.

`setup` prints the administrator email and generated password once to your
terminal; change the password after signing in. When stdout is not a terminal
(CI, jobs), setup refuses to start unless `-credentials-file PATH` names a new
file, which is created with mode 0600. The password is never written to logs.

```sh
curl -X POST http://127.0.0.1:4000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@misko.local","password":"PASSWORD_FROM_SETUP"}'
curl http://127.0.0.1:4000/api/auth/me -H 'Authorization: Bearer TOKEN'
```

The Compose project `misko` keeps the database on the `postgres-data` volume;
it does not mount the older `foundation-db` or `db-data` volumes, which are left
untouched. Videos are kept on the `local-videos` volume through
`LOCAL_STORAGE_DIR`. Credentials and `JWT_SECRET` in
`docker-compose.yml` are development-only and the API binds to loopback. No
frontend or GCP deployment is started. Re-running `migrate` applies only pending
migrations and does nothing on an up-to-date database. A `foundation-db` volume
installed with the migrations before the schema rebuild is refused; see
Migrations for the reset.

## Run Go directly

Use the version in `go.mod` and a dedicated empty PostgreSQL 18 database (the
schema uses `uuidv7()`). The installing user needs CREATE permission on the
database, because `migrate` creates the `misko` schema. Export the variables
below (see `.env.example`), then from `backend/`:

```sh
go run ./cmd/bootstrap migrate
go run ./cmd/bootstrap setup
go run ./cmd/api
go run ./cmd/jobs
```

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `DATABASE_URL` | required | all | PostgreSQL URL with host and database |
| `HTTP_ADDR` | `:4000` | api | Listen address |
| `PROBE_TIMEOUT` | `2s` | api | Readiness probe and connect timeout |
| `SHUTDOWN_TIMEOUT` | `10s` | api | Request drain period on SIGTERM |
| `JWT_SECRET` | required | api | HS256 key, at least 32 bytes |
| `JWT_ISSUER` / `JWT_AUDIENCE` | `misko` / `misko-api` | api | Required token claims |
| `TOKEN_TTL` | `12h` | api | Token lifetime, 5m to 168h |
| `BCRYPT_COST` | `12` | api, setup | bcrypt cost, 10 to 14 |
| `ADMIN_EMAIL` | required | setup | First administrator |
| `LAB_NAME` | required | setup | Laboratory name |
| `LAB_TIMEZONE` | `UTC` | setup | IANA time zone |
| `JOB_INTERVAL` | `10s` | jobs | Time between analysis scheduler ticks, at least 1s |

The processes do not load `.env` files or change the schema on startup. The API
reports not ready while the database is behind the newest migration it embeds.

## Migrations

`schema/NNN_name.sql` files are numbered from `001` without gaps. `bootstrap
migrate` takes an advisory lock, runs every file not yet in
`misko.schema_migrations` in one transaction and records each file's SHA-256, so
either all pending migrations apply or none do.

The schema was rebuilt from scratch for the study model: `001_foundation` to
`012_reports` replace every earlier migration, and there is no legacy adoption
any more.

Later files: `013_user_last_login`, `014_calibration_zones` (zone shapes drawn
with a calibration), `015_scoring` (observer scoring, applied in some
databases, never edited), `016_drop_scoring` (drops the scoring tables, the
`method` and `scored_by` run columns and their triggers, restores the
automated-only `analysis_runs_state_check` and `check_analysis_published` of
`011_analysis`, and adds a `label` to analysis events) and
`017_side_view_plane` (the `APPARATUS_SIDE` measurement plane),
`018_analysis_jobs` (analysis jobs with their snapshot and outputs, QC, the
experiment-side links, `subject_results.job_id`, the `test_subject_jobs` view
and the report views over jobs; every analysis run moves into a job with the
same id, its pinned calibration copied into the snapshot, and the run tables
are dropped) and `019_paradigm_trials` (paradigm trials and their job links,
calibrations owned by a trial, `trial_effective_calibrations`). The old API
reads the dropped run columns, so `016` gives a short error window between
`migrate` and `up`; `018` does the same for the run tables (workers keep their
leases because jobs keep the run ids).

- A database installed with the old migrations is refused: its recorded
  checksums no longer match (`an applied migration was changed`), and a `misko`
  schema without `misko.schema_migrations` is refused as not installed by these
  migrations. Nothing is changed in either case. Reset such a database before
  migrating: drop and recreate the database (with Compose,
  `docker compose down -v` removes the `foundation-db` volume), then run
  `migrate` and `setup` again. Earlier study data is not carried over.
- Add a change as the next numbered file. Never edit an applied file: the
  recorded checksum no longer matches and `migrate` refuses to run.
- Deployments run `migrate` with the new image before `up`, while the old API is
  still serving. Write migrations the old code survives (add first, remove in a
  later release) or accept a short error window. A database newer than the code
  is accepted, so a rollback can start.
- sqlc reads the same files, so `make generate` sees the schema after every
  migration.

Configuration errors never echo the database URL or signing secret. Shutdown
marks readiness unavailable, stops accepting requests, and drains them for
`SHUTDOWN_TIMEOUT`; on expiry connections are forced closed and the process fails.

## Verification

```sh
make check
TEST_DATABASE_URL='postgres://user:password@localhost/empty_db?sslmode=disable' make integration
make build
```

`make check` runs gofmt, `go vet` (with and without the integration tag),
`sqlc diff`, race-enabled unit/HTTP/architecture tests and govulncheck. After
editing schema or query SQL, run `make generate` and commit the result.

Integration tests need a PostgreSQL server where the user may create databases.
Schema tests create their own empty `misko_schema_*` databases and cover a
fresh install, a repeated run, a refused unknown schema, a changed migration and
the database rules (lifecycle, frozen design, allocation, test subjects,
immutability). HTTP tests create uniquely named `misko_test_*` databases and drop
only those. They cover concurrent last-administrator protection, concurrent
setup, the experiment lifecycle and start rules, allocation and randomisation,
cross-experiment references, dosing and the due schedule, per-kilogram dose
conversion, tests with several animals, SQL constraints, search escaping, stable
paging and the read/write RBAC matrix over HTTP.

CI runs these checks on PRs and on pushes to `main`, then builds the container
without pushing an image. Dependabot opens its updates against `main`, and its
pull requests are merged automatically once CI is green.

Known limits: login has no rate limiting yet, and there is no server-side logout
(a client signs out by discarding its token; password changes revoke all tokens).
Experiments cannot be deleted through the API, and allocations of a running
study are excluded, never deleted; research records are kept.

[OpenAPI](api/openapi.yaml) describes only implemented endpoints.
[Dependencies](DEPENDENCIES.md) records pinned versions and verification sources.
