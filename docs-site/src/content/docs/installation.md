---
title: Installation
description: How to install and run Mişko, both with Docker and as a local development setup.
---

This page explains how to get Mişko running, whether you are a lab administrator
who just wants the whole system up, or a developer who wants to work on the code.

## What you are installing

Mişko is made of a few parts that run together:

- **Database** (PostgreSQL) stores all lab records.
- **Backend** (Go API) holds the business logic and talks to the database.
- **Jobs** is a small process that queues analysis runs and requeues expired ones.
- **Frontend** (Vue 3 single-page app, served by Nginx) is the web panel you use in the browser.
- **Worker** (Python) analyzes recorded videos. It is optional.

With Docker you start them together. For development you can run the database in
Docker and the applications from source.

## Prerequisites

| Tool | Why you need it | Notes |
|------|-----------------|-------|
| Docker + Docker Compose | Run the full stack | The recommended path for most users. |
| Go 1.27+ and Node.js 22+ | Local development | Only needed if you run the applications outside Docker. |
| Git | Get the source code | `git clone` the repository first. |

If you only want to use Mişko, Docker is the only thing you need.

## Option A: Docker (recommended)

```bash
git clone https://github.com/HappyHackingSpace/Misko.git
cd Misko

cp .env.example .env                               # optional: every value has a default
docker compose --profile setup run --rm migrate    # install or upgrade the schema
docker compose --profile setup run --rm setup      # create the laboratory and the administrator
docker compose up -d                               # start the API, the jobs process and the panel
```

When it finishes:

