# Porting the backend to the fresh schema

Working agreement for the parallel port of the Go backend onto
`backend/schema/001-012` (branch `feat/fresh-schema`). Read with
[DOMAIN_REDESIGN.md](DOMAIN_REDESIGN.md) (business model and decisions).

## Rules for every work package

1. Work in your own worktree and branch, created from `feat/fresh-schema`:
   `git -C /Users/recep.kizilarslan/Workspace/Repositories/Misko-schema worktree add ../Misko-wp-<name> -b wp/<name> feat/fresh-schema`.
   Commit there (several commits are fine). Never push. Never touch other worktrees.
2. Do not change `backend/schema/*.sql`. If the schema blocks you, stop and
   report the exact change you need.
3. Own only the directories listed for your package, plus:
   - your `backend/internal/bootstrap/<name>_integration_test.go` file(s),
   - your entries in `backend/sqlc.yaml`,
   - your wiring lines in `backend/internal/bootstrap/api.go` (and
     `analysis.go` for the analysis package),
   - your paths and schemas in `backend/api/openapi.yaml`,
   - permissions in `internal/access/domain` only if listed below.
   Delete code, queries, tests and routes that the new schema made obsolete in
   your contexts. Do not edit other contexts.
4. sqlc: other packages are ported in parallel, so `make generate` for the whole
   config fails until all are done. Generate only your contexts with a
   temporary config that lists just your `sql:` entries
   (`go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate -f /tmp/<you>.yaml`
   run from `backend/`, paths relative to `backend/`). Do not commit that file.
5. Follow the existing conventions: domain / application / adapters (http,
   postgres, catalog), `actor.Require(permission)` first in every use case,
   sentinel errors mapped in the handler's `failures` table to a stable code,
   composite keys and SQL constraints as the last line of defence, English
   comments, no em dash anywhere. The architecture test in
   `backend/tests/architecture` must pass.
6. Map database rule violations (constraint names from the SQL files, for
   example `allocations_frozen`, `experiment_groups_frozen`) to sentinel errors
   in the store's translate function, so the API answers 409/400 with a code,
   never 500.
7. Tests: unit tests for domain and application; HTTP + database integration
   tests in `internal/bootstrap/<name>_integration_test.go` against
   `TEST_DATABASE_URL=postgres://postgres:test@127.0.0.1:55433/postgres?sslmode=disable`
   (a shared Postgres 18.6 container is running; tests create their own
   databases). Run `go vet ./...`, `go vet -tags integration ./...`,
   `go test ./internal/<yours>/... ./tests/architecture/...` and
   `go test -tags integration -run <YourTests> ./internal/bootstrap/ ./internal/<yours>/...`.
8. Error codes are new i18n keys for the panel: list every new code
   (`experiment.notDraft`, ...) in your final report so they get translated.
9. Final report: what you built, every route with method and body, error
   codes, permissions used, anything left open, and the commit hash.

## Permissions

| Action | Permission |
|---|---|
| Catalog (substances, disease models), experiment design, allocation, protocols | `study:write` |
| Environments and calibration | `apparatus:write` |
| Subjects | `subject:write` |
| Weights | `weight:write` |
| Planning tests (sessions, their animals) | `test:write` |
| Running tests, trials, videos, doses, induction outcomes, result review | `test:run` |
| Reading anything | `*:read` |

## Work packages

### WP1 catalog, environments, subjects (`wp/catalog`)
- New context `internal/catalog`: substances (name, kind, description) and
  disease models (name, induction type, confirmation criterion, description):
  list (search, kind filter), get, create, update, delete (409 when used).
  Routes `/api/catalog/substances`, `/api/catalog/disease-models`.
- `internal/environments`: the revision column is `parameters` (APPARATUS and
  ANALYSIS values); rename the API field `apparatus` to `parameters`. Keep
  validation through the paradigm catalog (`ResolveSetup`).
- `internal/subjects`: weights move here from interventions
  (`POST/GET /api/subjects/{id}/weights`, `weight:write`). Subject JSON gains a
  read-only `allocation` summary (experiment id and code, group id, code and
  role, excluded) or null; list filter `unallocated=true`.
