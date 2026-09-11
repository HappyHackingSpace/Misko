# Dependency baseline

Versions were resolved against official release/module endpoints on the listed
date. Builds and CI use pinned versions, not `@latest`.

| Component | Version | Verified | Source |
|---|---|---|---|
| Go | 1.27.1 | 2026-09-10 | https://go.dev/dl/?mode=json |
| pgx/v5 | v5.11.0 | 2026-09-10 | https://proxy.golang.org/github.com/jackc/pgx/v5/@latest |
| golang-jwt/jwt/v5 | v5.3.1 | 2026-09-10 | https://proxy.golang.org/github.com/golang-jwt/jwt/v5/@latest |
| x/crypto (bcrypt) | v0.57.0 | 2026-09-10 | https://proxy.golang.org/golang.org/x/crypto/@latest |
| x/sync (indirect) | v0.23.0 | 2026-09-10 | https://proxy.golang.org/golang.org/x/sync/@latest |
| x/text (indirect) | v0.42.0 | 2026-09-10 | https://proxy.golang.org/golang.org/x/text/@latest |
| sqlc (tool) | v1.31.1 | 2026-09-10 | https://github.com/sqlc-dev/sqlc/releases/latest |
| govulncheck (tool) | v1.8.0 | 2026-09-10 | https://proxy.golang.org/golang.org/x/vuln/@latest |
| actions/setup-go | v7.0.0 | 2026-09-10 | https://github.com/actions/setup-go/releases/latest |
| actions/checkout | v7.0.1 | 2026-09-10 | https://github.com/actions/checkout/releases/latest |
| PostgreSQL | 18.6-alpine | 2026-09-10 | https://www.postgresql.org/versions.json |
| cloud.google.com/go/storage | v1.67.1 | 2026-09-11 | https://proxy.golang.org/cloud.google.com/go/storage/@latest |
| google.golang.org/api (indirect) | v0.287.1 | 2026-09-11 | resolved by `go get cloud.google.com/go/storage@v1.67.1` |
| google.golang.org/grpc (indirect) | v1.82.1 | 2026-09-11 | resolved by `go get cloud.google.com/go/storage@v1.67.1` |

`go.mod`/`go.sum` record the complete dependency graph. Go's module checksum
verification remains enabled. Docker pins both release tags and the verified
Go/PostgreSQL manifest digests. Tools run through `go run module@version` in the
Makefile, so their versions are pinned without a separate install step; sqlc
needs cgo and a C compiler. Standard `net/http`, `slog`, `testing`, `httptest`,
`go vet` and `gofmt` are used without extra frameworks. The GCS client arrived with #132; no unused CV
packages are preinstalled; each owning issue resolves and pins its then-current
stable dependencies with tests. The schema uses `uuidv7()`, which requires
PostgreSQL 18 or later, and the trusted `btree_gist` contrib extension shipped
with PostgreSQL for the group assignment exclusion constraint.
