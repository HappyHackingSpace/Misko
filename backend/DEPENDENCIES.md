# Dependency baseline — 2026-09-10

Versions were resolved against official release/module endpoints on this date.
Builds and CI use pinned versions, not `@latest`.

| Component | Version | Source |
|---|---|---|
| Go | 1.27.1 | https://go.dev/dl/?mode=json |
| pgx/v5 | v5.11.0 | https://proxy.golang.org/github.com/jackc/pgx/v5/@latest |
| x/sync (indirect) | v0.23.0 | https://proxy.golang.org/golang.org/x/sync/@latest |
| x/text (indirect) | v0.42.0 | https://proxy.golang.org/golang.org/x/text/@latest |
| govulncheck | v1.8.0 | https://proxy.golang.org/golang.org/x/vuln/@latest |
| actions/setup-go | v7.0.0 | https://github.com/actions/setup-go/releases/latest |
| actions/checkout | v7.0.1 | https://github.com/actions/checkout/releases/latest |
| PostgreSQL | 18.6-alpine | https://www.postgresql.org/versions.json |

`go.mod`/`go.sum` record the complete dependency graph. Go's module checksum
verification remains enabled. Docker pins both release tags and the verified Go/PostgreSQL manifest digests. Standard `net/http`, `slog`,
`testing`, `httptest`, `go vet` and `gofmt` are used without extra frameworks.
No unused JWT, GCS, SQL generator or CV packages are preinstalled; each owning
issue resolves and pins its then-current stable dependencies with tests.
