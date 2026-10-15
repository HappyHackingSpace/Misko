# Everything before analysis

> **2026-10-15:** the paradigm catalog was trimmed to `MWM`, `NOVEL_OBJECT`,
> `POLE` and `TREADMILL`; the other seven paradigms were removed. The text
> below predates this change and is kept as written.

Status: plan. Date: 2026-10-15.

Scope: everything up to the point where an animal has been tested and its
video is uploaded. Worker runs, analysis results, human review and reports are
the next phase. Design background: [DOMAIN_REDESIGN.md](DOMAIN_REDESIGN.md).
Review backlog: [CODE_REVIEW_2026-10.md](CODE_REVIEW_2026-10.md).

Every step is one pull request with backend, migration, panel screens, tests
and docs (English and Turkish) together, so the panel keeps working after each
merge. Production holds no study data worth keeping, so old design tables are
replaced, not migrated.

## Order

| # | Step | Depends on | State |
|---|---|---|---|
| 0 | Paradigm templates | - | Code done in `feat/paradigm-templates`, not yet verified or opened |
| 1 | Groundwork: shared helpers and panel foundation | - | Not started |
| 2 | Catalog: substances and disease models | 1 | Not started |
| 3 | Experiment design | 2 | Not started |
| 4 | Subjects and allocation | 3 | Not started |
| 5 | Dosing, induction and weights | 4 | Not started |
| 6 | Tests as sessions | 4 | Not started |
| 7 | Clean-up and hardening | 6 | Not started |

## 0. Paradigm templates

Branch `feat/paradigm-templates` (worktree `../Misko-paradigms`).

Done: names, descriptions, parameter labels and help, zone, event and metric
text in English and Turkish next to each definition; API answers in the
requested language; parameters split into APPARATUS, ANALYSIS and SESSION;
environment form, environment detail, paradigm pages and protocol steps
rebuilt (units in words, ratios as percent, switches, defaults, session
settings finally editable). Contract diff against the published manifests: 0.

Left:
- Rebase on `main` (4 commits behind).
- Merge one of the open CRLF golden-file fixes (#250 or #251) first, or this
  branch conflicts with it on every manifest golden.
- Run the integration suite and the Playwright suite (needs Docker), check the
  screens in a browser, then open the PR.
- Confirm the scope moves: open field center size, water maze wall zone, novel
  object exploration settings to ANALYSIS; rotarod speeds to SESSION.
- Covers the unit part of #234 (`cm_s` shown as `cm/s`). Rounding of result
  values belongs to the analysis phase.

Follow-up inside this step or right after:
- Choice parameters: integer codes such as the Y maze novel arm (0-3) shown as
  a list (none, A, B, C) instead of a number.
- Environment list shows the paradigm name instead of its key.

## 1. Groundwork

Done before the new contexts multiply the duplication.

Backend (from the review):
- `platform/nullable` for the `nullable`, `optional`, `value` helpers.
- `pgtx.Translate(err, notFound, constraints, op)` for the nine store copies.
- `httpjson.Page[T]` and shared paging, `httpjson.AuthFailures`.
- A pure text normalizer usable from domains (policy allowlist exception).

Panel foundation, so every later screen is built once in the new style:
- Layout, navigation and design tokens (light and dark), one form field and
  one table pattern, empty and loading states, success toasts, confirm dialogs
  for destructive actions.
- Shared composables: `useAsync`, `useFormatDate(locale)`, `useStatusTag`.
- Route parameter changes reload detail pages (today `/experiments/a` to
  `/experiments/b` keeps showing `a`).
- Lists page and search on the server instead of `pageSize: 100`.
- Auth: only a 401 logs the user out, not a network error or a 500.

## 2. Catalog: substances and disease models

- Migration: `substances.kind` (compound, vehicle, inducer, reference drug),
  `disease_models.induction_type` and `confirmation_criterion`.
- Backend: validation, API, OpenAPI.
- Panel: catalog screens for substances and disease models (list, create,
  edit), both languages.

## 3. Experiment design

- Migration: experiments get hypothesis, status (`DRAFT`, `RUNNING`,
  `COMPLETED`), `day_zero`, randomisation, blinded; drop `requires_control`.
  Groups get a code, five roles and a disease model. New `regimen_items`,
  `timepoints`, `endpoints`. Drop `experiment_phases` and `intervention_plans`.
- Backend: the start rules (role and regimen checks, one primary endpoint
  whose metric belongs to a paradigm of the study's protocols); starting sets
  day 0 and freezes the design; completed studies are read-only.
- Protocols stay as they are, with session settings from step 0.
- Panel: experiment page with tabs (overview, groups and regimens, timepoints,
  protocols, endpoints), a start button that lists what is missing.

## 4. Subjects and allocation

- Migration: `allocations` replaces `enrollments` and `group_assignments`; a
  subject belongs to exactly one group (`UNIQUE (subject_id)`); exclusion with
  reason, note and date; optional cage.
- Backend: allocate in `DRAFT` and `RUNNING`; exclude instead of delete;
  optional randomisation that fills groups up to their target size.
- Panel: subject list with group and status, allocation screen per experiment
  with group fill against target, exclusion dialog.

## 5. Dosing, induction and weights

- Migration: `administrations` re-keyed to allocation and regimen item;
  `induction_outcomes` replaces `subject_conditions`; weights unchanged.
- Backend: a dose in mg/kg needs a weight; the planned dose comes from the
  regimen item; a daily dosing list derived from regimens and day 0.
- Panel: "today" worklist of doses due per group, quick weigh-and-dose entry,
  induction confirmation per animal.

## 6. Tests as sessions

- Migration: `tests` become sessions of one experiment (timepoint, protocol
  step, schedule, status); new `test_subjects` (animal plus arena
  environment revision, one animal per arena, an animal once per test);
  `test_recordings` exactly one active video per test; trials hang off the
  test subject. Old test data is dropped.
- Backend: planning checks the animal is allocated, not excluded, not already
  in another test of the same timepoint and step, and its arena runs the
  test's paradigm; starting and completing a session.
- Upload: fix the browser resumable upload resuming from the wrong offset on a
  308 without `Range` (review item 3); bound the upload size in the signed URL
  and delete rejected objects (review item 9).
- Panel: plan a session (timepoint, step, animals into arenas), session
  worklist by day (today, overdue, upcoming), run screen with trials per
  animal and the single video upload.

## 7. Clean-up and hardening

- Remove the old screens and endpoints that steps 3 to 6 replaced; delete
  orphaned video objects of dropped tests.
- Login rate limit (review item 8).
- Deploy: pin the VM host key (item 10), refuse to deploy without
  `SITE_ADDRESS` or bind to loopback (item 11), pin the Caddy image, backend
  healthcheck in the prod compose file.
- Documentation sweep #249 for everything above.

## Left for the analysis phase

Calibration overrides per test subject, worker changes (one run per test
subject, paradigm check #244, memory and resilience items 2, 4, 5, 6),
scheduler starvation (item 1), human review of results (`subject_results`),
endpoint reports and group comparisons, scored observations (#245 to #248),
heatmaps (#232), result rounding (#234), Open Field validation (#241 to #243),
export timeouts (item 12).
