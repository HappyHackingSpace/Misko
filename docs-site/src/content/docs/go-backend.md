---
title: Go backend (preview)
description: Install and use the Go backend, including sign-in, users, roles, the catalog, study design, dosing, test sessions, videos, calibration, analysis review and the study report.
---

:::caution
This page describes the unreleased Go backend. The Vue panel is still being
moved to this API, so the Installation and Usage pages still describe the
current release. Do not deploy it.
:::

## What exists today

| Area | Status |
|------|--------|
| Health and readiness probes | Implemented |
| Sign-in, current user, own password change | Implemented |
| User management with the five roles | Implemented |
| Laboratory settings (one laboratory per installation) | Implemented |
| Catalog of substances and disease models | Implemented |
| Subjects and body weights | Implemented |
| Experiments: groups with roles, regimens, timepoints, endpoints, allocation and exclusion | Implemented |
| Dosing: administrations, daily dosing schedule, induction outcomes | Implemented |
| Paradigm catalog with metric definitions for four paradigms | Implemented |
| Environments with measurement revisions and experiment test protocols | Implemented |
| Tests as sessions with one or more animals in arenas, trials and comments | Implemented |
| Video uploads to Google Cloud Storage, one current video per test | Implemented, live storage test pending |
| Calibration per arena with per-animal overrides | Implemented |
| Analysis runs per animal and human review of every result | Implemented for all four paradigms; the worker is tested on synthetic video only |
| Study report with role-based comparisons, CSV export, dashboard | Implemented |
| Vue panel on the new API | Scaffolded, not complete |

## Installation

You need Docker with Docker Compose. Run these commands from the repository root:

```bash
docker compose up -d db
docker compose --profile setup run --rm migrate
docker compose --profile setup run --rm setup
docker compose up --build -d backend jobs
curl --fail http://127.0.0.1:4000/api/ready
```

1. `db` starts PostgreSQL in a new volume. The old installation's data is not touched.
2. `migrate` creates the tables in an empty database, or applies the migrations an existing one is missing. On an up-to-date database it does nothing.
3. `setup` creates the laboratory and the first administrator, then prints the
   administrator's email and generated password **once** in your terminal. Run
   it again and it creates nothing.
4. `backend` starts the API on `127.0.0.1:4000`, and `jobs` queues analysis runs.

If `setup` runs without a terminal (for example in CI), it refuses to start
unless you pass `-credentials-file PATH`. The password then goes into that new
file, readable only by its owner. It is never written to logs.

If you use your own PostgreSQL server instead of the Compose database, it must be
PostgreSQL 18 or later. The user that runs `migrate` needs permission to create
a schema in the database.

### Databases from before the schema rebuild

The database schema was rebuilt from scratch for the study model described
below. `migrate` refuses a database that was installed with the earlier
migrations and changes nothing in it. Such a database must be reset: drop and
recreate it, or with Compose remove the volume with `docker compose down -v`,
then run `migrate` and `setup` again. Earlier data is not carried over.

### Settings

The Compose file contains development-only values. For any real installation,
set your own values:

| Variable | Default | Meaning |
|----------|---------|---------|
| `JWT_SECRET` | development value | Key that signs sign-in tokens. Use at least 32 random bytes. |
| `TOKEN_TTL` | `12h` | How long a sign-in lasts, from 5 minutes to 7 days. |
| `BCRYPT_COST` | `12` | Password hashing strength, from 10 to 14. |
| `ADMIN_EMAIL` | `admin@misko.local` | Email of the first administrator created by `setup`. |
| `LAB_NAME` | `Misko Laboratory` | Laboratory name. |
| `LAB_TIMEZONE` | `UTC` | Laboratory time zone, as an IANA name such as `Europe/Istanbul`. Day 0, dosing days and default test times use it. |

The backend README lists every variable, including database, storage and timeout settings.

## Usage

### Sign in

```bash
curl -X POST http://127.0.0.1:4000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@misko.local","password":"PASSWORD_FROM_SETUP"}'
```

