# Go backend foundation

Issue [#124](https://github.com/HappyHackingSpace/Misko/issues/124), targeting `new-backend`.
This is the first implementation step, not a complete application. Only liveness
and readiness endpoints exist. Authentication, RBAC, subjects, experiments,
paradigms, GCS uploads and video analysis are separate following issues.

The Vue application is intentionally not wired into this foundation: its old
login and business routes will return 404 until their new implementations land.
There is no legacy API adapter or data migration. The scientific definitions and
RBAC reference remain available in Git at `d480e2f64bdedfeacfd9ed4a72a0241c4dc8632e`.

## Layout and boundaries

- `cmd/api`: process entry, signals, exit status.
- `cmd/bootstrap`: explicit fresh SQL schema installation; no user creation yet.
- `internal/bootstrap`: constructor wiring; owns concrete dependencies.
- `internal/health/application`: readiness use case and consumer-owned dependency port.
- `internal/health/adapters/http`: HTTP protocol and JSON representation.
- `internal/health/adapters/postgres`: PostgreSQL readiness implementation.
- `internal/platform`: configuration and bounded HTTP lifecycle, no business rules.
- `schema`: embedded fresh-install SQL; never run implicitly by the API.
- `tests/architecture`: source import checks and positive/negative policy fixtures.

Business `domain` packages are added when their rules are implemented; empty
packages and artificial entities are not created for folder symmetry. The
architecture test checks all Go sources under `internal`, even inactive build
tags. Domain packages allow pure standard-library dependencies and their own
subpackages only. Application packages add context/synchronization and their own
domain; adapters cannot reach another domain's adapters. `net/http`,
`database/sql`, GCP SDKs and other I/O dependencies cannot enter inner layers.
Tests run without PostgreSQL unless the `integration` tag is explicitly set.

## Local container run

From the repository root:

```sh
docker compose up -d db
docker compose --profile setup run --rm schema
docker compose up --build -d backend
curl --fail http://127.0.0.1:4000/api/health
curl --fail http://127.0.0.1:4000/api/ready
```

The Compose project `misko-new-backend` uses a new `foundation-db` volume; it does
not mount the old installation's volume. Credentials are development-only and the
API binds to loopback. No frontend or GCP deployment is started. Re-running schema
installation refuses an existing `misko` schema; it never drops data. Only a
namespace exists at this step; domain tables arrive with their owning issues.

## Run Go directly

Use the version in `go.mod`. Export `DATABASE_URL` pointing to a dedicated empty
PostgreSQL database, then from `backend/`:

```sh
go run ./cmd/bootstrap
go run ./cmd/api
```

The API does not load `.env` automatically or change the database on startup.
See `.env.example` for settings. Configuration errors never echo the database URL.
Readiness is bounded by `PROBE_TIMEOUT` (default 2s). Shutdown marks readiness
unavailable, stops accepting requests, and drains them for `SHUTDOWN_TIMEOUT`
(default 10s); on expiry connections are forced closed and the process fails.

## Verification

```sh
make check
TEST_DATABASE_URL='postgres://user:password@localhost/disposable_db?sslmode=disable' make integration
make build
```

Integration tests require a fresh disposable database and deliberately leave their
fixture in it. They never drop a schema: provide a new database for each run.
They verify missing-schema readiness, fresh installation, query cancellation and
non-destructive refusal to reinstall. Unit/HTTP tests cover dependency failure,
probe timeout/cancellation, invalid configuration, sanitized responses, graceful
and forced shutdown. Architecture fixtures prove both accepted and rejected imports.

CI runs these checks on PRs, including PRs targeting `new-backend`, then builds the
container without pushing an image. Publication jobs are restricted to `main`;
this unfinished branch cannot be manually deployed or released. The first release
workflow must be revisited in the final acceptance issue (tag publishing is gated
off in this foundation). No auto-merge is enabled for `new-backend`.

[OpenAPI](api/openapi.yaml) describes only implemented endpoints.
[Dependencies](DEPENDENCIES.md) records pinned versions and verification sources.
