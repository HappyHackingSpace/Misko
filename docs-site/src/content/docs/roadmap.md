---
title: Roadmap
description: The phased plan — get the backbone working end to end first, then add flesh.
---

Philosophy: **get the backbone working end to end first** (even with fake data),
then add flesh in each phase. The two systems (Mişko + CV service) stay
independent from the start; their only contact point is the
[integration contract](../integration/).

## Phase 0 — Identity & internal SaaS model ✅

Mişko is an **internal (org-only) SaaS**, deployed on-prem. No public sign-up.

- Superadmin bootstrap at Docker startup (system-generated strong password,
  printed once; idempotent).
- Sign-up removed (frontend + backend).
- Internal user management (admin-only).

## Phase 1 — Foundation: tenancy + RBAC + paradigm registry

- **Tenancy (`Laboratory`, single-tenant):** exactly one lab per installation,
  created by an installation wizard with a singleton guard.
- **RBAC (5 roles, code matrix):** SUPERADMIN, LAB_MANAGER, RESEARCHER,
  TECHNICIAN, VIEWER with a code-defined permission matrix.
- **Paradigm registry (hardcoded, SOLID):** `ParadigmSpec` registry (MWM,
  OPEN_FIELD, EPM, ROTAROD); the UI form auto-renders from a paradigm's parameters.
- **`LabParadigm`:** admins toggle which paradigms are active per lab.

## Phase 2 — Domain model (the science)

- Rich `Subject` + `WeightLog` time series.
- `DiseaseModel` & `Treatment` catalogs + N–N joins.
- `Apparatus` — physical rig (geometry in cm, surface color/material, zones).
- `Study → Group` + longitudinal testing.

## Phase 3 — Backbone: boundary contract (with a fake CV)

- `POST /api/tests/:id/result` (service auth, idempotent).
- Calibration wired into Test.
- CV service skeleton with its own PostgreSQL and a stub result push.
- MinIO for object storage.

## Phase 4 — Scenario intelligence

- Scenario/apparatus geometry editor (zone drawing, cm scale).
- Per-paradigm acceptance criteria + metric engine.

## Phase 5 — Real CV

- `local_usb` adapter (OpenCV) + YOLOv8 + ByteTrack.

## Phase 6 — Live monitoring & multiple sources

- CV → Vue panel via WebSocket/SSE (bypassing Mişko).
- `rtsp/http` and `ws_push` (phone) adapters.

## Phase 7 — Deployment

- Image registry (GHCR) + staging/prod deployment.
- (Optional) TimescaleDB on the CV side.

---

**Boundary reminder:** raw telemetry, events and video stay in the **CV
service**; Mişko keeps only **summary metrics + artifact URLs**.