The response contains a `token`. Send it with every other request as
`Authorization: Bearer TOKEN`. `GET /api/auth/me` returns your user and the
permissions of your role. Change the generated password right away with
`POST /api/auth/password` (`currentPassword`, `newPassword`). The response holds
a new token, and all older tokens stop working.

Passwords need at least 8 characters and at most 72 bytes. Letters outside
English (such as ş or ğ) take more than one byte each.

### Roles

| Role | Can do |
|------|--------|
| SUPERADMIN, LAB_MANAGER | Everything, including managing users, laboratory settings and analysis workers |
| RESEARCHER | Design studies, maintain the catalog, subjects, apparatus and protocols, plan and run tests; read everything |
| TECHNICIAN | Record weights, run tests, record doses and induction outcomes, calibrate one animal of a test, review analysis results; read everything |
| VIEWER | Read only |

| Permission | Covers |
|---|---|
| `study:write` | Catalog, experiment design and lifecycle, allocation and exclusion, protocols |
| `apparatus:write` | Environments, their revisions and arena calibrations |
| `subject:write` | Subjects |
| `weight:write` | Body weights |
| `test:write` | Planning tests: creating them, adding or removing animals, cancelling |
| `test:run` | Running tests and trials, videos, per-animal calibration overrides, doses, induction outcomes, manual analysis runs, result review |
| `*:read` | Reading everything |

A role change takes effect on the user's next request. Deleting a user signs
them out immediately.

### Manage users

Only SUPERADMIN and LAB_MANAGER can use these endpoints:

- `GET /api/users` lists users with `search`, `role`, `sort`, `order`, `page` and `pageSize`.
- `POST /api/users` creates a user with `email`, `name`, `role` and an optional
  `password`. Without a password, a strong one is generated and returned once.
- `PATCH /api/users/{id}` changes `name` or `role`.
- `POST /api/users/{id}/reset-password` sets a new password (or generates one)
  and signs that user out everywhere.
- `DELETE /api/users/{id}` deletes a user.

Two safety rules always apply. You cannot delete your own account, and the last
SUPERADMIN or LAB_MANAGER cannot be deleted or given a lower role. There is no
public sign-up.

### Laboratory settings

Every signed-in user can read `GET /api/lab`. SUPERADMIN and LAB_MANAGER can
change `name`, `code` and `timezone` with `PATCH /api/lab`. An empty `code`
clears it. `GET /api/meta` is public and returns the laboratory name for the
sign-in screen.

### Catalog

Substances and disease models are shared by every study of the laboratory.
Researchers and the roles above them maintain them; everyone can read them.

- `POST /api/catalog/substances` (`name`, `kind`, optional `description`).
  `kind` is `COMPOUND`, `REFERENCE_DRUG`, `VEHICLE` or `INDUCER`; it only helps
  pickers. What a substance does in a study comes from the regimen that uses it.
- `POST /api/catalog/disease-models` (`name`, `inductionType` of `CHEMICAL`,
  `SURGICAL`, `DIET`, `STRESS` or `OTHER`, optional `confirmationCriterion` and
  `description`). Every disease model is induced; the inducing agent and its
  dose are part of each study's regimen.
- Both lists accept `search`; substances filter by `kind` and disease models by
  `inductionType`. Change an entry with `PATCH`. An entry still in use cannot be
  deleted (`substance.inUse`, `diseaseModel.inUse`).

### Subjects and weights

Every signed-in user can read subjects. SUPERADMIN, LAB_MANAGER and RESEARCHER
can change them.

- `POST /api/subjects` creates a subject with `code`, `species` (`MOUSE` or
  `RAT`) and `sex` (`FEMALE`, `MALE` or `UNKNOWN`). Codes are unique regardless
  of upper or lower case.
- `GET /api/subjects` lists subjects with `search`, `species`, `sex`,
  `unallocated=true`, `sort`, `order`, `page` and `pageSize`. Each subject shows
  its `allocation` (experiment, group and whether it is excluded), or null.
- `PATCH /api/subjects/{id}` changes the fields you send.
- `DELETE /api/subjects/{id}` deletes a subject that is not allocated and has no weights.
- Technicians and the roles above them record body weight with
  `POST /api/subjects/{id}/weights` (`grams`, `measuredAt`). Weights belong to
  the animal and never change.

