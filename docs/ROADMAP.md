# Mişko — Roadmap (phased)

> Philosophy: **get the backbone working end to end first** (even with fake
> data), then add flesh in each phase. The two systems (Mişko + CV service)
> stay **independent** from the start; their only contact point is the contract
> in `docs/INTEGRATION.md`. The full data model is in `docs/DOMAIN.md`.

## Phase 0 — Identity & internal SaaS model ✅

Mişko is an **internal (org-only) SaaS**, deployed **on-prem**. No public sign-up.

- **Superadmin bootstrap** at Docker startup (`ADMIN_EMAIL`, system-generated
  strong password printed once; idempotent).
- **Sign-up removed** (frontend register flow + backend `POST /api/auth/register`).
- **Internal user management** (`/api/users`, admin-only).

## Phase 1 — Foundation: tenancy + RBAC + paradigm registry

The platform skeleton everything else hangs off of.

- **Tenancy (`Laboratory`, single-tenant):** exactly one lab per installation
  (singleton). An **installation wizard** (extends `bootstrap-admin.js`) takes the
  lab name + superadmin email and creates the `Laboratory` **and** the
  `SUPERADMIN` together; a singleton guard refuses a second lab.
- **RBAC (5 roles, code matrix):** `SUPERADMIN`, `LAB_MANAGER`, `RESEARCHER`,
  `TECHNICIAN`, `VIEWER`. Permissions are a code-defined `role → permissions`
  matrix; `requirePermission(...)` middleware. Existing ADMIN/OPERATOR users are
  migrated.
- **Paradigm registry (hardcoded, SOLID):** `ParadigmSpec` interface + registry
  (`MWM`, `OPEN_FIELD`, `EPM`, `ROTAROD`). Parameters/zones/metrics are declared
  in code; the UI form is auto-rendered from a paradigm's `parameters`.
- **`LabParadigm` (enable/disable per lab):** admin (`SUPERADMIN`/`LAB_MANAGER`)
  toggles which paradigms are active; the UI only offers enabled ones.

## Phase 2 — Domain model (the science)

Replace the thin model with the research-grade one from `docs/DOMAIN.md`.

- **Rich `Subject`** (strain, line/genotype, zygosity, sex, coat color, etc.) +
  **`WeightLog`** time series.
- **`DiseaseModel`** & **`Treatment`** catalogs + **N–N** joins to Subject.
- **`Apparatus`** — physical rig instantiating a paradigm's parameters (geometry
  in cm, surface color/material for CV contrast, zones).
- **`Study` → `Group`** + longitudinal testing (`Test.studyId` / `timepoint`).
- Migrations + seed updates + minimal CRUD UI per entity, each gated by the
  relevant permission.

## Phase 3 — Backbone: boundary contract (with a fake CV)

- Mişko: `POST /api/tests/:id/result` (service-to-service `X-Service-Key` auth,
  **idempotent** via `captureSessionId`). `metrics → Test.result`,
  `passed → Test.passed`, status `DONE`.
- **Calibration** wired into Test: fixed (on apparatus) + per-session override.
- CV service skeleton: `cv-service/` (monorepo), its **own PostgreSQL**,
  FastAPI `/health`, a **STUB** result push (no real vision yet).
- Object storage: add **MinIO** to compose (for video/artifact URLs).

## Phase 4 — Scenario intelligence

- Scenario/apparatus **geometry editor** (zone drawing, cm scale).
- Per-paradigm **acceptance criteria** + **metric engine** (computing `passed`).

## Phase 5 — Real CV

- `local_usb` adapter (OpenCV) + **YOLOv8** (native) + **ByteTrack**.
- The CV service writes telemetry/events to its own DB; on test end it pushes a
  summary to Mişko.

## Phase 6 — Live monitoring & multiple sources

- CV → Vue panel via **WebSocket/SSE** (a separate channel that bypasses Mişko).
- `rtsp/http` and `ws_push` (phone getUserMedia) adapters.

## Phase 7 — Deployment

- Image registry (GHCR) push + staging/prod deployment.
- (Optional) **TimescaleDB** on the CV side for per-frame telemetry.

---

**Boundary reminder:** raw frame telemetry, events and video stay in the **CV
service**; Mişko only keeps **summary metrics + artifact URLs**. Details:
`docs/INTEGRATION.md`.
