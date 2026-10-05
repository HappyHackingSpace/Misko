# Contributing to Mişko

Thank you for helping. Mişko is an open source behavioral test system for
laboratory mice, used by real laboratories, so a wrong number is worse than a
missing feature. This guide explains how the project is organized and how to get
a change merged.

Turkish and English are both welcome in issues and pull requests.

## Where things live

| Area | Folder | Stack | Detailed docs |
|---|---|---|---|
| API and metric engine | `backend/` | Go, PostgreSQL, sqlc | [backend/README.md](backend/README.md) |
| Panel | `frontend/` | Vue 3, Vite, Pinia, vue-i18n | `frontend/tests/e2e` for examples |
| Video analysis worker | `worker/` | Python, PyAV, NumPy, SciPy | [worker/README.md](worker/README.md) |
| Published documentation | `docs-site/` | Astro, English and Turkish | |
| Deployment | `docker-compose*.yml`, `.github/workflows` | Docker, GitHub Actions | [docs/DEPLOY.md](docs/DEPLOY.md) |

Start with the [README](README.md) to run the stack, then read the section of
[backend/README.md](backend/README.md) for the domain you are changing.

Some files under `docs/` describe the earlier Node implementation and carry a
notice at the top. The Go code and `backend/README.md` are the source of truth.

## Rules that protect scientific results

These are not style preferences. Reviewers will ask for them.

1. **Paradigms, metrics, units and events are code.** They live in
   `backend/internal/paradigms/domain` and are never editable at runtime.
2. **A published definition never changes.** If a formula, threshold or
   zone rule must change, publish a new paradigm version (and bump
   `MetricEngineVersion` when an existing calculation changes) instead of editing
   version 1. The golden manifests in
   `backend/internal/paradigms/adapters/http/testdata/manifests` fail when a
   published version changes. Do not regenerate them to make a test pass.
3. **The worker measures, the Go engine computes.** The Python worker produces a
   trajectory only. Metrics and events come from the Go engine so the same
   formula is never implemented in two languages.
4. **A missing value is never zero.** Report the reason (`NO_VALID_INTERVALS`,
   `NOT_SCORED`, and so on) instead of inventing a number.
5. **Behavioral results are data, not a verdict.** Quality control (QC) says
   whether tracking is reliable, not whether an animal passed.
6. **Completed analyses are immutable.** A correction creates a new run.
7. **Layers keep their direction.** `domain` does not import `application` or
   `adapters`. `backend/tests/architecture` enforces this in CI.
8. **Role checks are enforced on the server.** Hiding a button is not
   authorization. A new endpoint needs an allowed and a denied role test.

If your change touches a metric, a formula or the tracker, say so in the pull
request and include a hand-calculated example or a comparison with a manually
scored video.

## Getting started

Prerequisites: Docker, Go (the version in `backend/go.mod`), Node 22 and, for the
worker, Python 3.14.

```bash
git clone https://github.com/HappyHackingSpace/Misko.git
cd Misko
docker compose up -d db
```

The complete local setup, including the schema and the first administrator, is
in the [README](README.md#local-development).

## Before you open a pull request

Run the checks of the folders you changed. CI runs the same commands, but
failing locally is faster.

```bash
# backend/
make check            # gofmt, go vet, sqlc diff, race tests, govulncheck
make integration      # needs TEST_DATABASE_URL, see backend/README.md

# frontend/
npm run lint
npm run build
npx playwright test   # browser tests against the end to end API server

# worker/
python -m pytest -q
```

If you change a SQL query, run `make generate` and commit the generated code.
`sqlc diff` fails when it is stale.

Every user-visible text goes in both `frontend/src/i18n/locales/en.js` and
`tr.js`.

Write a test with the change. Domain rules need a unit test, database behavior
needs an integration test, and a panel flow needs a browser test. Tests that only
count mock calls or exercise getters are not helpful.

## Branches and commits

- Branch from `main`: `feat/short-name`, `fix/short-name`, `docs/short-name`,
  `chore/short-name`.
- Never push to `main`; every change goes through a pull request.
- Commit messages follow the style already in the history:
  `type(scope): what changed`, in the imperative.

  ```
  fix(frontend): show the unit of ratio metrics as percent
  feat(backend): record scored events for novel object tests
  docs: describe how a worker is registered
  ```

  Types in use: `feat`, `fix`, `docs`, `test`, `chore`, `deploy`.
  Scopes in use: `backend`, `frontend`, `worker`, `deploy`.
- Keep a pull request to one purpose. A small pull request is reviewed in
  minutes; a large one waits.

## Pull requests

1. Fill in the pull request template.
2. Link the issue (`Closes #123`).
3. Wait for CI. Backend, frontend, worker and workflow checks run when their
   folders change.
4. One approval from a code owner is required. Reviewers may ask for changes;
   please answer each comment or push a fix.
5. The author merges after approval and green checks, using squash merge so that
   `main` keeps one commit per change.

## Choosing something to work on

Issues labeled `good first issue` are small and self-contained. If you want
something bigger, comment on the issue first so two people do not do the same
work. For a new feature, open an issue and describe the problem before writing
code.

Files that change often (`worker/misko_worker/tracking.py`,
`backend/internal/paradigms/domain`) should have one person working on them at a
time. Ask in the issue.

## Reporting a problem

- A bug or a wrong measurement: open an issue with the matching template.
  For a measurement problem, say what you expected, how you measured it by hand
  and what Mişko reported.
- A security problem: do **not** open a public issue. Follow
  [SECURITY.md](SECURITY.md).

## Security and secrets

Never commit credentials, tokens, service account files or real recordings of
people. `.env` files stay local; `.env.example` holds placeholders only. The
worker token and signed URLs must never appear in logs.

## License

By contributing you agree that your work is released under the
[MIT License](LICENSE) of this project.
