---
title: Go backend (preview)
description: Install and use the Go backend being rebuilt on the new-backend branch, including sign-in, users, roles, laboratory settings, subjects and experiments.
---

:::caution
This page describes the unreleased `new-backend` branch. The Go API does not yet
serve the Vue panel, so the Installation and Usage pages still describe the
current release. Do not deploy this branch.
:::

## What exists today

| Area | Status |
|------|--------|
| Health and readiness probes | Implemented |
| Sign-in, current user, own password change | Implemented |
| User management with the five roles | Implemented |
| Laboratory settings (one laboratory per installation) | Implemented |
| Subjects, experiments, phases, groups and enrollments | Implemented |
| Disease models, substances, intervention plans and administrations | Implemented |
| Paradigm catalog with metric definitions for eleven paradigms | Implemented |
| Environments with measurement revisions and experiment test protocols | Implemented |
| Tests, video uploads and video analysis | Not yet |
| Vue panel on the new API | Not yet |

## Installation

You need Docker with Docker Compose. Run these commands from the repository root
on the `new-backend` branch:

```bash
docker compose up -d db
docker compose --profile setup run --rm schema
docker compose --profile setup run --rm setup
docker compose up --build -d backend
curl --fail http://127.0.0.1:4000/api/ready
```

1. `db` starts PostgreSQL in a new volume. The old installation's data is not touched.
2. `schema` creates the tables in the empty database. Running it again is refused, and nothing is dropped.
3. `setup` creates the laboratory and the first administrator, then prints the
   administrator's email and generated password **once** in your terminal. Run
   it again and it creates nothing.
4. `backend` starts the API on `127.0.0.1:4000`.

If `setup` runs without a terminal (for example in CI), it refuses to start
unless you pass `-credentials-file PATH`. The password then goes into that new
file, readable only by its owner. It is never written to logs.

If you use your own PostgreSQL server instead of the Compose database, it must be
PostgreSQL 18 or later. The user that runs `schema` needs permission to create
objects in the database, because the schema enables the bundled `btree_gist`
extension.

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
| `LAB_TIMEZONE` | `UTC` | Laboratory time zone, as an IANA name such as `Europe/Istanbul`. |

The backend README lists every variable, including database and timeout settings.

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
| SUPERADMIN, LAB_MANAGER | Everything, including managing users and laboratory settings |
| RESEARCHER | Manage studies, subjects, apparatus and tests; read everything |
| TECHNICIAN | Record weights and run tests; read everything |
| VIEWER | Read only |

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

### Subjects

Every signed-in user can read subjects. SUPERADMIN, LAB_MANAGER and RESEARCHER
can change them.

- `POST /api/subjects` creates a subject with `code`, `species` (`MOUSE` or
  `RAT`), `sex` (`FEMALE`, `MALE` or `UNKNOWN`) and optional `strain`,
  `birthDate` (`YYYY-MM-DD`) and `notes`. Codes are unique regardless of upper
  or lower case.
- `GET /api/subjects` lists subjects with `search`, `species`, `sex`, `sort`,
  `order`, `page` and `pageSize`.
- `PATCH /api/subjects/{id}` changes the fields you send. An empty `strain`,
  `birthDate` or `notes` clears it.
- `DELETE /api/subjects/{id}` deletes a subject that is not enrolled in any experiment.
- `GET /api/subjects/{id}/enrollments` shows every experiment the subject is enrolled in.

A subject exists on its own, so the same animal can join several experiments
without being copied.

### Experiments

Every signed-in user can read experiments. SUPERADMIN, LAB_MANAGER and
RESEARCHER can design them and enroll subjects.

1. Create the experiment with `POST /api/experiments` (`code`, `title`, optional
   `description` and `requiresControl`). The experiment detail shows
   `controlRequirementMet`, which becomes true once a control group exists.
2. Add measurement phases with `POST /api/experiments/{id}/phases` (`name`,
   `position`), for example Baseline at position 1 and Post-treatment at position 2.
3. Add groups with `POST /api/experiments/{id}/groups` (`name`, `role` of
   `CONTROL` or `TREATMENT`, optional `targetSize`).
   `GET /api/experiments/{id}/groups` shows each group's target and how many
   subjects are in it right now.
4. Enroll a subject with `POST /api/experiments/{id}/enrollments` (`subjectId`,
   `enrolledAt`, optional `groupId`). A subject can be enrolled only once in
   each experiment.
5. Move a subject to another group with
   `POST /api/experiments/{id}/enrollments/{enrollmentId}/assignments`
   (`groupId`, `effectiveFrom`). The previous group period ends at that moment
   and stays in the history shown by
   `GET /api/experiments/{id}/enrollments/{enrollmentId}`.

Phases and groups are separate on purpose. A phase says when a subject is
measured; a group says which arm it belongs to. A treated subject measured at a
healthy baseline stays in its treatment group.

The system refuses:

- phases, groups or enrollments that belong to another experiment,
- a group period that overlaps another one, starts before enrollment or does not
  start after the current period,
- deleting a group that still has assignments, or a subject that is enrolled.

Experiments and enrollments cannot be deleted yet, so research records are kept.