An animal can be registered before it joins a study, and it joins exactly one.

### Experiments

An experiment is designed as a draft, started, and completed. Every signed-in
user can read experiments. SUPERADMIN, LAB_MANAGER and RESEARCHER design and
start them and allocate animals.

1. Create the experiment with `POST /api/experiments` (`code`, `title`, optional
   `description`, `hypothesis`, `randomisation` and `blinded`). It starts as
   `DRAFT`.
2. Add groups with `POST /api/experiments/{id}/groups` (`code` such as `G1`,
   `name`, `role`, `diseaseModelId`, `targetSize`). The role is one of:

   | Role | Disease model | Receives | Purpose |
   |---|---|---|---|
   | `HEALTHY_CONTROL` | none | vehicle, if any | normal baseline |
   | `SHAM` | required | a vehicle in place of the induction | controls for the induction procedure |
   | `DISEASE_CONTROL` | required | induction and vehicle | the deficit |
   | `POSITIVE_CONTROL` | required | induction and a reference drug | proves the test can detect a rescue |
   | `TREATMENT` | required | induction and the tested substance | efficacy and dose response |

3. Describe what each group receives with
   `POST /api/experiments/{id}/regimen-items` (`groupId`, `purpose` of
   `INDUCTION`, `VEHICLE` or `TREATMENT`, `substanceId`, `amount`, `unit`,
   `route`, `frequency` of `ONCE`, `DAILY`, `TWICE_DAILY`, `WEEKLY` or
   `EVERY_N_DAYS` with `intervalDays`, `startDay`, `endDay`). Days are study
   days counted from day 0. A group has at most one `TREATMENT` item, so a
   treatment group has one substance at one dose. Several treatment groups of
   the same substance at different doses form the dose response.
4. Add timepoints with `POST /api/experiments/{id}/timepoints` (`name`,
   `studyDay`, `position`), for example Baseline on day -1 and Day 14 on day 14.
5. Add a protocol (see below), then choose what the study is judged on with
   `POST /api/experiments/{id}/endpoints` (`timepointId`, `paradigmKey`,
   `metricKey`, `kind` of `PRIMARY` or `SECONDARY`, `direction` of
   `HIGHER_IS_BETTER` or `LOWER_IS_BETTER`, optional `baselineTimepointId`).
   `GET /api/experiments/{id}/metric-options` lists the metrics the protocol's
   paradigms can measure. There is at most one primary endpoint.
6. Check the design with `GET /api/experiments/{id}/readiness`, which lists every
   unmet rule, then start with `POST /api/experiments/{id}/start` (`dayZero`, a
   date). Starting needs at least one timepoint and one group, a target size on
   every group, one primary endpoint, and a regimen that matches each role: a
   treatment or positive control group has one treatment and an induction, a
   disease control has an induction and a vehicle, a sham has a vehicle and no
   induction, a healthy control has no induction or treatment, and every sham,
   treatment and positive control group has a disease control with the same
   disease model.
7. When every test is completed or cancelled, close the study with
   `POST /api/experiments/{id}/complete`. A completed experiment is read-only.

Starting freezes groups, regimen items, timepoints and endpoints. The code,
title and description can still be edited until completion.

### Allocation and exclusion

- Put an animal in a group with `POST /api/experiments/{id}/allocations`
  (`subjectId`, `groupId`, optional `cage`), or let the system do it with
  `POST /api/experiments/{id}/allocations/randomise` (`subjectIds`, optional
  `groupIds` and `cage`). Randomisation fills the groups evenly up to their
  target sizes and allocates either every animal or none.
- An animal belongs to one group of one experiment, for good. While the
  experiment is a draft you can move it to another group or remove it. Once the
  study runs, only its cage can change.
- If an animal leaves the study, exclude it with
  `POST /api/experiments/{id}/allocations/{allocationId}/exclusion` (`reason`:
  `DIED`, `HUMANE_ENDPOINT`, `INDUCTION_FAILED`, `TECHNICAL` or `OTHER`, optional
  note). An excluded animal is never deleted: it stays visible in the report,
  joins no new test and is not analyzed again.
