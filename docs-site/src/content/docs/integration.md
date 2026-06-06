---
title: Integration (Mişko ↔ CV service)
description: The contract between two fully independent systems.
---

This document defines how two **fully independent** systems talk to each other.
The goal is loose coupling: each side owns its database, release cycle and
deployment, and they communicate only over the boundary below.

## Two systems and responsibilities

| | **Mişko** | **CV service** |
|---|---|---|
| Role | System of record for the lab workflow | Capture + inference + telemetry |
| Stack | Node/Express/Prisma/**PostgreSQL** + Vue | Python/FastAPI/YOLOv8/ByteTrack/OpenCV |
| Holds | user, subject, environment, scenario, **test + summary** | raw frame telemetry, events, video |
| Volume | Small / relational | **Large** (its own PostgreSQL) |
| Owns | the test's "what / who / when" | the test's "measurement / evidence" |

## Data ownership

- **Mişko** → the test's identity and context, plus its **per-environment metric
  results** (`result` JSON).
- **CV service** → the raw output: per-frame position/pose telemetry, event
  rows, video files. None of this enters Mişko's database.
- **Object storage** (MinIO/S3) → video and large artifacts. Both sides keep
  only a **URL reference**, never the bytes.

## Mapping: Test ↔ capture session

A Mişko **Test** corresponds to a CV **capture session**, linked by two things:

- `cameraId` / `cageId` — which camera/cage (CV tags every event with it).
- The time window — the Test's `startedAt → endedAt` range.

The CV service can run continuously; a Test simply claims a **slice** of that
stream, so CV never has to wait on Mişko.

## Test lifecycle

A Test is created from a **Scenario + Subject** only. The scenario already carries
the environments (geometry/zones) and their paradigms (which fix the metrics) - so
neither the operator nor the CV picks those at run time.

```mermaid
sequenceDiagram
    participant Op as Operator (Vue)
    participant M as Mişko API
    participant CV as CV service
    participant S as Object storage

    Op->>M: POST /api/tests (scenario, subject, cameraId)
    M-->>Op: Test (PENDING)
    Op->>M: PATCH /api/tests/:id (status=RUNNING, startedAt)
    Note over CV: CV is already processing that camera;<br/>events are written to its DB by camera_id
    Op->>M: PATCH /api/tests/:id (status=DONE, endedAt)
    CV->>CV: compute the scenario's metrics for [startedAt, endedAt] + cameraId
    CV->>S: upload video + trajectory
    CV->>M: POST /api/tests/:id/result (service auth) + metrics + artifact URLs
    M->>M: validate + store per environment
    M-->>CV: 200 OK (idempotent)
```

## CV result flow (inside Mişko)

When the CV pushes a result, Mişko runs a fixed pipeline:

1. **Service auth** - validate `X-Service-Key` against `SERVICE_API_KEY` (no
   operator JWT); 401 otherwise.
2. **Idempotency** - if this `captureSessionId` was already recorded, return the
   stored result as a no-op. Safe to retry.
3. **Resolve the contract** - the push targets one **environment** of the test's
   scenario; load that environment's **paradigm** + metric dictionary.
4. **Validate** - every `metrics` key must be a known metric of that paradigm
   (unknown keys rejected); values match `valueType`/`validRange` (templated
   metrics are zone maps).
5. **Store** - metrics go to `Test.result.environments[envId].metrics` (JSON, with
   QC + artifact URLs alongside). Raw video/telemetry never enters Mişko.
6. **Finalize** - mark the environment DONE; the test is DONE when all are. There
   is **no pass/fail verdict** - the result is data, interpreted at the analysis
   layer. QC is stored alongside the metrics.

## Boundary API (contract)

Direction: **CV → Mişko push**. CV is the single source of truth for raw data;
it computes the metric and reports it. Mişko never queries CV.

```
POST /api/tests/:id/result
X-Service-Key: <SERVICE_API_KEY>
Content-Type: application/json
```

```json
{
  "captureSessionId": "cv-9f3a...",
  "cameraId": "cam-1",
  "startedAt": "2026-06-01T15:00:00Z",
  "endedAt":   "2026-06-01T15:05:00Z",
  "metrics": {
    "distance_cm": 1234.5,
    "time_in_zone_s": 45.2,
    "latency_to_platform_s": 12.0,
    "events": { "ate": 3, "rest": 7 }
  },
  "artifacts": {
    "videoUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.mp4",
    "trajectoryUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.parquet"
  }
}
```

Mişko validates `metrics` against the environment's paradigm dictionary, stores
them at `Test.result.environments[envId].metrics`, and marks the environment (and,
when all are done, the test) `DONE`. There is no pass/fail verdict. If
`captureSessionId` was already processed, the call is a no-op (idempotent).

## Service-to-service auth

Operator endpoints use JWT (existing). Service-to-service calls use a separate
**service key**: the `X-Service-Key` header, validated against `SERVICE_API_KEY`.
The CV service carries no operator JWT — it arrives with its own service identity.

## Resilience

- **Idempotency:** repeated pushes are safe via `captureSessionId`.
- **Retry:** if Mişko is unreachable, CV retries with exponential backoff and
  keeps the result marked "unsent" until it succeeds.
- **Independent survival:** if Mişko is down, CV keeps capturing and sends later;
  if CV is down, Mişko's domain management is unaffected.
