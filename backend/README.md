# Go backend

Integration branch `new-backend`. The rebuild is incomplete; do not release it.

| Issue | Implemented |
|---|---|
| [#124](https://github.com/HappyHackingSpace/Misko/issues/124) | Liveness/readiness, graceful shutdown, fresh schema installation, architecture checks |
| [#125](https://github.com/HappyHackingSpace/Misko/issues/125) | Login, current user, own password change, user management, laboratory singleton, RBAC, idempotent setup |
| [#126](https://github.com/HappyHackingSpace/Misko/issues/126) | Subjects, experiments, phases, groups, enrollments and dated group assignments |
| [#127](https://github.com/HappyHackingSpace/Misko/issues/127) | Disease models, substances, intervention plans, weights, subject conditions and actual administrations |
| [#128](https://github.com/HappyHackingSpace/Misko/issues/128) | Read-only versioned paradigm catalog and the OPEN_FIELD metric engine |
| [#129](https://github.com/HappyHackingSpace/Misko/issues/129) | The other ten paradigm contracts and metric engines, calibrated zones, scored events and pinned manifests |
| [#130](https://github.com/HappyHackingSpace/Misko/issues/130) | Environments with immutable measurement revisions and experiment test protocols with immutable versions |
| [#131](https://github.com/HappyHackingSpace/Misko/issues/131) | Tests of enrolled subjects on protocol steps, their lifecycle, appended trials and test comments |
| [#132](https://github.com/HappyHackingSpace/Misko/issues/132) | Video assets and test recordings in private Google Cloud Storage: signed resumable uploads, server-side verification, pinned read URLs (live GCS smoke test pending) |
| [#133](https://github.com/HappyHackingSpace/Misko/issues/133) | Versioned per-video calibration: normalized DLT homography from FIT points, independent CHECK validation, correction chains and the waiting-for-calibration status |
| [#134](https://github.com/HappyHackingSpace/Misko/issues/134) | Analysis workers, pinned analysis runs with leases and fencing attempts, the scheduler process, verified output publication and the original/analyzed video pair |
| [#137](https://github.com/HappyHackingSpace/Misko/issues/137) | Read-only metric and event reports with provenance, latest-run selection, CSV/JSON exports and subject-level group comparisons |

No computer vision worker ships with this repository yet. The Vue application is not wired to this API yet; its old
business routes return 404. There is no legacy API adapter or data migration.

## Layout and boundaries

- `cmd/api`: process entry, signals, exit status.
- `cmd/bootstrap`: `schema` installs the SQL schema into an empty database; `setup` creates the laboratory and first administrator.
- `cmd/jobs`: the analysis scheduler loop; it serves no HTTP.
- `internal/bootstrap`: composition root; constructor wiring of concrete adapters.
- `internal/access/domain`: the shared RBAC kernel (roles, permission matrix, authenticated actor).
- `internal/identity`: users, passwords, tokens and user administration.
- `internal/laboratory`: the laboratory singleton.
- `internal/subjects`: laboratory animals, independent of experiments.
- `internal/experiments`: experiments, measurement phases, groups, enrollments and group assignments.
- `internal/interventions`: disease models, substances, intervention plans, body weights, subject conditions and actual administrations.
- `internal/paradigms`: the hardcoded, versioned paradigm catalog and pure metric engine.
- `internal/environments`: physical apparatus and their immutable measurement revisions.
- `internal/protocols`: test protocols of an experiment with immutable versions of ordered paradigm steps.
- `internal/tests`: tests of enrolled subjects, their trials and comments.
- `internal/media`: video assets and test recordings; `adapters/gcs` is the only package that imports the Cloud Storage client.
- `internal/calibration`: per-video pixel-to-centimeter calibrations and the status that gates analysis.
- `internal/analysis`: analysis workers, runs, leases, result validation and publication of metrics, events, artifacts and video pairs.
- `internal/reports`: read-only reports, exports and group comparisons over the reporting views.
- `internal/health`: readiness use case and probes.
- `internal/platform`: configuration, HTTP server lifecycle, JSON helpers, PostgreSQL transaction and error helpers, the in-memory rate limiter and the integration-test database helper. No business rules.
- `schema`: embedded SQL files, applied in lexical order in one transaction; never run by the API.
- `sqlc.yaml`, `internal/*/adapters/postgres/queries.sql`: SQL sources for the generated `sqlcgen` packages. Do not edit generated code; run `make generate`.
- `tests/architecture`: source import checks with positive and negative policy fixtures.

Domain packages allow pure standard-library dependencies and their own
subpackages. Application packages add context/synchronization and their own
domain. `internal/access/domain` is the only domain package other domains may
import, so every use case authorizes against the same matrix. Adapters may use
any domain or application package but never another domain's adapters: other
HTTP adapters resolve the caller through `identityhttp.Authenticator`, a function
wired in the composition root. Relationships between domains, such as an
enrollment's subject, are enforced with foreign keys rather than cross-domain
imports. Environments and protocols check parameters through small `catalog`
adapters that call `internal/paradigms/domain`. `net/http`, `database/sql`, JWT, bcrypt, pgx and GCP packages cannot
enter inner layers. The architecture test scans every Go file under `internal`,
including inactive build tags.

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
| `*:read` | Yes | Yes | Yes | Yes | Yes |

| Route | Required access |
|---|---|
| `GET /api/health`, `GET /api/ready`, `GET /api/meta`, `POST /api/auth/login` | Public |
| `GET /api/auth/me`, `POST /api/auth/password` | Any signed-in user |
| `GET, POST /api/users`; `GET, PATCH, DELETE /api/users/{id}`; `POST /api/users/{id}/reset-password` | `user:manage` |
| `GET /api/lab` | `*:read` |
| `GET /api/paradigms`, `GET /api/paradigms/{key}`, `GET /api/paradigms/{key}/versions/{version}` | `*:read` |
| `PATCH /api/lab` | `lab:configure` |
| `GET /api/subjects`, `GET /api/subjects/{id}`, `GET /api/subjects/{id}/enrollments` | `*:read` |
| `POST /api/subjects`, `PATCH, DELETE /api/subjects/{id}` | `subject:write` |
| `GET /api/experiments`, `GET /api/experiments/{id}` and its `phases`, `groups`, `enrollments`, `enrollments/{enrollmentId}` | `*:read` |
| `POST, PATCH` experiments; `POST, PATCH, DELETE` phases and groups; `POST` enrollments and `enrollments/{enrollmentId}/assignments` | `study:write` |
| `GET /api/disease-models`, `/api/substances`, `/api/conditions`, `/api/administrations`; `GET /api/experiments/{id}/intervention-plans`; `GET /api/subjects/{id}/weights`, `conditions`, `administrations` | `*:read` |
| `POST, PATCH` disease models, substances and intervention plans | `study:write` |
| `POST /api/subjects/{id}/weights` | `weight:write` |
| `POST /api/subjects/{id}/conditions`, `POST /api/experiments/{id}/enrollments/{enrollmentId}/administrations` | `test:run` |
| `GET /api/environments`, `GET /api/environments/{id}`, its `revisions` and `revisions/{number}`; `GET /api/experiments/{id}/protocols`, `protocols/{protocolId}`, its `versions` and `versions/{number}` | `*:read` |
| `POST, PATCH` environments; `POST` environment revisions; `POST, PATCH` protocols; `POST` protocol versions | `apparatus:write` |
| `GET /api/experiments/{id}/tests`, `GET /api/tests/{id}`, `GET /api/tests/{id}/trials` | `*:read` |
| `POST /api/experiments/{id}/tests`, `POST /api/tests/{id}/cancel` | `test:write` |
| `POST /api/tests/{id}/start`, `POST /api/tests/{id}/complete`, `POST /api/tests/{id}/trials` | `test:run` |
| `GET, POST /api/tests/{id}/comments` | Any signed-in user |
| `PATCH /api/tests/{id}/comments/{commentId}` | The comment's author |
| `DELETE /api/tests/{id}/comments/{commentId}` | The comment's author, SUPERADMIN or LAB_MANAGER |
| `GET /api/tests/{id}/recordings`, `GET /api/tests/{id}/recordings/{recordingId}/read-url` | `*:read` |
| `POST /api/tests/{id}/recordings` | `test:run` |
| `POST /api/tests/{id}/recordings/{recordingId}/finalize` | `test:run`, and the uploader, SUPERADMIN or LAB_MANAGER |
| `GET /api/tests/{id}/recordings/{recordingId}/calibrations`, `GET .../calibration-status` | `*:read` |
| `POST /api/tests/{id}/recordings/{recordingId}/calibrations` | `test:run` |

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
- Passwords need at least 8 characters and at most 72 UTF-8 bytes (the bcrypt
  limit; longer input is rejected, never truncated). They are compared byte for
  byte without normalization. Generated passwords are 26 random base32 characters.
- Unknown accounts and wrong passwords return the same 401 after a bcrypt comparison.
- There is no signup route. Setup creates an administrator only when no
  privileged user exists, so repeated runs create nothing.
- Unexpected errors log the route pattern and error only, never headers, bodies,
  tokens or passwords. Clients receive a stable `code` and a generic message.

## Subjects and experiments

A subject's identity does not depend on any experiment; one subject can be
enrolled in many experiments and later tested many times.

- **Subject**: code (unique ignoring case), species (`MOUSE`, `RAT`), sex
  (`FEMALE`, `MALE`, `UNKNOWN`), optional strain, birth date (not in the future)
  and notes. A subject cannot be deleted while it is enrolled.
- **Experiment**: code (unique ignoring case), title, description and
  `requiresControl`. The experiment detail reports `controlRequirementMet`.
- **Phase**: an ordered measurement period such as a healthy baseline. Name
  (ignoring case) and position are unique within the experiment. A phase never
  implies a group.
- **Group**: an arm with role `CONTROL` or `TREATMENT` and an optional
  `targetSize`. The group list returns `activeSubjects`, the assignments active
  at the database clock, so target and actual counts can be compared. A group
  with assignments cannot be deleted.
- **Enrollment**: one per subject per experiment, with `enrolledAt`. An initial
  `groupId` may be given; the assignment then starts at enrollment.
- **Group assignment**: a dated period `[validFrom, validTo)`. Moving a subject
  closes its open assignment at `effectiveFrom` and opens a new one, so crossover
  keeps full history. A new assignment must start after the current one and not
  before enrollment, and cannot repeat the current group. A future assignment is
  stored but does not change today's counts.

The database enforces these relationships as well: composite foreign keys keep
an assignment's enrollment and group in the same experiment, a GiST exclusion
constraint forbids overlapping periods for one enrollment, and a row lock on the
enrollment serializes concurrent assignments. Routes under
`/api/experiments/{id}` return 404 for phases, groups and enrollments of another
experiment. Lists are paginated with `page` and `pageSize` (default 10, maximum
100): subjects filter by `search`, `species`, `sex` and sort by `code`,
`species`, `sex`, `birthDate` or `createdAt`; experiments filter by `search` and
sort by `code`, `title` or `createdAt`; enrollments filter by `subjectId` and
current `groupId` and sort by `enrolledAt`.

## Interventions

A plan is not an administration, and induction is not a confirmed disease.

- **Disease model** and **substance**: named catalog entries, unique ignoring case.
- **Intervention plan**: what a group of an experiment is meant to receive
  (substance, dose, route, schedule and optional phase). Creating or changing a
  plan never records an administration.
- **Weight measurement**: a subject's body weight in grams, with up to 3
  decimals and at most 5000 g.
- **Subject condition**: an append-only observation of a disease model with
  status `INDUCED`, `CONFIRMED`, `NOT_CONFIRMED` or `RESOLVED`, optionally linked
  to one of the same subject's enrollments. The current status per model is the
  latest observation; `GET /api/conditions?current=true` filters on it.
- **Administration**: an actual administration recorded against an enrollment,
  with substance, amount, unit, route and time. It cannot precede enrollment or
  lie in the future (5 minutes of clock skew are tolerated). Per-kilogram units
  (`mg/kg`, `ug/kg`, `IU/kg`) need a weight of the same subject measured within
  the 7 days before, and the response includes `absoluteDose` (dose times body
  mass, rounded half up to 6 decimals). A linked plan must belong to the same
  experiment, use the same substance and match the subject's group at that time.

Amounts are decimal strings with up to 6 decimals and are stored as exact
integers. Doses and dates are never inferred from plans. Records copy the
substance or disease model name, dose, route and body weight when saved, so
renaming a definition or editing a plan does not change history. Units: `mg`,
`ug`, `g`, `mL`, `uL`, `IU`, `mg/kg`, `ug/kg`, `IU/kg`. Routes: `ORAL`,
`INTRAPERITONEAL`, `SUBCUTANEOUS`, `INTRAVENOUS`, `INTRAMUSCULAR`, `INTRANASAL`,
`INTRACEREBROVENTRICULAR`, `TOPICAL`, `INHALATION`. Composite foreign keys tie
an administration's subject to its enrollment and its weight to the same
subject, and a check constraint requires a weight for per-kilogram doses.
Phases and groups referenced by plans cannot be deleted. Weights, conditions and
administrations cannot be edited or deleted yet.

## Paradigm catalog and metric engine

Paradigm, metric, unit, event and calculation definitions are Go code in
`internal/paradigms/domain`. There are no create, update or delete routes; a
change means a new published version, and callers only receive copies.

- `ParadigmVersion` identifies one paradigm's contract, `MetricEngineVersion`
  the calculations and `ResultSchemaVersion` the shape of a result. Every result
  and manifest carries all three.
- `GET /api/paradigms/{key}/versions/{version}` is the manifest a worker
  consumes: parameters with units, ranges, defaults and value type, zones
  (derived from parameters or calibrated per video), the scored input events the
  engine consumes, the interval and point events it derives, metrics with
  definition, formula, inputs, missing-data rule and tolerance, and QC rules.
- `automatedAnalysis` stays false until a real worker capability is registered;
  catalog presence does not mean a video can be analyzed.

OPEN_FIELD version 1 evaluates a trajectory of samples (time in microseconds,
x and y in arena-local centimeters, tracked or lost). A counted interval is a
pair of consecutive samples that are both tracked and inside the arena with
0 < dt <= `max_sample_gap_s`.

| Metric | Calculation |
|---|---|
| `duration_s` | Sum of dt over counted intervals |
| `distance_cm` | Sum of straight-line steps over counted intervals; lost frames are not interpolated |
| `mean_speed_cm_s` | `distance_cm / duration_s` |
| `center_time_s`, `periphery_time_s` | Counted time attributed to the zone of the earlier sample; the center rectangle includes its boundary |
| `center_time_ratio`, `periphery_time_ratio` | Zone time divided by `duration_s` |
| `center_entries_count` | Counted intervals moving from periphery to center; starting in the center is not an entry |
| `immobility_s` | Contiguous intervals with speed strictly below `immobility_threshold_cm_s`, in bouts of at least `min_immobility_bout_s` |

Samples outside the arena count as lost. With no counted interval, every metric
is reported missing with reason `NO_VALID_INTERVALS`, never as zero. Tracked
samples need finite coordinates, timestamps must be non-negative and strictly
increasing, and unknown or out-of-range parameters are rejected. The engine also
emits `in_center` and `immobile` intervals and `center_entry` points for the
event timeline. Tolerances are 1e-6 in the metric unit for times, distances and
speeds, and 1e-9 for ratios and counts; booleans use 1e-9 too.

### The other ten paradigms (version 1)

Every paradigm takes the same input: parameters, samples, calibrated zone
shapes, scored observed events, the event types that were scored, and the
recorded duration. Calibrated zones are circles or polygons with at least three
vertices, in the same centimeter coordinates as the samples, and include their
boundary; each shape of a multi-shape zone counts as the same zone. Unknown,
missing required or malformed zones are rejected. An observed event needs a
declared and scored type, the declared point or interval shape, an allowed
label, and must end within the recorded duration; single events occur at most
once. Trajectory paradigms use the OPEN_FIELD counted-interval rule: time and
zone membership come from the earlier sample, an entry is a counted interval
whose earlier sample is outside the zone and whose later sample is inside, and
latency is the time of the first tracked sample inside the zone.

| Paradigm | Input | Metrics |
|---|---|---|
| `MWM` | Trajectory in tank-centered centimeters, x east and y north; platform, target quadrant and wall annulus derived from parameters; samples outside the tank are lost | escape latency (start of the first platform dwell of at least `min_platform_dwell_s`), path length and mean swim speed up to the escape, target quadrant and thigmotaxis time ratios, platform crossings, time-weighted mean distance to the platform |
| `EPM` | Trajectory; calibrated `center`, `open_arms`, `closed_arms`; scored `risk_assessment` points | distance, zone times and ratios, open and closed arm entries, latency to open arm, risk assessment count |
| `Y_MAZE` | Trajectory; calibrated `arm_a`, `arm_b`, `arm_c`; optional `novel_arm` (1 to 3) | total arm entries (the start arm is the first entry), spontaneous alternation ratio, novel arm time ratio |
| `BARNES_MAZE` | Trajectory; calibrated `target_hole` and `holes` with one shape per hole | primary latency, primary errors (hole entries before the target), total errors |
| `THREE_CHAMBER` | Trajectory; calibrated chambers, optional `social_interaction` and `object_interaction` | chamber times, sociability index `(social - object) / (social + object)`, interaction times, chamber entries |
| `LIGHT_DARK` | Box-local trajectory split at `box_width_cm x light_fraction`; the dividing line belongs to the dark side | light and dark time, light time ratio, light entries, transitions, latency to dark |
| `ROTAROD` | Scored single `fall` point; fixed or accelerating rod | latency to fall (censored at `max_trial_duration_s`), fall detected, rpm at fall |
| `POLE` | Scored single `turn_complete`, `base_reached` and `fall` points | time to turn, total time, descent time and speed, fall detected |
| `TREADMILL` | Scored single `exhaustion` and repeated `shock` points; fixed or accelerating belt | latency to exhaustion, run time, run distance (integral of the belt speed), shocks strictly before the run ends |
| `NOVEL_OBJECT` | Scored `exploration` intervals labeled `novel` or `familiar` | novel, familiar and total exploration (overlaps count once), discrimination index |

Missing metrics carry a reason instead of zero: `NO_VALID_INTERVALS`,
`EVENT_NOT_OBSERVED` (for example no escape in a probe trial),
`NOT_SCORED` (the event type was not scored), `ZERO_DENOMINATOR`,
`INSUFFICIENT_ENTRIES` (fewer than three Y maze entries), `INCOMPLETE_RECORDING`
(no event and a recording shorter than the cut-off), `NOT_APPLICABLE` (no novel
arm), `ZONE_NOT_PROVIDED` (an optional zone was not calibrated) and
`BELOW_EXPLORATION_CRITERION`. Rules across parameters reject a platform outside
the tank, a wall annulus as wide as the tank radius, and end speeds below start
speeds.

Every published manifest is pinned to a golden file in
`internal/paradigms/adapters/http/testdata/manifests`, and the test fails when a
published contract changes or disappears. Change a contract by publishing a new
version; `go test ./internal/paradigms/adapters/http -update` only writes
goldens for versions that do not have one yet.

## Environments and protocols

An environment is a physical apparatus set up for one paradigm, such as a
particular water tank. Its measurements are stored as numbered revisions.

- Creating an environment stores revision 1; `POST /api/environments/{id}/revisions`
  adds the next number. The environment row is locked while a revision is
  numbered, so concurrent revisions get consecutive numbers.
- A revision gives every apparatus parameter of its paradigm version, because no
  physical measurement is defaulted. Values must be within range and satisfy the
  version's rules across parameters, such as an MWM platform inside the tank.
- Revisions are immutable: a trigger rejects `UPDATE` and `DELETE`, and a
  composite foreign key stops the environment's paradigm from changing. Only an
  environment's name and notes can be edited.

A protocol belongs to one experiment and has numbered versions. A version is an
ordered list of 1 to 50 steps, and the steps may use different paradigms.

- A step names a paradigm key and version, an environment revision of the same
  paradigm and version, a trial type defined by that version, 1 to 1000 trials,
  an inter-trial interval of 0 to 86400 seconds and session parameter values.
  Positions run from 1 without gaps or duplicates.
- Session values are validated together with the revision's apparatus values and
  stored with defaults filled in.
- Versions and steps are immutable. Triggers reject `UPDATE` and `DELETE`, a
  composite foreign key keeps each step's paradigm and version equal to its
  revision's, and a deferred constraint trigger rejects steps added to a version
  after it was committed.
- A new environment revision never changes a version that references an earlier
  revision; publish a new protocol version to use it. Tests will reference a
  fixed protocol version and, through its steps, fixed environment revisions.
- Protocol routes are scoped to their experiment, so another experiment's
  protocol id returns 404.

## Tests, trials and comments

A test is one session of an enrolled subject on one step of a protocol version
in the same experiment. The step fixes the paradigm, its version, the
environment revision and the planned number of trials.

- `POST /api/experiments/{id}/tests` takes an enrollment, an optional phase, a
  protocol version, a step position and the scheduled time. The subject comes
  from the enrollment and the group is the one assigned at the scheduled time.
  An enrollment, phase or protocol version of another experiment, a missing step
  or a time before the enrollment is rejected, and composite foreign keys
  enforce the same combinations in the schema.
- Status moves only forward: `PLANNED` to `IN_PROGRESS` (start) or `CANCELLED`,
  and `IN_PROGRESS` to `COMPLETED` or `CANCELLED`. Transitions lock the test row,
  so concurrent requests get one success and `test.invalidTransition` (409) for
  the rest. A trigger rejects other status changes, edits of the context columns
  and deletes.
- Starting locks the enrollment and requires the subject's group at the start
  time to equal the planned group; otherwise the API returns `test.groupChanged`
  and the test should be cancelled and planned again.
- Trials are appended while a test is in progress. `repetition` runs from 1 to
  the planned trials; recording a repetition again adds the next `attempt` and
  never replaces an earlier trial. Trials are immutable, and completing a test
  needs at least one trial and a time after its start and every trial.
- Test status is the session's lifecycle only. Analysis runs get their own
  status in a later issue.

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

1. `POST /api/tests/{id}/recordings` declares file name, content type
   (`video/mp4`, `video/quicktime` or `video/webm`), size, base64 CRC32C and the
   clip range, and stores a `PENDING` video with a new object name
   `tests/{testId}/originals/{videoId}`. The response carries a V4-signed `POST`
   with `x-goog-resumable: start` and `x-goog-if-generation-match: 0`, so the
   upload cannot overwrite an existing object. The client sends it, uploads to
   the returned session URI and keeps that URI to itself.
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
  `[{"origin":["https://UI_ORIGIN"],"method":["GET","HEAD","POST","PUT","OPTIONS"],"responseHeader":["Content-Type","Content-Range","Range","Location","ETag","x-goog-resumable"],"maxAgeSeconds":3600}]`.
- Application Default Credentials for the API (workload identity on GCP; no
  service account JSON in the repository). The identity needs object create and
  get permissions on the bucket, for example `roles/storage.objectUser`. When the
  credentials cannot sign (workload identity), set `GCS_SIGNER_EMAIL` to a service
  account that grants the API `roles/iam.serviceAccountTokenCreator` so URLs are
  signed through the IAM Credentials API.

| Variable | Default | Meaning |
|---|---|---|
| `GCS_BUCKET` | empty | Bucket name; empty makes video routes return 503 `media.storageNotConfigured` |
| `GCS_SIGNER_EMAIL` | empty | Service account used to sign URLs through IAM |
| `VIDEO_MAX_BYTES` | 21474836480 | Largest accepted video, 1 MiB to 5 TiB |
| `UPLOAD_URL_TTL` | 1h | Validity of the signed upload start, 1m to 168h |
| `READ_URL_TTL` | 15m | Validity of read URLs, 1m to 12h |

The unit and integration tests use a fake object store and an offline signer;
they do not prove real GCS behavior. The live smoke test uploads a 3 MiB object
under `smoke/` with a resumable session, checks CRC32C, the overwrite
precondition, a range read, CORS and URL expiry, and deletes nothing:

```sh
GCS_LIVE_BUCKET=BUCKET GCS_LIVE_ORIGIN=https://UI_ORIGIN go test -tags=gcslive -run TestLive ./internal/media/adapters/gcs/
```

## Calibration

Physical metrics need a validated calibration of the video. Paradigms whose QC
contract has `max_calibration_error_cm` (the trajectory paradigms) require one;
observation-only paradigms such as ROTAROD report `NOT_REQUIRED`.

- `POST /api/tests/{id}/recordings/{recordingId}/calibrations` (`test:run`) takes
  the camera id, frame size, crop, the video time of the reference frame, the
  measurement plane (`ARENA_FLOOR`, `WATER_SURFACE`, `APPARATUS_TOP`), 4 to 100
  FIT points and 3 to 100 CHECK points, each a full-frame pixel with plane
  coordinates in centimeters.
- The transform is a homography fitted with the normalized direct linear
  transform: both point sets are centered and scaled, the 8 unknowns are solved by
  least squares with partial pivoting, and a negligible pivot (collinear or
  repeated points) is rejected as `calibration.degenerateFit`. The algorithm
  version `homography-dlt-normalized-v1` is stored with the result.
- Points must lie inside the crop, the crop inside the frame, CHECK points at
  least 1 pixel away from every FIT point, and, for paradigms whose zones come
  from parameters (OPEN_FIELD, LIGHT_DARK, MWM), plane coordinates inside the
  environment revision's geometry plus the tolerance.
- The stored calibration is `VALID` when the largest CHECK error is within the
  paradigm's `max_calibration_error_cm`, otherwise `REJECTED` with
  `EXCESSIVE_CHECK_ERROR`. FIT error alone never validates a calibration.
- Calibrations are immutable. A correction names `supersedesId`, which must be
  the recording's latest calibration, and keeps the camera id and frame size;
  the crop may change. The recording row is locked, a unique index allows one
  first calibration, and a unique `supersedes_id` keeps one linear chain.
- `GET .../calibration-status` returns `WAITING_FOR_CALIBRATION` until the video
  is verified and its latest calibration is `VALID`, then `CALIBRATED` with that
  calibration. Analysis runs reference the calibration id, so older runs keep
  their inputs after a correction.

## Analysis runs

Computer vision runs outside the API in analysis workers. The API schedules runs,
leases them to workers, verifies what they upload and publishes it; it never
reads video bytes. Workers measure positions only: the Go metric engine computes
every published metric and event from the stored trajectory. The first worker,
for single-subject Open Field video and tested only on synthetic video so far,
is in [`worker/`](../worker/README.md).

Workers are service identities, not users:

- `POST /api/analysis/workers` (`lab:configure`) registers a name, model version
  and capabilities (paradigm key and version from the catalog). A capability must
  be computable from a trajectory alone: versions that need scored observations
  or calibrated zone shapes return `worker.unknownCapability`, because catalog
  presence is not analysis support. The response
  shows the token (`mw_...`) once; only its SHA-256 hash is stored.
  `GET /api/analysis/workers` lists workers and
  `POST /api/analysis/workers/{id}/disable` revokes a token; its running lease
  then expires normally. `GET /api/analysis/capabilities` (`*:read`) lists what
  enabled workers can analyze.
- Worker routes under `/api/worker` accept only `Authorization: Worker TOKEN`.
  A user bearer token there returns 401 `worker.unauthenticated`, and a worker
  token on a user route returns 401 `auth.unauthenticated`.

A run pins its inputs when it is created: test, recording, source video and its
GCS generation and CRC32C, clip range, calibration (only for paradigms that
require one), paradigm version with its metric engine and result schema
versions, environment revision, protocol version and the merged apparatus and
session parameters. The inputs never change; reanalysis is a new run.

- `cmd/jobs` ticks every `JOB_INTERVAL`. A tick first returns runs whose lease
  expired to the queue, or fails them with `LEASE_EXPIRED` after the last
  attempt, then creates one `AUTOMATIC` run for every recording whose video is
  `VERIFIED`, whose latest calibration is `VALID` when the paradigm requires it,
  and whose paradigm version an enabled worker supports. A partial unique index on
  recording, source generation, paradigm version and calibration makes concurrent
  ticks create each automatic run once; a new calibration creates a new run.
- `POST /api/tests/{id}/recordings/{recordingId}/analysis-runs` (`test:run`)
  queues a `MANUAL` run with the current inputs, or returns 409
  `analysis.notReady`. Earlier runs and their results stay unchanged.
- `POST /api/worker/claim` leases the oldest available queued run the worker can
  analyze (`FOR UPDATE SKIP LOCKED`), increments `attempt` and returns the run,
  its output prefix `runs/{runId}/attempts/{attempt}/`, the trajectory schema
  `misko.trajectory.v1`, the contract (the metrics, event types, missing reasons
  and QC rules the engine will apply) and the pinned calibration (frame size, crop,
  plane and the pixel to centimeter homography, or null). With
  nothing to do it returns 204. A lease lasts 5 minutes;
  `.../runs/{runId}/heartbeat` extends it. A run has 3 attempts.
- The attempt is a fencing token. Heartbeat, `source-url`, `outputs`, `result`
  and `failure` take the attempt and return 409 `analysis.staleAttempt` unless the
  caller holds the current lease. Database triggers accept result rows only for
  the current attempt of a `RUNNING` run, move the status and attempt only
  forward, and keep finished runs and published rows unchanged.
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
  run id, attempt and recording duration, equal-length `tUs`, `xCm`, `yCm`,
  `tracked` and `confidence` arrays, strictly increasing times within the
  recording, confidences from 0 to 1 and coordinates for tracked samples
  (`analysis.invalidTrajectory`). QC rules the trajectory can measure are
  applied: `max_lost_frame_ratio` against untracked samples and
  `min_tracking_confidence` against the mean confidence of tracked samples. A
  failure marks the run `FAILED` with a `QC_FAILED` reason, without retry, and
  publishes nothing.
- The Go metric engine then computes the metrics and events from the samples
  with the run's pinned parameters and duration; an event's confidence is the
  mean confidence of the tracked samples it covers. Metrics, events, artifacts,
  the analyzed video asset and the pair are written with the `SUCCEEDED` status
  in one transaction; a deferred trigger refuses success without the pair.
  Delivering the accepted result again returns the run.
- `.../failure` records a reason. A retryable failure returns the run to the
  queue while attempts remain; otherwise the run is `FAILED`.

Users read the results with `*:read`: `GET /api/tests/{id}/analysis-runs`,
`GET /api/analysis-runs/{runId}` (with the published result when it succeeded)
and `GET /api/analysis-runs/{runId}/video-pair`, which signs reads of the pinned
original generation and that run's own analyzed video, with the time offsets and
mapping version for synchronized playback.

The integration tests cover concurrent schedulers and claims, lease expiry and a
late attempt, missing and corrupt outputs, duplicate delivery, reanalysis next to
earlier results and the database triggers, with a fake object store. They do not
prove real GCS behavior or any tracking accuracy.

## Reports and exports

Reports read published results through the read-only views of
`schema/012_reports.sql` (`report_runs`, `report_tests`); they never write, and
the views reject writes. Each domain keeps its own list endpoints; reports join
them for questions that start anywhere. Every role has `*:read`, and there is one
laboratory per installation.

- `GET /api/reports/metrics` returns metric results with their provenance:
  experiment, subject, phase and group stored on the test, test and its trial
  count, paradigm version, environment revision, recording, source video, run,
  trigger, model version, metric engine and result schema versions, and
  calibration. Filters: `experimentId`, `subjectId`, `groupId`, `phaseId`,
  `testId`, `environmentId`, `videoId`, `runId`, `diseaseModelId` (subjects with
  any recorded condition of that model), `substanceId` (subjects that received it
  in the test's experiment), `paradigmKey`, `paradigmVersion`,
  `metricEngineVersion` and `metricKey`. Sorts are allowlisted and end with test,
  run and metric keys, so pages are stable; `pageSize` defaults to 20 and is
  capped at 100.
- `selection=latest` (default) keeps the newest succeeded run of each test for
  each metric engine version, so a reanalysis never counts a test twice.
  `selection=all` lists every succeeded run, and `runId` reports that run even
  when it is superseded.
- A missing result keeps `value: null` with its `missingReason`; it is never
  turned into zero, and it sorts last.
- `GET /api/reports/events` lists published events with the same provenance plus
  the trial number, repetition and attempt; it filters by `eventType` instead of
  `metricKey`.
- `.../metrics/export` and `.../events/export` return every matching row as
  `format=csv` (default) or `format=json`, in report order, up to 100,000 rows
  (`report.exportTooLarge` otherwise). CSV text cells that start with `=`, `+`,
  `-`, `@`, tab or carriage return get a leading apostrophe so spreadsheets do not
  run them as formulas.
- `GET /api/reports/metric-summary?experimentId=...&metricKey=...` compares groups
  using the latest runs. Each subject contributes one value, the mean of its
  tests, because repeated tests and trials of one animal are not independent; `n`
  is the number of subjects with a value, and the summary reports mean, sample
  SD, SEM, minimum, maximum, subjects without any value and tests per missing
  reason. When the rows mix paradigm versions, metric engine versions or units,
  it returns 409 `report.incompatibleVersions` naming them; add `paradigmVersion`
  and `metricEngineVersion`.

The indexes added with the views cover the latest-run check and the group, phase,
environment and source video filters. An integration test generates 50
experiments, 2,000 subjects, 10,000 tests and trials, and 10,400 published runs
with 31,200 metric results and 20,800 events through the normal triggers, runs
`ANALYZE`, and explains the filtered statements the store actually sends. With
sequential scans disabled, every one of them reaches runs, results, events,
tests, recordings, videos and trials through index conditions (a full index scan
fails the test); the latest-run check uses `analysis_runs_report_idx`. In the
cost-based plans on this dataset only the event count over every run of an
experiment still scans `analysis_runs`, and small lookup tables such as subjects
may be hashed. The dataset is synthetic;
plans on a real installation depend on its data.

## Local container run

From the repository root:

```sh
docker compose up -d db
docker compose --profile setup run --rm schema
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

The Compose project `misko-new-backend` uses a new `foundation-db` volume; it
does not mount the old installation's volume. Credentials and `JWT_SECRET` in
`docker-compose.yml` are development-only and the API binds to loopback. No
frontend or GCP deployment is started. Re-running `schema` refuses an existing
`misko` schema; it never drops data.

## Run Go directly

Use the version in `go.mod` and a dedicated empty PostgreSQL 18 database. The
schema creates the `btree_gist` extension, a trusted contrib extension, so the
installing user needs CREATE permission on the database. Export the variables
below (see `.env.example`), then from `backend/`:

```sh
go run ./cmd/bootstrap schema
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

The processes do not load `.env` files or change the schema on startup.
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
`TEST_DATABASE_URL` must name an empty database: the schema tests install into it
and deliberately leave their fixture. Identity, laboratory, subject, experiment
and end-to-end HTTP tests create uniquely named `misko_test_*` databases and drop
only those. They cover concurrent last-administrator protection, concurrent
setup, concurrent enrollment and assignment, cross-experiment references,
overlapping periods, target versus actual group counts, intervention references and
history, per-kilogram dose conversion, SQL constraints, search escaping, stable paging and the read/write RBAC matrix over HTTP.

CI runs these checks on PRs, including PRs targeting `new-backend`, then builds the
container without pushing an image. Publication jobs are restricted to `main`;
this unfinished branch cannot be manually deployed or released. No auto-merge is
enabled for `new-backend`.

Known limits: login has no rate limiting yet, and there is no server-side logout
(a client signs out by discarding its token; password changes revoke all tokens).
Experiments and enrollments cannot be deleted yet; research records are kept.

[OpenAPI](api/openapi.yaml) describes only implemented endpoints.
[Dependencies](DEPENDENCIES.md) records pinned versions and verification sources.
