---
title: Architecture
description: Layers, technology stack, and the two-system boundary.
---

Mişko is two independent systems joined by one thin contract. Mişko itself is a
layered Express API with a Vue 3 SPA; the CV service is a separate Python
service with its own database.

## Technology stack

| Layer    | Technology                                       |
|----------|--------------------------------------------------|
| Backend  | Node.js 20 · Express · Prisma · PostgreSQL       |
| Frontend | Vue 3 · Vite · Vue Router · Pinia                |
| Security | JWT · bcryptjs · helmet · zod                    |
| Infra    | Docker · Docker Compose · Nginx · GitHub Actions |
| CV svc   | Python · FastAPI · YOLOv8 · ByteTrack · OpenCV   |

## Backend layers

```
Request → routes → middleware (auth/validate) → controller → service → Prisma → DB
                                                    ↑
                                           asyncHandler + ApiError
                                                    ↓
                                         central errorHandler → JSON
```

Each module (`auth`, `users`, `scenarios`, `subjects`, `devices`, `tests`)
follows the same shape: `routes + controller + service (+ validation)`. A common
CRUD factory keeps the simple resources consistent.

## List queries

Every list endpoint returns a paginated envelope rather than a bare array:

```json
{ "data": [ ... ], "total": 42, "page": 1, "pageSize": 10 }
```

A shared helper (`common/listQuery.js`) turns the request query into Prisma
`where` / `orderBy` / `skip` / `take`, so search, per-column filters, sorting and
pagination are all applied by the database. Each module declares its own
`searchFields`, `filterFields` and `sortFields`; the static paradigm registry
uses the same shape over an in-memory array. Filters use bracket notation
(`filter[status]=DONE`), and `?all=true` bypasses pagination for the form
dropdowns that need a full list. On the frontend a single `DataTable` component
plus the `useDataTable` composable consume this envelope across all screens. The
component also renders a continuous row index and exports the current page
client-side: CSV via a generated blob and PDF via the browser print dialog, so
no PDF library is bundled.

## The two-system boundary

Mişko is the **system of record** for the lab workflow. The camera + vision side
is a **fully independent** service with its own PostgreSQL. Heavy data — frame
telemetry, events, video — stays in the CV service; Mişko keeps only the test's
**summary metrics** and artifact URLs.

```
Laboratory (singleton) ── Mişko API ── PostgreSQL (relational, small)
                              │
                              │  POST /api/tests/:id/result  (X-Service-Key)
                              ▼
                         CV Service ── PostgreSQL (telemetry, large)
                              │
                              ▼
                      Object storage (MinIO/S3) — video + artifacts
```

The full contract — test ↔ capture-session mapping, the result push body, and
service-to-service auth — is in [Integration](../integration/).

## Deployment

The whole stack runs with Docker Compose (PostgreSQL + API + Nginx). Migrations
apply automatically on container start, and the superadmin is bootstrapped on
first boot. Backend and frontend each have a separate, path-filtered GitHub
Actions pipeline.
