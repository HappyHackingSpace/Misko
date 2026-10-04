# 🐭 Mişko

Behavioral test management system for laboratory mice.

Mişko manages, from a single place, the behavioral experiments run on mice in
biology labs: subjects and their weights, experiments and the groups inside
them, interventions, protocols and environments, the tests themselves, the
videos recorded for each test, and the measurements a computer vision worker
produces from those videos.

[![Mişko yayında! 🐭](https://i.ytimg.com/vi/CGzqYK24f88/hqdefault.jpg)](https://youtu.be/CGzqYK24f88)

> **This branch is a rebuild.** The backend is Go, and
> [backend/README.md](backend/README.md) documents every implemented endpoint in
> detail. No release or deployment should be made before the
> [rebuild epic](https://github.com/HappyHackingSpace/Misko/issues/123) is
> complete: video storage has not been exercised against a real bucket yet, and
> the analysis worker has only been tested on synthetic video.

## Tech stack

| Layer    | Technology                                        |
|----------|---------------------------------------------------|
| Backend  | Go 1.27 · pgx · sqlc · PostgreSQL 18               |
| Frontend | Vue 3 · Vite · Vue Router · Pinia · vue-i18n       |
| Worker   | Python · PyAV · NumPy · SciPy                      |
| Storage  | Google Cloud Storage (private bucket, signed URLs) |
| Infra    | Docker · Docker Compose · Nginx · GitHub Actions   |

## Repository layout

```
Misko/
├── docker-compose.yml       # local stack: db + schema + setup + backend + jobs + frontend
├── docker-compose.prod.yml  # pull-based deployment from prebuilt images
├── .env.example             # variables docker-compose.yml reads
│
├── backend/                 # Go API (modular monolith, clean architecture)
│   ├── cmd/api              # the HTTP server
│   ├── cmd/bootstrap        # one-time: install the schema, create the laboratory
│   ├── cmd/jobs             # queues analysis runs, requeues expired leases
│   ├── internal/<domain>/   # domain / application / adapters per domain
│   ├── schema/              # numbered SQL files, installed as a whole
│   └── tests/               # architecture policy and end to end harness
│
├── frontend/                # Vue 3 panel (Nginx serves it, proxies /api)
├── worker/                  # Open Field analysis worker (Python)
└── docs-site/               # documentation published to GitHub Pages
```

## Quick start (Docker)

The whole stack with three commands. The schema and the first administrator are
installed explicitly, so an existing database is never migrated by accident.

```bash
cp .env.example .env                                        # optional: every value has a default
docker compose --profile setup run --rm schema              # install the schema
docker compose --profile setup run --rm setup               # create the laboratory and the administrator
docker compose up -d                                        # start the API, the jobs process and the panel
```

The `setup` step prints the generated administrator password to your terminal
**once**. It is never written to a log. Copy it before closing the terminal; to
receive it as a file instead, add `-credentials-file /out/admin.txt` and mount a
folder at `/out`.

- Panel: http://localhost:8080
- API: http://localhost:4000/api (health at `/api/health`, readiness at `/api/ready`)

Sign in with `ADMIN_EMAIL` (default `admin@misko.local`) and that password, then
change it and create the other users from the **Users** screen.

Video upload and playback need a private Google Cloud Storage bucket. Without
`GCS_BUCKET` those routes answer 503 and the rest of the system works normally.

## Local development

Run PostgreSQL in Docker and the applications from source.

```bash
docker compose up -d db
```

**Backend** (one terminal). The Go processes do not read `.env` files, so export
the variables; `backend/.env.example` lists them.

```bash
cd backend
export DATABASE_URL='postgres://misko:misko_local_only@127.0.0.1:5432/misko_foundation?sslmode=disable'
export JWT_SECRET="$(openssl rand -base64 48)"
go run ./cmd/bootstrap schema
go run ./cmd/bootstrap setup
go run ./cmd/api                      # http://localhost:4000
```

**Frontend** (a second terminal):

```bash
cd frontend
npm install
npm run dev                           # http://localhost:5173, /api proxied to :4000
```

## Commands

### Backend (`make` in `backend/`)

| Command | Description |
|---------|-------------|
| `make check` | gofmt, `go vet` (with and without the integration tag), `sqlc diff`, tests with the race detector, `govulncheck` |
| `make test` | Unit tests with the race detector |
| `make integration` | Integration tests against a real PostgreSQL |
| `make generate` | Regenerate the sqlc query code |
| `make build` | Build the `api`, `bootstrap` and `jobs` binaries |

### Frontend (`npm` in `frontend/`)

| Command | Description |
|---------|-------------|
| `npm run dev` | Vite development server |
| `npm run build` | Production build |
| `npm run lint` | ESLint |
| `npx playwright test` | Browser tests against the end to end API server |

### Worker (`worker/`)

| Command | Description |
|---------|-------------|
| `pytest` | Worker tests, including the cross-language metric fixture |

## API

Every endpoint, its permissions and its rules are documented in
[backend/README.md](backend/README.md). In short: sign in at
`POST /api/auth/login`, send `Authorization: Bearer <token>` afterwards, and
analysis workers use `Authorization: Worker <token>` on their own routes. List
endpoints return `{ "data": [...], "total": n, "page": n, "pageSize": n }`.

## Documentation

The published documentation lives in `docs-site/` and is deployed to
[GitHub Pages](https://happyhackingspace.github.io/Misko/) in English and
Turkish. Deployment from prebuilt images is described in
[docs/DEPLOY.md](docs/DEPLOY.md).

## CI

Path-filtered GitHub Actions pipelines:

- **Backend CI** with a PostgreSQL service: `make check`, `make integration`,
  `make build`, and a container build.
- **Frontend CI**: lint, build, browser tests against the end to end API server,
  and a container build.
- **Worker CI**: the Python worker's tests.
- **Workflows CI**: every workflow file is linted with actionlint, so a broken
  workflow fails a check instead of failing a deployment.
- **Docs**: the documentation site is published on changes under `docs-site/`.

## Roadmap

The phased plan is in [docs/ROADMAP.md](docs/ROADMAP.md), and the rebuild is
tracked in [issue #123](https://github.com/HappyHackingSpace/Misko/issues/123).

## License

[MIT](LICENSE) © Happy Hacking Space
