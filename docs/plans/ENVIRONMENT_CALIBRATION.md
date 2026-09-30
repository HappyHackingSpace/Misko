# Environment calibration and drift check

Status: phase 1 (environment calibration with per-recording override) is implemented on
the `feat/environment-calibration` branch.
Phase 2 (automatic drift check) is specified here only as far as phase 1 must
leave room for it.

## Problem

Calibration is stored per recording. A lab that films 200 videos of one rig with
one fixed camera has to mark the same reference points 200 times. The camera and
arena rarely move, so the calibration is really a property of the physical setup,
which the model already calls the environment.

Making it environment-wide has one danger: if the camera or arena moves for a
single video, the stored transform silently produces wrong centimeters. The
design therefore has two halves:

1. Calibrate once per environment revision (this document, phase 1).
2. Detect drift per video and fall back to a manual per-video calibration when it
   is detected (phase 2). Until phase 2 exists, an environment calibration is
   trusted but visibly marked as unchecked.

The design applies to every paradigm that declares `max_calibration_error_cm` in
its QC contract, including paradigms added later. No paradigm-specific code is
added.

## Decisions

### D1. One table, two scopes

`misko.calibrations` gains a scope instead of a second table, because analysis
runs, reports, the dashboard and the worker payload already speak
`calibration_id`, and the worker needs one shape.

| column | change |
|---|---|
| `environment_revision_id uuid NOT NULL` | new. Recording calibrations carry the revision of their test too, so bounds and tolerance always come from the same revision. |
| `recording_id uuid` | now nullable. `NULL` means the environment default. |
| `reference_frame_us bigint` | nullable for environment scope (no video exists yet), required for recording scope. |
| `scope` | not stored; derived (`recording_id IS NULL` is ENVIRONMENT). |

Why revision and not environment: revisions hold the apparatus measurements
(arena size, tank diameter). Plane coordinates and bounds are only meaningful
against one revision, and tests already pin `environment_revision_id`. A new
revision means new geometry, so it needs a new calibration. This is the same
immutability rule the rest of the schema uses.

Chains stay linear and immutable:

- Environment chain: one per `environment_revision_id` among rows with
  `recording_id IS NULL`. Unique first key and unique `supersedes_id` as today.
- Recording chain: one per `recording_id`, unchanged.
- The supersedes foreign key is scoped so a chain never mixes scopes.

`analysis_runs_calibration_fkey` currently references
`calibrations (recording_id, id)`. It changes to `calibrations (id)`, and a
trigger (or a composite key on `environment_revision_id`) asserts that the
pinned calibration belongs to the run's recording or to the run's
`environment_revision_id`. Pinned runs stay immutable.

Backfill: existing rows get `environment_revision_id` from their recording's test.

### D2. One resolution rule, in one place

A single domain function decides the effective calibration of a recording. The
calibration status endpoint, the analysis scheduler and the dashboard all use it,
so they cannot disagree (the dashboard already documents this goal).

```
Effective(recordingChain, environmentChain):
  if recordingChain has a latest calibration:
      VALID    -> use it            (source RECORDING)
      REJECTED -> waiting           (a failed override never falls back silently)
  else if environmentChain latest is VALID:
      use it                        (source ENVIRONMENT)
  else waiting
```

A rejected override does not fall back to the environment. If someone entered a
manual calibration, they had a reason, and hiding a failure behind the default
would defeat the point.

`Requirement` (required or not, tolerance, bounds) is unchanged and still comes
from the paradigm catalog with the revision's apparatus. Paradigms without
`max_calibration_error_cm` (for example ROTAROD) stay `NOT_REQUIRED` and the
environment calibration endpoints answer `calibration.notRequired`.

### D3. Environment correction must not mass-reanalyze

Today a new calibration creates a new automatic run because the dedupe key
includes `calibration_id`. With an environment calibration, one correction would
requeue every recording of that revision, possibly hundreds, and replace results
researchers may already have published.

Rule: a calibration of ENVIRONMENT source never creates automatic runs for a
recording that already has an automatic run. Recording-scoped calibrations keep
today's behavior. Reanalysis after an environment correction stays an explicit
manual run (`POST .../analysis-runs`), which already pins the current inputs and
leaves earlier runs unchanged. `ReadyRecordings` and its dedupe change
accordingly.

### D4. Environment calibration has no video

Environment calibration is entered when the rig is set up, before any video
exists. The current form only takes numeric pixel points and a reference frame
time, so nothing in the calibration math needs a video. What is lost is the
reference image the points were read from, which phase 2 needs (drift is measured
against it). Phase 1 makes `reference_frame_us` nullable for environment scope and
adds nothing else; phase 2 adds an optional reference image asset to the
calibration row.

### D5. Permissions

- Environment calibration: `apparatus:write`, the same right that changes the
  rig's measurements.
- Recording override: `test:run`, unchanged.
- Reads: `*:read`.

## API

New (environment scope, addressed by revision number like existing revision routes):

