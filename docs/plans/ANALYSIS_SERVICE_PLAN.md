# Analysis as an independent service, and paradigm trials

Status: planned, starts after AUTOMATIC_ANALYSIS_PLAN.md is merged.
Date: 2026-10-18.

## Goal

1. **Analysis is a standalone module.** It analyses "this video, with this
   paradigm, these parameters and this calibration" and knows nothing about
   experiments, tests, subjects or review. Workers talk only to it.
2. **Paradigm trials** ("Paradigma testi", a main menu entry): pick a
   paradigm and an environment, calibrate there if needed, upload a video, see
   the analysis result (metrics, track, overlay video, QC), change thresholds
   and analyse the same video again. No groups, no review, no reports. A trial
   result never moves into an experiment.
3. **Experiments use the same service.** A test subject's analysis is a job
   in the service; review, reports and the dashboard stay on the experiment
   side and read the job's results through the service's interface.

## Module boundaries

Every module has its own domain/application/adapters, its own tables, and
talks to others only through small application interfaces (ports) wired in
bootstrap. The architecture test enforces that no module imports another's
domain or adapters.

| Module | Owns | Depends on |
|---|---|---|
| `analysis` (service) | analysis jobs, workers and capabilities, leases and attempts, outputs, metric results, derived events, QC, scheduler of queued jobs | `paradigms` (catalog, engine), `media` (read a verified video asset, store outputs) |
| `calibration` | calibrations and their zones, owned by an environment revision (default), a test subject (override) or a paradigm trial | `paradigms`, `environments` |
| `trials` (new) | paradigm trials: paradigm, environment revision, own calibration, uploaded video, the jobs run on it | `analysis`, `calibration`, `media`, `environments` |
| experiment side (`tests`, `review`, `reports`) | which job analyses which test subject, review decisions, reports | `analysis` through its port only |

## Analysis job

An immutable request plus its execution state:

- **Input snapshot:** paradigm key and version; resolved parameters
  (apparatus, analysis, session); calibration snapshot (frame size, crop,
  transform, zones in pixels and cm); source video asset and clip; the
  requester (`TRIAL` or `TEST_SUBJECT`) and the requesting id as an opaque
  reference. The service never joins the requester's tables.
- **Execution:** status QUEUED, RUNNING, SUCCEEDED, FAILED; attempts, lease,
  worker, model version; outputs (trajectory, overlay video); metric results;
  derived events; QC.
- **API:**
  - in-process port: `Submit(input) -> jobId`, `Get(jobId)`, `Results(jobIds)`;
  - HTTP: the worker routes as today (claim, heartbeat, outputs, result,
    failure) and read routes for a job's results, used by both screens;
  - worker and capability management stays here.

## Experiment side

- `test_subject_analyses (test_subject_id, job_id, created_at)` links a test
  subject to its jobs. An orchestrator submits a job when a test subject has a
  verified video, an effective calibration and a non-excluded allocation; it
  replaces the scheduler's test-subject candidate query.
- `subject_results` (review) points to a job instead of a run. Reports read
  accepted metrics through a view or the service port.

## Paradigm trials (UI)

- Menu "Paradigma testi" / "Paradigm trials": a list of trials (name,
  paradigm, environment, date, who, last result) and "New trial".
- Trial page:
  - choose paradigm and environment revision;
  - use the environment calibration, or calibrate on the trial's own video
    (the calibrator from the environment page; stays on the trial and never
    changes the environment);
  - upload the video; the job starts automatically;
  - the result panel: metrics, QC, overlay video and track;
  - "Analyse again" with changed ANALYSIS thresholds (a new job, earlier ones
    kept for comparison).
- Permission: `trial:run` (new) for creating trials, reading open to every
  role.

## Database

Migrations from 017 on (001-016 are immutable). Production has no analysis
data yet, so the analysis tables can be replaced; the migration still moves
existing rows if any exist. The `analysis_runs` foreign keys to tests, test
subjects and recordings move to `test_subject_analyses`.

## Work packages (after the automatic analysis merge)

- **SV service:** `analysis` module refactor (jobs with snapshots, port,
  worker API unchanged for the worker except the payload's source and
  calibration, which come from the snapshot), migrations, tests.
- **EX experiment side:** orchestrator, `test_subject_analyses`, review and
  reports through the port, tests.
- **TR trials:** `trials` module, calibration ownership by a trial, API,
  integration tests.
- **UI:** "Paradigma testi" menu and pages, experiment screens on the new
  read routes, TR and EN, Playwright.
- Docs EN and TR, local end-to-end check.