- `GET /api/experiments/{id}/allocations` lists the animals (filter by `groupId`
  or `excluded`) and each group's count against its target.

### Dosing

Technicians and the roles above them record what was actually given while the
experiment runs.

- `GET /api/experiments/{id}/dosing?date=YYYY-MM-DD` lists every dose due that
  day per animal, from the regimen items and day 0, with how many were already
  given and the dose for the animal's latest weight. Without `date` it uses
  today in the laboratory's time zone.
- `POST /api/experiments/{id}/allocations/{allocationId}/administrations`
  records one dose. With `regimenItemId` the substance, dose and route come from
  the plan and you may enter a different actual amount or route; an unplanned
  dose names them all. Doses per kilogram (`mg/kg`, `ug/kg`, `mL/kg`, `IU/kg`)
  need a weight of the same animal from the previous 7 days; the latest one is
  used unless you name `weightMeasurementId`, and the response shows the
  absolute dose. Disease induction is recorded the same way, with purpose
  `INDUCTION`.
- `POST /api/experiments/{id}/allocations/{allocationId}/induction-outcome`
  records once whether the animal's disease model was `CONFIRMED` or
  `NOT_CONFIRMED`.
- `GET /api/experiments/{id}/administrations`,
  `GET /api/subjects/{id}/administrations` and
  `GET /api/experiments/{id}/induction-outcomes` list the records.

Write amounts as decimal text, for example `"0.25"`. Records keep the names,
doses and weights they were saved with, and they never change.

### Paradigm catalog

Every signed-in user can read the paradigm catalog. Paradigms are defined in the
code and cannot be created, edited or deleted through the API.

- `GET /api/paradigms` lists paradigms with their published versions. Four
  paradigms are published: `MWM`, `NOVEL_OBJECT`, `POLE` and `TREADMILL`, in
  two categories (`LEARNING_MEMORY` for `MWM` and `NOVEL_OBJECT`, `MOTOR` for
  `POLE` and `TREADMILL`). Every paradigm is analysed automatically: an
  analysis worker tracks the subject on the video once a worker is registered
  for the paradigm version, Misko derives every event and metric, and a person
  only reviews the result. There is no observer scoring.
- `GET /api/paradigms/{key}` returns the latest version, and
  `GET /api/paradigms/{key}/versions/{version}` returns one exact version. Each
  version lists its parameters, each with its scope (`APPARATUS` and `ANALYSIS`
  on an environment, `SESSION` on a protocol step), label, help text, unit,
  allowed range and default; its zones and events; and for every metric a plain
  description, the definition, formula, inputs and what happens when data is
  missing. Names, labels and descriptions follow `?lang=` or
  `Accept-Language` (`en` or `tr`).

Every paradigm is measured from the tracked position of the subject and its
zones:

- The water maze derives its zones from the apparatus parameters (platform,
  target quadrant, wall zone). The other paradigms declare zones marked
  `calibrated`, drawn as circles or polygons with the calibration: the novel
  and the familiar object (`novel_object`, `familiar_object`), the pole and its
  base (`pole`, `base`) and the treadmill's shock grid (`shock_grid`).
- `events` lists what Misko derives from the positions and the zones:
  exploration bouts labelled `novel` or `familiar`, the pole turn, base and
  fall, treadmill shocks and exhaustion, water maze platform visits. Every rule
  threshold (minimum bout, turn descent, fall speed, time on the grid for a
  shock or for exhaustion) is an `ANALYSIS` parameter with a standard default
  that a lab can change per environment. `inputEvents` is always empty.

Results always name the paradigm version, the metric engine version and the
result schema version they were computed with. A metric that cannot be computed
is reported as missing with a reason, never as zero. For example, a probe trial
without an escape reports `EVENT_NOT_OBSERVED`, and a novel object trial whose
total exploration is below the minimum reports `BELOW_EXPLORATION_CRITERION`
for the discrimination index. A published version never
changes; a changed definition is published as a new version.

### Environments and protocols

An environment is one physical apparatus, such as a particular water tank or one
of several novel object boxes under one camera, set up for one paradigm. In a test
each animal is placed in an arena, which is one revision of an environment.
Researchers, lab managers and super admins create and revise environments and
protocols; every signed-in user can read them.

