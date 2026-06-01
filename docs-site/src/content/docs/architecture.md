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
