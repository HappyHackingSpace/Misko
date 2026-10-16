# Panel work on the fresh schema

Working agreement for the parallel rewrite of the Vue panel (`frontend/`) onto
the ported API. Read with [DOMAIN_REDESIGN.md](DOMAIN_REDESIGN.md) and
[PORT_CONTRACT.md](PORT_CONTRACT.md). The API is described in
`backend/api/openapi.yaml` and implemented under `backend/internal/*/adapters/http`;
read the handler when the spec is unclear.

## Rules

1. Own worktree and branch from `feat/fresh-schema`:
   `git -C /Users/recep.kizilarslan/Workspace/Repositories/Misko-schema worktree add ../Misko-fe-<name> -b fe/<name> feat/fresh-schema`.
   Run `npm ci` in its `frontend/`. Commit there; never push; never touch other worktrees.
2. Own only the files listed for your package. Shared files are already
   prepared: routes in `src/router.js`, the menu in `src/App.vue`, the
   experiment page shell `src/views/experiments/ExperimentDetail.vue` (tabs,
   with `inject("study")` giving `{ experiment, reload, id }`). If you need a
   change in a shared file, keep it to a few lines and say so in the report.
3. Translations: never edit `src/i18n/locales/en.js` or `tr.js`. Put your
   messages in `src/i18n/locales/en/<area>.js` and `src/i18n/locales/tr/<area>.js`
   (merged on top by `src/i18n/index.js`), same keys in both. Every visible text
   is translated; Turkish is the default language. Translate every API error
   code your screens can receive under `errors.<code>` (for example
   `errors: { experiment: { notReady: "..." } }`).
4. API calls go through `src/api/client.js` (`api(path, {method, body, query})`).
   Put your calls in your own module `src/api/<area>.js`.
5. UI: Naive UI components, the existing `PageHead`, theme tokens (`var(--muted)`,
   `var(--line)`, `var(--accent)` ...), no hard-coded colours. One pattern
   everywhere:
   - lists are `NDataTable` with server paging where the API pages, a search
     box, empty and loading states;
   - create and edit in an `NDrawer` or `NModal` form, not a separate page,
     unless the form is large;
   - success shows `useMessage().success(...)`, errors show the translated
     message next to the form;
   - destructive actions ask for confirmation (`NPopconfirm`);
   - enums are shown in words, never as raw keys; dates and numbers use the
     current locale; doses show amount and unit (`10 mg/kg`).
   - Mobile width works (no horizontal page scroll).
   - Write controls are hidden when `useAuth().can(permission)` is false.
6. Remove the old screens and components your area replaces.
7. Verify for real. Start your own stack:
   ```bash
   docker exec misko-nb-pg psql -U postgres -c "CREATE DATABASE misko_fe_<name>"
   cd backend
   export DATABASE_URL="postgres://postgres:test@127.0.0.1:55433/misko_fe_<name>?sslmode=disable"
   go run ./cmd/bootstrap migrate
   ADMIN_EMAIL=admin@misko.local LAB_NAME="Dev lab" go run ./cmd/bootstrap setup -credentials-file /tmp/misko-fe-<name>.txt
   HTTP_ADDR=127.0.0.1:<api port> JWT_SECRET=$(openssl rand -base64 48) go run ./cmd/api &
   cd ../frontend && VITE_API_PROXY=http://127.0.0.1:<api port> npx vite --port <ui port>
   ```
   Drive the screens with a small Playwright script (`npx playwright` is
   installed; chromium can be installed with `npx playwright install chromium`)
   or with the browser tools if you have them: create real data through the UI,
   check every screen in Turkish and English, then stop your servers.
   Run `npm run lint` and `npm run build`.
8. The Playwright suite in `frontend/tests/e2e` and its server
   (`backend/tests/e2e/main.go`) are rewritten later in one go; you may delete
   specs your area made obsolete, and list in your report the flows a new spec
   should cover.
9. Never use the em dash character. Final report: screens built, routes used,
   anything the API was missing, commit hash.

## Packages and ports

| Package | Branch | API / UI port | Owns |
|---|---|---|---|
| FA catalog, subjects, environments | `fe/catalog` | 4101 / 5201 | `src/views/catalog/*`, `src/views/Subjects.vue`, `src/views/SubjectForm.vue` (or a subjects folder), environment views (field `parameters`), `src/api/catalog.js`, `src/api/subjects.js`, locale files `catalog.js`, `subjects.js`, `environments.js` |
| FB experiment design | `fe/design` | 4102 / 5202 | `src/views/Experiments.vue`, `src/views/ExperimentForm.vue`, tabs `Overview`, `Design`, `Timeline`, `Endpoints`, `Animals`, `src/api/experiments.js`, `src/api/protocols.js`, locale file `design.js` |
| FC execution | `fe/execution` | 4103 / 5203 | tabs `Dosing`, `Tests`, `src/views/Tests.vue`, `src/views/TestDetail.vue` and `src/components/test/*` except result and run panels, video upload and calibration UI, `src/api/dosing.js`, `src/api/tests.js`, `src/api/media.js`, locale file `execution.js` |
| FD analysis and reports | `fe/analysis` | 4104 / 5204 | tab `Results`, `src/views/review/*`, `src/views/Reports.vue`, `src/views/Dashboard.vue`, result and run panels, `src/api/analysis.js`, locale file `analysis.js` |