- `POST /api/environments` creates an environment with its first revision. Give
  every apparatus measurement of the paradigm version in `parameters`, for
  example the tank and platform sizes for `MWM`, and optionally its analysis
  settings; omitted analysis settings use their defaults. Values outside the
  allowed ranges, or inconsistent values such as a platform outside the tank,
  are rejected.
- When the apparatus changes, add a revision with
  `POST /api/environments/{id}/revisions`. Earlier revisions never change and
  cannot be deleted. You can still rename an environment or edit its notes.
  `GET /api/environment-revisions` lists every revision for arena pickers.

A protocol is the test battery of an experiment. It can combine paradigms, for
example a pole test followed by a treadmill run.

- `POST /api/experiments/{id}/protocols` creates a protocol with its first
  version. Each step gives its position, the paradigm and its version, the trial
  type, the number of trials, the pause between trials in seconds and any
  session parameters. A step names no apparatus: the arena is chosen per animal
  in each test.
- Number the steps from 1 without gaps. A trial type the paradigm does not define
  and session values out of range are rejected. Session parameters you leave out
  get the paradigm defaults, and the stored version shows them.
- To change a protocol, add a version with
  `POST /api/experiments/{id}/protocols/{protocolId}/versions`. Stored versions
  never change.

### Tests, trials and comments

A test is one session of a running experiment: one timepoint, one protocol step
and one video. It films one or more animals, each in its own arena; usually one,
sometimes up to three boxes under one camera. Animals of different groups may
share a session. The group of every measurement comes from the animal's
allocation.

- Researchers, lab managers and super admins plan tests with
  `POST /api/experiments/{id}/tests`: the timepoint, the protocol version and
  step, optionally the scheduled time, and `subjects`, a list of
  `allocationId` and `environmentRevisionId` pairs (up to 12). Without a time,
  the test is scheduled on day 0 plus the timepoint's study day. Every arena must
  run the step's paradigm, and an animal cannot be in two tests of the same
  timepoint and step.
- While a test is planned, add an animal with `POST /api/tests/{id}/subjects` or
  remove one with `DELETE /api/tests/{id}/subjects/{subjectId}`. The last animal
  cannot be removed; cancel the test instead.
- Technicians and the roles above them run tests: `POST /api/tests/{id}/start`,
  then record each animal's trials with
  `POST /api/tests/{id}/subjects/{subjectId}/trials`, then
  `POST /api/tests/{id}/complete`. Every animal needs at least one trial. If a
  trial has to be repeated, record the same repetition again; it gets the next
  attempt number and the earlier trial is kept.
- A test that has not been completed can be cancelled with a reason. A test with
  an excluded animal cannot start until that animal is removed.
- `GET /api/tests` is the worklist across experiments, filtered by experiment,
  status, timepoint and date range.

Everyone who is signed in, including viewers, can comment on a test. You can edit
only your own comments. Lab managers and super admins can delete any comment.
Each user can post or edit up to 20 comments per minute.

### Videos

Videos are stored in a private Google Cloud Storage bucket. Until an
administrator sets `GCS_BUCKET` (and, with workload identity, `GCS_SIGNER_EMAIL`),
video routes answer 503 with `media.storageNotConfigured`. See
`backend/README.md` for the bucket, CORS and permission setup.

A test has one video, which shows all of its animals. Videos can be attached
once the test has started.

- Technicians and the roles above them start an upload with
  `POST /api/tests/{id}/recordings`, giving the file name, type (MP4, MOV or
  WebM), size and CRC32C checksum. The response contains a signed request; the
  browser uses it to upload the file directly to storage.
- After the upload, the same person (or a lab manager or super admin) calls
  `POST /api/tests/{id}/recordings/{recordingId}/finalize`. The server checks the
  stored file. A file that does not match is marked `REJECTED` with the reason,
  such as `CHECKSUM_MISMATCH`.
- A new upload replaces the test's video. The earlier one is kept for
  provenance, and `GET /api/tests/{id}/recordings/current` returns the newest
  verified one.