- Web panel: [http://localhost:8080](http://localhost:8080)
- API: [http://localhost:4000/api](http://localhost:4000/api)

The schema and the administrator are installed by those two explicit commands
rather than on container start. After updating to a new version, run `migrate`
again before `docker compose up -d`: it applies only the migrations your database
does not have yet, all in one transaction, and does nothing when it is up to
date.

### The first login password

There is no fixed default password and no public sign-up. The `setup` command
creates the administrator using `ADMIN_EMAIL`, generates a strong password and
prints it to your terminal **once**:

```
Administrator created. This password is shown only once; change it after signing in.
email: admin@misko.local
password: ...
```

It is never written to a log. Copy it before closing the terminal. If you would
rather receive it as a file, mount a folder and ask for it there:

```bash
docker compose --profile setup run --rm -v "$PWD/secrets:/out" setup -credentials-file /out/admin.txt
```

The file is created with mode 0600 and the command refuses to overwrite an
existing one. Sign in, change the password, then create the other users from the
**Users** screen. See [Usage](../usage/) for the day-to-day workflow.

### Video storage

By default videos and analysis outputs are stored on this machine, in the
`local-videos` Docker volume. The backend runs with `LOCAL_STORAGE_DIR=/data/videos`
and serves the upload and playback URLs itself under `/api/local-storage/`, so
no cloud account is needed. The URLs are signed with `JWT_SECRET` and expire like
cloud storage URLs do.

To use a private Google Cloud Storage bucket instead, set `GCS_BUCKET` (and
`GCS_SIGNER_EMAIL`). When a bucket is set it takes precedence over the local
directory.

### Analysis worker

The worker is optional: without it, tests can still be recorded and the
observer-scored paradigms (rotarod, pole, treadmill, novel object) are scored
in the panel, but tracked paradigms are not analysed. It tracks the seven
tracked paradigms: `OPEN_FIELD`, `EPM`, `Y_MAZE`, `BARNES_MAZE`,
`THREE_CHAMBER`, `MWM` and `LIGHT_DARK`. It has been tested on synthetic video
only.

1. Sign in as a lab manager or the administrator and register the worker with
   model version `misko-tracker-bgsub 2.0.0` and every tracked capability at
   version 1. Observer-scored paradigms cannot be capabilities.

   ```bash
   TOKEN=$(curl -s http://localhost:4000/api/auth/login \
     -H 'Content-Type: application/json' \
     -d '{"email": "admin@misko.local", "password": "YOUR_PASSWORD"}' | jq -r .token)
   curl -s http://localhost:4000/api/analysis/workers \
     -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
     -d '{
    "name": "worker-1",
    "modelVersion": "misko-tracker-bgsub 2.0.0",
    "capabilities": [
      {"paradigmKey": "OPEN_FIELD", "paradigmVersion": 1},
      {"paradigmKey": "EPM", "paradigmVersion": 1},
      {"paradigmKey": "Y_MAZE", "paradigmVersion": 1},
      {"paradigmKey": "BARNES_MAZE", "paradigmVersion": 1},
      {"paradigmKey": "THREE_CHAMBER", "paradigmVersion": 1},
      {"paradigmKey": "MWM", "paradigmVersion": 1},
      {"paradigmKey": "LIGHT_DARK", "paradigmVersion": 1}
    ]
  }'
   ```

2. The response shows the worker token (`mw_...`) once. Put it in `.env` as
   `MISKO_WORKER_TOKEN` and start the worker:

   ```bash
   docker compose --profile worker up -d worker
   ```

`GET /api/analysis/capabilities` lists what the enabled workers can analyse.
The worker's own settings and limits are in `worker/README.md`.

### Upgrading from an earlier version

The database volume is now called `postgres-data`. The older `foundation-db` and
`db-data` volumes are no longer mounted and are left as they are, never deleted,
so after upgrading the stack starts with an empty database: run the `migrate` and
`setup` commands again. Remove an old volume with `docker volume rm` once you no
longer need it (`docker volume ls` lists them).

### Stopping and resetting

```bash
docker compose down            # stop the containers, keep the data
docker compose down -v         # stop AND delete the database and video volumes (full reset)
```

After a `down -v` the database and the stored videos are gone, so run the `migrate` and `setup`
commands before starting the stack.

## Option B: Local development

Run only the database in Docker, and run the applications from source.

```bash
docker compose up -d db        # database only
```

**Backend** (one terminal). The Go processes do not read `.env` files, so export
the variables. `backend/.env.example` lists all of them.

```bash
cd backend
export DATABASE_URL='postgres://misko:misko_local_only@127.0.0.1:5432/misko_foundation?sslmode=disable'
export JWT_SECRET="$(openssl rand -base64 48)"
go run ./cmd/bootstrap migrate  # install or upgrade the schema
go run ./cmd/bootstrap setup    # create the laboratory and the administrator
export LOCAL_STORAGE_DIR="$PWD/.local-storage"   # optional: store videos in a folder
go run ./cmd/api                # API on http://localhost:4000
```

With `LOCAL_STORAGE_DIR` set the API keeps videos in that folder and the panel
uploads to it through the same `/api` proxy. A worker run from source uses
`MISKO_API_URL=http://localhost:4000` and reaches the files the same way.

**Frontend** (a second terminal):

```bash
cd frontend
npm install
npm run dev                     # panel on http://localhost:5173
```

In development the frontend proxies `/api` calls to the backend on port 4000, so
you use the panel at [http://localhost:5173](http://localhost:5173).

## Environment variables

`docker-compose.yml` reads these from `.env`. Every one has a default, and the
local database credentials are fixed in the compose file because that stack is
for development only.

| Variable | Default | What it does |
|----------|---------|--------------|
| `JWT_SECRET` | a local development value | Signs login tokens. Use at least 32 random bytes anywhere that is not your own machine. |
| `ADMIN_EMAIL` | `admin@misko.local` | Email of the administrator that `setup` creates. |
| `LAB_NAME` | `Misko Laboratory` | Laboratory name shown in the panel. |
| `LAB_TIMEZONE` | `UTC` | Time zone the laboratory records use. |
| `FRONTEND_PORT` | `8080` | Host port for the web panel. |
| `JOB_INTERVAL` | `10s` | How often the jobs process queues analysis runs. |
| `GCS_BUCKET` | empty | Private bucket for videos. When set it is used instead of local storage. |
| `GCS_SIGNER_EMAIL` | empty | Service account that signs the upload and read URLs. |
| `MISKO_WORKER_TOKEN` | empty | Only for the `worker` profile, after registering a worker in the API. |

In `docker-compose.yml` the backend also has `LOCAL_STORAGE_DIR=/data/videos` on
the `local-videos` volume. Outside Docker, set `LOCAL_STORAGE_DIR` to any folder
to store videos there; with neither it nor `GCS_BUCKET` the video routes answer 503.

For the backend outside Docker the full list, including `TOKEN_TTL`,
`BCRYPT_COST`, `UPLOAD_URL_TTL` and `READ_URL_TTL`, is in `backend/.env.example`.

:::caution
Before deploying for real, set a strong `JWT_SECRET`, use real database
credentials, and serve the panel over HTTPS. Deployment from prebuilt images is
described in [DEPLOY.md](https://github.com/HappyHackingSpace/Misko/blob/main/docs/DEPLOY.md).
:::

## Verifying the installation

1. Check the API: [http://localhost:4000/api/health](http://localhost:4000/api/health) returns a success response, and `/api/ready` also checks the database.
2. Open the panel and sign in with the administrator email and the password `setup` printed.
3. If the experiments screen loads, the installation is healthy.

## Troubleshooting

- **Port already in use**: change `FRONTEND_PORT` in `.env`, or stop whatever holds port 4000, and run `docker compose up -d` again.
- **Cannot sign in and the password is gone**: the password is shown once and is not recoverable. Reset with `docker compose down -v`, then run the `migrate` and `setup` commands again.
- **`setup` refuses to run**: it needs a terminal for the password. In a script, pass `-credentials-file` with a path that does not exist yet.
- **Video upload returns 503**: neither `LOCAL_STORAGE_DIR` nor `GCS_BUCKET` is set on the backend.
- **The database is empty after an upgrade**: the stack now uses the `postgres-data` volume. Run `migrate` and `setup`; the old volume is still there if you need its data.
- **Backend cannot reach the database**: make sure the `db` container is healthy (`docker compose ps`).
