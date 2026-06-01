# Mişko — Roadmap (phased)

> Philosophy: **get the backbone working end to end first** (even with fake
> data), then add flesh in each phase. The two systems (Mişko + CV service)
> stay **independent** from the start; their only contact point is the contract
> in `docs/INTEGRATION.md`.

## Phase 0 — Identity & internal SaaS model (do this first)

Mişko is an **internal (org-only) SaaS** application. There is **no public
sign-up**. User management is done entirely internally.

- **Superadmin bootstrap (at Docker startup):** when the project first comes up,
  the superadmin **email** is provided via an environment variable
  (`ADMIN_EMAIL`); the system **generates a strong password** and writes it to
  the startup log **once**. If an admin already exists, this is a **no-op**
  (idempotent).
- **Sign-up removed:** the frontend "Register" flow and the backend public
  `POST /api/auth/register` endpoint are removed. Only **login** (`/login`) and
  **session** (`/me`) remain.
- **Internal user management:** new users are created only by an **ADMIN**
  (`/api/users`, admin-only). Username/email + password are assigned by the
  admin; the password can be reset by the admin.
- Roles: `ADMIN` (user management + everything) and `OPERATOR` (test/subject
  management).

## Phase 1 — Backbone: make the boundary contract work (with a fake CV)

- Mişko: `POST /api/tests/:id/result` (service-to-service `X-Service-Key` auth,
  **idempotent** via `captureSessionId`). `metrics → Test.result`,
  `passed → Test.passed`, status `DONE`.
- `result` / `Scenario.config` stored as **Jsonb** in PostgreSQL.
- CV service skeleton: `cv-service/` (monorepo), its **own PostgreSQL**,
  FastAPI `/health`, a **STUB** for the result push (no real vision yet).
- Object storage: add **MinIO** to compose (for video/artifact URLs).

## Phase 2 — Scenario intelligence

- Scenario **geometry editor** (area/zone drawing, cm scale) → `Scenario.config`.
- Per-scenario **acceptance criteria** and a **metric engine** (computing `passed`).

## Phase 3 — Real CV

- `local_usb` adapter (OpenCV) + **YOLOv8** (native) + **ByteTrack**.
- The CV service writes telemetry/events to its own DB; on test end it pushes a
  summary to Mişko.

## Phase 4 — Live monitoring & multiple sources

- CV → Vue panel via **WebSocket/SSE** (a separate channel that bypasses Mişko).
- `rtsp/http` and `ws_push` (phone getUserMedia) adapters.

## Phase 5 — Deployment

- Image registry (GHCR) push + staging/prod deployment.
- (Optional) **TimescaleDB** on the CV side for per-frame telemetry.

---

**Boundary reminder:** raw frame telemetry, events and video stay in the **CV
service**; Mişko only keeps **summary metrics + artifact URLs**. Details:
`docs/INTEGRATION.md`.