- Everyone who is signed in can get a short-lived link to a verified video with
  `GET /api/tests/{id}/recordings/{recordingId}/read-url`. Players can seek in it.

### Calibration

Before a video can be turned into distances and times in zones, the system needs
to know how camera pixels map to centimeters in each arena.

- A calibration belongs to the arena. When the rig is set up, someone who can
  change apparatus opens a frame from the fixed camera and calibrates each
  environment revision once, with
  `POST /api/environments/{id}/revisions/{number}/calibrations`. Its crop is that
  arena's region of the frame, so three arenas under one camera each get their
  own calibration. Every animal tested in that arena then uses it.
- The camera and the arena can move. Nothing checks that yet: the status of an
  animal that uses its arena's calibration says `drift: UNCHECKED`. If a video
  does not match, a technician (or a role above) calibrates that one animal by
  hand with `POST /api/tests/{id}/subjects/{testSubjectId}/calibrations` on the
  test's current video. The override replaces the arena calibration for that
  animal only, and if it is rejected the animal waits instead of falling back to
  the arena. A replacement video starts from the arena calibrations again.
- Both kinds take the same points: at least 4 FIT points and at least 3 separate
  CHECK points, each with its pixel position and its real position in
  centimeters. The FIT points define the mapping and the CHECK points test it. If
  a CHECK point is off by more than the paradigm allows (2 cm), the calibration
  is stored as `REJECTED`. The measurement plane is `ARENA_FLOOR`,
  `WATER_SURFACE`, `APPARATUS_TOP`, or `APPARATUS_SIDE` for rigs filmed from the
  side (the pole and the treadmill lane).
- A paradigm with `calibrated` zones (novel object, pole, treadmill) also needs
  those zones drawn on the same frame. Send them as `zones` in pixels, for
  example `{"key": "base", "polygon": [{"x": 300, "y": 480}, ...]}` or
  `{"key": "novel_object", "circle": {"x": 960, "y": 540, "radius": 60}}`; a
  key may have several shapes. Without a required zone the calibration is
  refused (`calibration.missingZone`). A per animal override on the test video
  carries its own zones, for example when the novel object is on the other
  side. Every required zone needs a
  shape and every shape must lie inside the frame. Misko converts them to
  centimeters with the calibration and returns each zone in both. The worker
  receives them with its job, and the metrics use the centimeter shapes. In the
  panel the visual calibrator places the points, the crop and the zones on a
  frame of a video: for an environment default a video file chosen on the
  computer (read in the browser, never uploaded), for an override the test's
  current video.
- World points must use the frame the paradigm's zones are defined in. For the
  water maze that frame is tank-centred: (0, 0) is the tank centre, x points
  east and y north, so the wall points are (r, 0), (0, r), (-r, 0) and (0, -r)
  with r = tank diameter / 2. The novel object arena is seen from above with
  the origin at a corner of the floor, x to the right and y down the image, and
  its points lie within the arena. The pole is seen from the side with x
  horizontal and y the height above the base (base at 0, top at the pole
  length); the treadmill from the side with x along the lane from the shock
  grid (0) to the front. Use at least four non-collinear points of the rig.
- To fix a calibration, send a new one with `supersedesId` set to the latest
  calibration of the same arena or animal. Earlier calibrations are kept.
- `GET /api/tests/{id}/calibration-status` lists every animal of a test:
  `WAITING_FOR_VIDEO`, `WAITING_FOR_CALIBRATION` until a valid calibration
  with every required zone applies, then `CALIBRATED`. Paradigms without video measurements, such as the
  pole test, show `NOT_REQUIRED`.

### Video analysis

Tracking runs in separate analysis workers that talk to the API. The worker in
the `worker/` folder tracks the subject's centroid in recorded fixed-camera
video of every paradigm at version 1. It has only been tested on synthetic video so far;
do not use its results for research until it has been checked against a
manually annotated recording.

