---
title: Go backend (preview)
description: Install and use the Go backend being rebuilt on the new-backend branch, including sign-in, users, roles and laboratory settings.
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
| Subjects, experiments, paradigms, video analysis | Not yet |
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

### Errors

Every error has a stable `code`, such as `auth.forbidden` or
`user.lastPrivileged`, and an English message. A 401 status means you are not
signed in or your token is no longer valid. A 403 status means your role does not
allow the action.