### Interventions

Disease models and substances are shared lists. SUPERADMIN, LAB_MANAGER and
RESEARCHER maintain them with `POST /api/disease-models` and
`POST /api/substances` (`name`, optional `description`).

- **Plans**: `POST /api/experiments/{id}/intervention-plans` describes what a
  group should receive (`groupId`, `substanceId`, `amount`, `unit`, `route`,
  `schedule`, optional `phaseId`). A plan never records that anything was given.
- **Weights**: TECHNICIAN and above record body weight with
  `POST /api/subjects/{id}/weights` (`grams`, `measuredAt`).
- **Conditions**: TECHNICIAN and above record observations with
  `POST /api/subjects/{id}/conditions` (`diseaseModelId`, `status`,
  `observedAt`). Record induction (`INDUCED`) and confirmation (`CONFIRMED` or
  `NOT_CONFIRMED`) as separate observations. `GET /api/subjects/{id}/conditions`
  shows the history and the current status, and
  `GET /api/conditions?current=true&status=CONFIRMED` finds subjects by their
  current status.
- **Administrations**: TECHNICIAN and above record what was actually given with
  `POST /api/experiments/{id}/enrollments/{enrollmentId}/administrations`
  (`substanceId`, `amount`, `unit`, `route`, `administeredAt`, optional `planId`
  and `weightMeasurementId`). Doses per kilogram (`mg/kg`, `ug/kg`, `IU/kg`)
  need a weight of the same animal from the previous 7 days, and the response
  shows the absolute dose. `GET /api/subjects/{id}/administrations` and
  `GET /api/administrations` filter by `substanceId`, `experimentId`, `from` and `to`.

Write amounts as decimal text, for example `"0.25"`. Records keep the names and
doses they were saved with, even if a substance is renamed or a plan changes later.

### Paradigm catalog

Every signed-in user can read the paradigm catalog. Paradigms are defined in the
code and cannot be created, edited or deleted through the API.

- `GET /api/paradigms` lists paradigms with their published versions and whether
  automated video analysis is available (`automatedAnalysis`). Eleven
  paradigms are published: `BARNES_MAZE`, `EPM`, `LIGHT_DARK`, `MWM`,
  `NOVEL_OBJECT`, `OPEN_FIELD`, `POLE`, `ROTAROD`, `THREE_CHAMBER`, `TREADMILL`
  and `Y_MAZE`. Automated analysis is not available for any of them yet.
- `GET /api/paradigms/{key}` returns the latest version, and
  `GET /api/paradigms/{key}/versions/{version}` returns one exact version. Each
  version lists its apparatus and session parameters with units, allowed ranges
  and defaults, its zones and events, and for every metric the definition,
  formula, inputs and what happens when data is missing.

Some paradigms need more than a tracked position:

- Zones marked `calibrated`, such as the elevated plus maze arms or the Barnes
  maze holes, are drawn per video as circles or polygons. Other zones, such as the
  water maze platform, come from the apparatus parameters.
- `inputEvents` lists observations that are scored rather than tracked, such as
  a rotarod fall, pole test turns or novel object exploration. An event type that
  was not scored produces missing metrics, not zeros.

Results always name the paradigm version, the metric engine version and the
result schema version they were computed with. A metric that cannot be computed
is reported as missing with a reason, never as zero. For example, a probe trial
without an escape reports `EVENT_NOT_OBSERVED`, and a Y maze trial with fewer
than three arm entries reports `INSUFFICIENT_ENTRIES`. A published version never
changes; a changed definition is published as a new version.

### Environments and protocols

An environment is one physical apparatus, such as a particular water tank, set up
for one paradigm. Researchers, lab managers and super admins can create and
revise environments and protocols; every signed-in user can read them.

- `POST /api/environments` creates an environment with its first revision. Give
  every apparatus measurement of the paradigm version, for example the tank and
  platform sizes for `MWM`. Values outside the allowed ranges, or inconsistent
  values such as a platform outside the tank, are rejected.
- When the apparatus changes, add a revision with
  `POST /api/environments/{id}/revisions`. Earlier revisions never change and
  cannot be deleted. You can still rename an environment or edit its notes.

A protocol describes how an experiment tests its subjects. It belongs to one
experiment and can combine paradigms, for example an open field test followed by
an elevated plus maze.

- `POST /api/experiments/{id}/protocols` creates a protocol with its first
  version. Each step gives its position, the paradigm and its version, an
  environment revision of that paradigm, the trial type, the number of trials,
  the pause between trials in seconds and any session parameters.
- Number the steps from 1 without gaps. An environment of another paradigm, a
  trial type the paradigm does not define and session values out of range are
  rejected. Session parameters you leave out get the paradigm defaults, and the
  stored version shows them.
- To change a protocol, add a version with
  `POST /api/experiments/{id}/protocols/{protocolId}/versions`. Stored versions
  never change, and a new environment revision does not change a version that
  uses an earlier revision.

### Errors

Every error has a stable `code`, such as `auth.forbidden` or
`user.lastPrivileged`, and an English message. A 401 status means you are not
signed in or your token is no longer valid. A 403 status means your role does not
allow the action.