- Remove substances, disease models and weights from `internal/interventions`
  only after WP4 has its own copy; coordinate by leaving
  `internal/interventions` untouched (WP4 owns it).

### WP2 experiments (`wp/experiments`)
`internal/experiments`: experiments (code, title, description, hypothesis,
randomisation, blinded, status, day 0), groups (code, name, role, disease
model, target size), regimen items, timepoints, endpoints, allocations.
- `POST /api/experiments/{id}/start {dayZero}` runs the start rules from
  DOMAIN_REDESIGN.md section 5 ("Rules the database cannot express") and
  answers 422 with the list of unmet rules when any fails;
  `GET /api/experiments/{id}/readiness` returns the same list without starting.
  Endpoint metrics must exist in the paradigm catalog (read through an
  `adapters/catalog` package) for a paradigm used by the experiment's protocol
  steps (read protocol steps with SQL in your own queries).
- `POST /api/experiments/{id}/complete` (all tests cancelled or completed is
  checked by WP3 data: read `tests` with SQL).
- Allocations: allocate, move or remove in a draft, cage, exclude with reason
  (once), list with group fill against target; `POST .../allocations/randomise`
  with subject ids distributes them over groups up to their target size.
- Remove phases, enrollments and group assignments completely.

### WP3 protocols and tests (`wp/tests`)
- `internal/protocols`: steps no longer have an environment revision; a step is
  paradigm key and version, trial type, trials, inter-trial interval, session
  values (validated with `ResolveSetup(key, version, nil, session)`).
- `internal/tests`: a test is a session (timepoint, protocol version and step,
  scheduled time, status) with test subjects (allocation plus arena environment
  revision). Create with its animals; add and remove animals while planned;
  start, complete, cancel; trials per test subject; comments; worklist with
  filters (experiment, status, date range, timepoint). Default scheduled date
  is day 0 plus the timepoint's study day. Reject an animal already in another
  non-cancelled test of the same timepoint and step.

### WP4 dosing (`wp/dosing`)
- Rename `internal/interventions` to `internal/dosing`; drop plans, conditions,
  substances, disease models and weights from it (catalog and weights live in
  WP1 now, plans became regimen items in WP2).
- Administrations: record a dose for an allocation (planned item or unplanned),
  per-kg doses take the latest weight of the animal unless one is named; list
  per experiment and per animal.
- `GET /api/experiments/{id}/dosing?date=YYYY-MM-DD`: every dose due that day
  from the regimen items and day 0 (frequency, interval, start and end day),
  per animal, with whether it was given.
- Induction outcomes: record and list per experiment.

### WP5 media and calibration (`wp/media`)
- `internal/media`: one current video per test; a replacement supersedes it;
  attaching needs the test in progress or completed. Fix the browser resumable
  upload bug (frontend `api/upload.js`: a 308 without `Range` restarts at 0)
  and sign the upload with a content length range for the declared size.
- `internal/calibration`: environment revision defaults (unchanged behaviour)
  and overrides per test subject for the current video; effective calibration
  per test subject from the `effective_calibrations` view.

### WP6 analysis, reports, dashboard, worker (`wp/analysis`)
- `internal/analysis`: runs per test subject over the current video with its
  effective calibration; scheduler candidates come from test subjects with a
  current verified video, a valid effective calibration, a non-excluded
  allocation and a capable worker, skipping and logging bad candidates instead
  of aborting the tick; claim, heartbeat, outputs and result submission keep
  the attempt fencing. On a first successful run create `subject_results`
  PENDING. Review API: list pending (blind when the experiment is blinded),
  accept, reject with reason, choose another run.
- `internal/reports`: per experiment, endpoint and timepoint, descriptives per
  group (n, mean, SD, SEM, pending and excluded counts) from
  `report_accepted_metrics` and `report_test_subjects`, plus the comparisons
  from DOMAIN_REDESIGN.md (model validity, assay sensitivity, efficacy with
  rescue percent, dose response); CSV export.
- `internal/dashboard`: counts that still make sense on the new model.
- `worker/`: adapt to the claim payload (one animal, its crop and transform);
  reject unsupported paradigms with `UNSUPPORTED_PARADIGM` (#244); keep its
  tests passing.