| Method and path | Right | Notes |
|---|---|---|
| `POST /api/environments/{id}/revisions/{number}/calibrations` | `apparatus:write` | Same body as the recording calibration without `referenceFrameUs` being required. `supersedesId` required for corrections. |
| `GET /api/environments/{id}/revisions/{number}/calibrations` | `*:read` | Chain, oldest first. |
| `GET /api/environments/{id}/revisions/{number}/calibration-status` | `*:read` | `NOT_REQUIRED`, `WAITING_FOR_CALIBRATION` or `CALIBRATED` with the current calibration. |

Changed:

- `GET .../recordings/{recordingId}/calibration-status` returns the effective
  status plus `source` (`ENVIRONMENT` or `RECORDING`) and, from phase 2, `drift`.
  A recording of a calibrated environment is `CALIBRATED` without any recording
  calibration, as soon as its video is verified.
- `POST .../recordings/{recordingId}/calibrations` keeps its meaning and is now
  the manual override. Its correction rules are unchanged; the first override does
  not have to supersede the environment calibration. As built, an override is not
  required to use the same camera id and frame size as the environment's
  calibration: an override exists because this video's setup differs, so
  constraining it would defeat it. Only corrections within one chain keep the
  camera and frame size, as before.
- Analysis claim payload: unchanged. The worker still receives one calibration
  with its transform, so no worker change is needed in phase 1.

## Analysis, dashboard and reports

- `ReadyRecordings` and `GetCandidate` resolve the calibration with D2 in SQL:
  latest recording calibration if present, otherwise latest environment
  calibration of the test's revision.
- `RecordingsAwaitingCalibrationCheck` uses the same resolution; the
  `calibrationWaiting` counter then counts only recordings that truly wait.
- Reports keep `calibrationId`; nothing to change beyond the FK.

## Frontend

- Extract the calibration form from `TestDetail.vue` into a shared component so
  the environment page and the recording override use one implementation.
- Environment detail: a calibration section per revision (state, chain,
  create or correct). Paradigms that do not need calibration show nothing.
- `TestDetail.vue`: each recording shows its source badge (Environment or Manual
  override) and an "enter calibration for this video" action. In phase 2 a drift
  failure shows here with the measured deviation.
- Test creation is not blocked by a missing environment calibration (the gate is
  analysis, as today), but the environment page and the test page warn about it.
- Dashboard `calibrationWaiting` links keep working.

## Phase 2 hooks (not built now)

- A `calibration_drift_checks` table: recording, calibration, status
  (`PENDING`, `PASSED`, `FAILED`, `UNAVAILABLE`), measured deviation in cm, method
  and version, thresholds used.
- The check runs in a worker (the API never reads video bytes) and reports the
  measurement; the API decides pass or fail against the paradigm's
  `max_calibration_error_cm`. `UNAVAILABLE` (reference not visible) must behave
  as a failure that asks for a manual calibration, never as a pass.
- D2 gains one line: an environment calibration whose drift check FAILED or is
  UNAVAILABLE counts as waiting for that recording.
- Phase 1 exposes `drift: "UNCHECKED"` in the status so clients and the UI are
  already prepared.

## Notes from the implementation

- `misko.effective_calibrations` (a view in schema 013) implements D2 in SQL for
  the analysis scheduler and the dashboard; `calibration/domain.Effective` is the
  same rule for the status endpoints. A test checks that the two agree.
- A generated `chain_id` column (recording, else environment revision) keeps every
  supersedes chain inside one scope with a plain composite foreign key.
- Database triggers refuse a recording calibration of another revision than its
  test's, and an analysis run that pins a calibration of another recording or
  revision.
- Recording an environment calibration through the API needs `apparatus:write`, so
  a technician can override one video but cannot calibrate the rig.
- Schema files install into a fresh database only; an existing database needs to
  be recreated to pick up 013.

## Work breakdown (this branch)

1. Schema `013`: new columns, backfill, scoped keys, new FK and trigger; update
   `schema_test.go`.
2. Calibration domain: scope-aware chain rules, `Effective` resolution, tests for
   the full matrix (no calibration, env only, override valid, override rejected,
   env rejected, correction chains).
3. Calibration service, store and sqlc queries; environment routes and errors;
   integration tests including concurrent corrections.
4. Analysis and dashboard SQL, D3 dedupe, updated service and integration tests.
5. OpenAPI, README calibration section, this document's status.
6. Frontend: shared form, environment calibration section, recording source
   badge and override, i18n (TR and EN), e2e (`calibration.spec.js`).

## Open questions for the team

1. Reference image at environment calibration time: phase 1 skips it. Is it
   acceptable that drift detection needs it later, and who captures it?
2. Should an environment calibration be required before a test can be created
   for a paradigm that needs one, or only before analysis (this design)?
3. Marker on the rig (for example ArUco) or marker-free alignment for phase 2?
   This decides how reliable drift detection can be and needs real videos.