- A lab manager registers a worker with `POST /api/analysis/workers`, giving its
  name, model version and the paradigm versions it supports. Register the
  bundled worker with the paradigms it supports at version 1 (see
  [Installation](../installation/#analysis-worker)). Versions outside the
  catalog are refused with `worker.unknownCapability`. The
  response shows the worker's token only once; store it in the worker's secret
  configuration.
  `POST /api/analysis/workers/{id}/disable` revokes it. Start the worker with
  `MISKO_WORKER_TOKEN` set, for example
  `docker compose --profile worker up -d worker`.
- Every `JOB_INTERVAL` (10 seconds by default) the `jobs` process queues one run
  per animal of a test, when the experiment is running or completed, the animal
  is not excluded, the test's video is verified, the animal's calibration is
  valid with every required zone and a worker supports the paradigm.
- The worker finds the animal inside its arena's crop in every frame, converts
  its position to centimeters, and uploads the positions (the trajectory) and an
  annotated video. It never learns the animal's group or treatment. Misko
  computes every metric and event from the trajectory; a worker cannot send its
  own numbers. If too many frames are lost or the tracking confidence is too
  low, the run is `FAILED` with a `QC_FAILED` reason and nothing is published.
  A worker may mark samples as inferred (the optional `inferred` flag), for
  example while the animal is hidden: they count as tracked, but not in the
  confidence mean. The bundled worker sends none.
- A run records exactly what was analyzed: the video version, the clip, the
  calibration, the paradigm version and the parameters. To analyze an animal
  again, a technician (or a role above) calls
  `POST /api/analysis/test-subjects/{testSubjectId}/runs`. Earlier results stay.
- `GET /api/tests/{id}/analysis-runs` and `GET /api/analysis-runs/{runId}` show
  runs and their metrics and events. `GET /api/analysis-runs/{runId}/video-pair`
  returns short-lived links to the original and analyzed video with the offsets
  needed to play them in sync.

### Review

Every result is approved by a person before it counts. The first successful run
of an animal opens its result as `PENDING`.

- `GET /api/analysis/results` is the review queue (`status`, `experimentId`).
  In a blinded experiment it shows no group, disease, substance or dose.
- Technicians and the roles above them accept a result with
  `POST /api/analysis/results/{testSubjectId}/accept` or reject it with
  `POST .../reject` and a `reason` (`TRACKING_LOST`, `WRONG_ANIMAL`,
  `VIDEO_DEFECT`, `PROTOCOL_DEVIATION`, `OTHER`). `POST .../run` puts another
  successful run under review, and `POST /api/analysis/results/bulk-accept`
  accepts up to 100 at once.
- A newer run never replaces a reviewed result by itself. Once the experiment is
  completed, decisions cannot change.

### Study report and dashboard

Everyone who is signed in can read reports. They only read records.

- `GET /api/experiments/{id}/report` returns every endpoint, primary first. The
  unit is the animal: each animal contributes one value per timepoint, and only
  accepted results of animals that are not excluded count. Per group it shows n,
  mean, SD, SEM, minimum, maximum, change from baseline and how many animals are
  pending review, waiting for analysis, rejected or excluded.
- Comparisons follow the group roles: model validity (disease control against
  healthy control and sham), assay sensitivity (positive control against disease
  control), efficacy of each treatment group against its disease control with
  rescue %, and dose response for the treatment groups of one substance.
- Results from different paradigm or metric engine versions are never mixed; the
  report lists the versions and leaves the comparison out.
- `GET /api/experiments/{id}/report/export` downloads CSV, one row per animal
  (`level=animals`) or per group (`level=groups`); `excel=1` writes a file Excel
  opens directly.
- `GET /api/dashboard/summary` counts experiments, tests, runs and results by
  status and animals waiting for calibration, and lists the latest results and
  the next planned sessions.

### The panel

The Vue panel in `frontend/` is being moved to this API. Screens for
substances, disease models, the experiment tabs (design, allocation, dosing, tests, report) and the
review queue are scaffolded but not complete. Until they are, use the API
directly.

### Errors

Every error has a stable `code`, such as `auth.forbidden`,
`experiment.notReady` or `allocation.frozen`, and an English message. A 401
status means you are not signed in or your token is no longer valid. A 403 status
means your role does not allow the action. A 409 status means the record's state
does not allow the change, for example editing the design of a running
experiment.
