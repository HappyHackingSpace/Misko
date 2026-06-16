---
title: Installation
description: How to install and run Mişko, both with Docker and as a local development setup.
---

This page explains how to get Mişko running, whether you are a lab administrator
who just wants the whole system up, or a developer who wants to work on the code.

## What you are installing

Mişko is made of three parts that run together:

- **Database** (PostgreSQL) stores all lab records.
- **Backend** (Node.js / Express API) holds the business logic and talks to the database.
- **Frontend** (Vue 3 single-page app, served by Nginx) is the web panel you use in the browser.

With Docker you start all three with a single command. For development you can run
the database in Docker and the backend and frontend directly on your machine.

## Prerequisites

| Tool | Why you need it | Notes |
|------|-----------------|-------|
| Docker + Docker Compose | Run the full stack | The recommended path for most users. |
| Node.js 20+ | Local development of backend/frontend | Only needed if you run the apps outside Docker. |
| Git | Get the source code | `git clone` the repository first. |

If you only want to use Mişko, Docker is the only thing you need.

## Option A: Docker (recommended)

This is the fastest way to get a working system. It starts PostgreSQL, the API,
and the web panel together.

```bash
git clone https://github.com/HappyHackingSpace/Misko.git
cd Misko

cp .env.example .env       # adjust values if you want (see the table below)
docker compose up -d --build
```

When it finishes:

- Web panel: [http://localhost:8080](http://localhost:8080)
- API: [http://localhost:4000/api](http://localhost:4000/api)

Database migrations run automatically on startup, so you do not have to apply
them by hand.

### Get the first login password

There is no fixed default password and no public sign-up. On the first startup
the system creates a **superadmin** using `ADMIN_EMAIL` and generates a strong
password that is printed to the backend log **once**:

```bash
docker compose logs backend | grep -A6 SUPERADMIN
```

Copy that password, log in, change it, and then create the other users from the
**Users** screen in the panel. See [Usage](../usage/) for the day-to-day workflow.

### Load sample data (optional)

To populate a few example scenarios and a sample subject:

```bash
docker compose exec backend node prisma/seed.js
```

### Stopping and resetting

```bash
docker compose down            # stop the containers, keep the data
docker compose down -v         # stop AND delete the database volume (full reset)
```

## Option B: Local development

Run only the database in Docker, and run the backend and frontend directly so
you can edit the code with live reload.

```bash
docker compose up -d db        # database only
```

**Backend** (one terminal):

```bash
cd backend
cp .env.example .env
npm install
npm run db:migrate             # apply the database schema
npm run db:bootstrap           # create the superadmin (prints the password)
npm run db:seed                # sample data (optional)
npm run dev                    # API on http://localhost:4000
```

**Frontend** (a second terminal):

```bash
cd frontend
npm install
npm run dev                    # panel on http://localhost:5173
```

In development the frontend proxies `/api` calls to the backend on port 4000, so
you use the panel at [http://localhost:5173](http://localhost:5173).

## Environment variables

These are read from `.env` by Docker Compose. Sensible defaults exist for local
use, but you should change the secrets before any real deployment.

| Variable | Default | What it does |
|----------|---------|--------------|
| `POSTGRES_USER` | `misko` | Database user. |
| `POSTGRES_PASSWORD` | `misko` | Database password. Change for production. |
| `POSTGRES_DB` | `misko` | Database name. |
| `DB_PORT` | `5432` | Host port for PostgreSQL. |
| `BACKEND_PORT` | `4000` | Host port for the API. |
| `JWT_SECRET` | `change-me-in-production` | Secret used to sign login tokens. **Must** be changed for production. |
| `JWT_TTL` | `7d` | How long a login session stays valid. |
| `CORS_ORIGIN` | `*` | Which web origins may call the API. Restrict this in production. |
| `ADMIN_EMAIL` | `admin@miskolab.com` | Email of the superadmin created on first startup. |
| `LAB_NAME` | `Mişko Laboratuvarı` | Lab name shown in the panel for branding. |
| `FRONTEND_PORT` | `8080` | Host port for the web panel. |

:::caution
Before deploying for real, set a strong `JWT_SECRET`, a real `POSTGRES_PASSWORD`,
and restrict `CORS_ORIGIN` to your actual panel address.
:::

## Verifying the installation

1. Open the API health check: [http://localhost:4000/api/health](http://localhost:4000/api/health). It should return a success response.
2. Open the panel at the frontend address and log in with the superadmin password from the log.
3. If login works and the dashboard loads, the installation is healthy.

## Troubleshooting

- **Port already in use**: change `FRONTEND_PORT`, `BACKEND_PORT`, or `DB_PORT` in `.env` and run `docker compose up -d` again.
- **Cannot find the password**: re-run the log grep, or for a clean start use `docker compose down -v` and bring the stack back up so the superadmin is bootstrapped again.
- **Backend cannot reach the database**: make sure the `db` container is healthy (`docker compose ps`); the backend waits for it but a stale volume can cause issues, in which case reset with `docker compose down -v`.
