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
- **Worker** (Python) analyzes recorded videos. It is optional and needs cloud storage.

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
docker compose --profile setup run --rm schema     # install the schema
docker compose --profile setup run --rm setup      # create the laboratory and the administrator
docker compose up -d                               # start the API, the jobs process and the panel
```

When it finishes:

- Web panel: [http://localhost:8080](http://localhost:8080)
- API: [http://localhost:4000/api](http://localhost:4000/api)

The schema and the administrator are installed by those two explicit commands
rather than on container start. That is deliberate: starting the stack never
migrates a database you already have.

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

Uploading and playing back videos needs a private Google Cloud Storage bucket.
Set `GCS_BUCKET` (and `GCS_SIGNER_EMAIL`) for that. Without a bucket those routes
answer 503 and everything else works normally, which is enough to explore the
system.

### Stopping and resetting

```bash
docker compose down            # stop the containers, keep the data
docker compose down -v         # stop AND delete the database volume (full reset)
```

After a `down -v` the database is empty again, so run the `schema` and `setup`
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
go run ./cmd/bootstrap schema   # install the schema
go run ./cmd/bootstrap setup    # create the laboratory and the administrator
go run ./cmd/api                # API on http://localhost:4000
```

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
| `GCS_BUCKET` | empty | Private bucket for videos. Empty means the video routes answer 503. |
| `GCS_SIGNER_EMAIL` | empty | Service account that signs the upload and read URLs. |
| `MISKO_WORKER_TOKEN` | empty | Only for the `worker` profile, after registering a worker in the API. |

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
- **Cannot sign in and the password is gone**: the password is shown once and is not recoverable. Reset with `docker compose down -v`, then run the `schema` and `setup` commands again.
- **`setup` refuses to run**: it needs a terminal for the password. In a script, pass `-credentials-file` with a path that does not exist yet.
- **Video upload returns 503**: `GCS_BUCKET` is not set. That is expected without a bucket.
- **Backend cannot reach the database**: make sure the `db` container is healthy (`docker compose ps`).
