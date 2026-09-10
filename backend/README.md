# Go backend

Integration branch `new-backend`. The rebuild is incomplete; do not release it.

| Issue | Implemented |
|---|---|
| [#124](https://github.com/HappyHackingSpace/Misko/issues/124) | Liveness/readiness, graceful shutdown, fresh schema installation, architecture checks |
| [#125](https://github.com/HappyHackingSpace/Misko/issues/125) | Login, current user, own password change, user management, laboratory singleton, RBAC, idempotent setup |

Subjects, experiments, paradigms, GCS uploads and video analysis are separate
following issues. The Vue application is not wired to this API yet; its old
business routes return 404. There is no legacy API adapter or data migration.

## Layout and boundaries

- `cmd/api`: process entry, signals, exit status.
- `cmd/bootstrap`: `schema` installs the SQL schema into an empty database; `setup` creates the laboratory and first administrator.
- `internal/bootstrap`: composition root; constructor wiring of concrete adapters.
- `internal/access/domain`: the shared RBAC kernel (roles, permission matrix, authenticated actor).
- `internal/identity`: users, passwords, tokens and user administration.
- `internal/laboratory`: the laboratory singleton.
- `internal/health`: readiness use case and probes.
- `internal/platform`: configuration, HTTP server lifecycle, JSON helpers, serializable transactions and the integration-test database helper. No business rules.
- `schema`: embedded SQL files, applied in lexical order in one transaction; never run by the API.
- `sqlc.yaml`, `internal/*/adapters/postgres/queries.sql`: SQL sources for the generated `sqlcgen` packages. Do not edit generated code; run `make generate`.
- `tests/architecture`: source import checks with positive and negative policy fixtures.

Domain packages allow pure standard-library dependencies and their own
subpackages. Application packages add context/synchronization and their own
domain. `internal/access/domain` is the only domain package other domains may
import, so every use case authorizes against the same matrix. Adapters may use
any domain or application package but never another domain's adapters: other
HTTP adapters resolve the caller through `identityhttp.Authenticator`, a function
wired in the composition root. `net/http`, `database/sql`, JWT, bcrypt, pgx and
GCP packages cannot enter inner layers. The architecture test scans every Go file
under `internal`, including inactive build tags.

## Roles and permissions

The five roles and their permissions are static code, not database rows.

| Permission | SUPERADMIN | LAB_MANAGER | RESEARCHER | TECHNICIAN | VIEWER |
|---|---|---|---|---|---|
| `user:manage` | Yes | Yes | No | No | No |
| `lab:configure` | Yes | Yes | No | No | No |
| `study:write` | Yes | Yes | Yes | No | No |
| `subject:write` | Yes | Yes | Yes | No | No |
| `weight:write` | Yes | Yes | Yes | Yes | No |
| `apparatus:write` | Yes | Yes | Yes | No | No |
| `test:write` | Yes | Yes | Yes | No | No |
| `test:run` | Yes | Yes | Yes | Yes | No |
| `*:read` | Yes | Yes | Yes | Yes | Yes |

| Route | Required access |
|---|---|
| `GET /api/health`, `GET /api/ready`, `GET /api/meta`, `POST /api/auth/login` | Public |
| `GET /api/auth/me`, `POST /api/auth/password` | Any signed-in user |
| `GET, POST /api/users`; `GET, PATCH, DELETE /api/users/{id}`; `POST /api/users/{id}/reset-password` | `user:manage` |
| `GET /api/lab` | `*:read` |
| `PATCH /api/lab` | `lab:configure` |

Use cases check the actor and permission themselves; HTTP handlers only translate.
Rules enforced by tests:

- Tokens are HS256 JWTs with issuer, audience, issued-at and expiry, carrying the
  user id and a session version but no role. Other algorithms are rejected.
- The current role is loaded from PostgreSQL on every request, so a role change
  applies to the next request and deleted users get 401 immediately.
- Resetting or changing a password increments the session version and revokes
  earlier tokens. `POST /api/auth/password` returns a replacement token.
- 401 means missing, malformed, expired or revoked credentials; 403 means the
  current role lacks the permission.
- Users cannot delete themselves. The last SUPERADMIN or LAB_MANAGER cannot be
  deleted or demoted to an unprivileged role; checks run in a SERIALIZABLE
  transaction with bounded retries, verified with concurrent requests.
- Passwords need at least 8 characters and at most 72 UTF-8 bytes (the bcrypt
  limit; longer input is rejected, never truncated). They are compared byte for
  byte without normalization. Generated passwords are 26 random base32 characters.
- Unknown accounts and wrong passwords return the same 401 after a bcrypt comparison.
- There is no signup route. Setup creates an administrator only when no
  privileged user exists, so repeated runs create nothing.
- Unexpected errors log the route pattern and error only, never headers, bodies,
  tokens or passwords. Clients receive a stable `code` and a generic message.

## Local container run

From the repository root:

```sh
docker compose up -d db
docker compose --profile setup run --rm schema
docker compose --profile setup run --rm setup
docker compose up --build -d backend
curl --fail http://127.0.0.1:4000/api/ready
```

`setup` prints the administrator email and generated password once to your
terminal; change the password after signing in. When stdout is not a terminal
(CI, jobs), setup refuses to start unless `-credentials-file PATH` names a new
file, which is created with mode 0600. The password is never written to logs.

```sh
curl -X POST http://127.0.0.1:4000/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"email":"admin@misko.local","password":"PASSWORD_FROM_SETUP"}'
curl http://127.0.0.1:4000/api/auth/me -H 'Authorization: Bearer TOKEN'
```

The Compose project `misko-new-backend` uses a new `foundation-db` volume; it
does not mount the old installation's volume. Credentials and `JWT_SECRET` in
`docker-compose.yml` are development-only and the API binds to loopback. No
frontend or GCP deployment is started. Re-running `schema` refuses an existing
`misko` schema; it never drops data.

## Run Go directly

Use the version in `go.mod` and a dedicated empty PostgreSQL database. Export
the variables below (see `.env.example`), then from `backend/`:

```sh
go run ./cmd/bootstrap schema
go run ./cmd/bootstrap setup
go run ./cmd/api
```

| Variable | Default | Used by | Meaning |
|---|---|---|---|
| `DATABASE_URL` | required | all | PostgreSQL URL with host and database |
| `HTTP_ADDR` | `:4000` | api | Listen address |
| `PROBE_TIMEOUT` | `2s` | api | Readiness probe and connect timeout |
| `SHUTDOWN_TIMEOUT` | `10s` | api | Request drain period on SIGTERM |
| `JWT_SECRET` | required | api | HS256 key, at least 32 bytes |
| `JWT_ISSUER` / `JWT_AUDIENCE` | `misko` / `misko-api` | api | Required token claims |
| `TOKEN_TTL` | `12h` | api | Token lifetime, 5m to 168h |
| `BCRYPT_COST` | `12` | api, setup | bcrypt cost, 10 to 14 |
| `ADMIN_EMAIL` | required | setup | First administrator |
| `LAB_NAME` | required | setup | Laboratory name |
| `LAB_TIMEZONE` | `UTC` | setup | IANA time zone |

The processes do not load `.env` files or change the schema on startup.
Configuration errors never echo the database URL or signing secret. Shutdown
marks readiness unavailable, stops accepting requests, and drains them for
`SHUTDOWN_TIMEOUT`; on expiry connections are forced closed and the process fails.

## Verification

```sh
make check
TEST_DATABASE_URL='postgres://user:password@localhost/empty_db?sslmode=disable' make integration
make build
```

`make check` runs gofmt, `go vet` (with and without the integration tag),
`sqlc diff`, race-enabled unit/HTTP/architecture tests and govulncheck. After
editing schema or query SQL, run `make generate` and commit the result.

Integration tests need a PostgreSQL server where the user may create databases.
`TEST_DATABASE_URL` must name an empty database: the schema tests install into it
and deliberately leave their fixture. Identity, laboratory and end-to-end HTTP
tests create uniquely named `misko_test_*` databases and drop only those. They
cover concurrent last-administrator protection, concurrent setup, SQL constraints,
search escaping, stable paging and the full RBAC matrix over HTTP.

CI runs these checks on PRs, including PRs targeting `new-backend`, then builds the
container without pushing an image. Publication jobs are restricted to `main`;
this unfinished branch cannot be manually deployed or released. No auto-merge is
enabled for `new-backend`.

Known limits: login has no rate limiting yet, and there is no server-side logout
(a client signs out by discarding its token; password changes revoke all tokens).

[OpenAPI](api/openapi.yaml) describes only implemented endpoints.
[Dependencies](DEPENDENCIES.md) records pinned versions and verification sources.
