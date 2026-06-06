# Mişko ↔ CV Service — Integration Contract

> Status: design. Date: 2026-06-01.
> Related: the CV service's internal design is in `Workspace/fare-davranis/BACKEND-MIMARI.md`.

This document defines how two **fully independent** systems talk to each other.
The goal is loose coupling: both sides have their own database, release cycle and
deployment; they communicate only over the boundary described below.

## 1. The two systems and their responsibilities

| | **Mişko** (this repo) | **CV Service** (`fare-davranis`) |
|---|---|---|
| Role | **System of record** for the lab workflow | Image capture + inference + telemetry |
| Stack | Node/Express/Prisma/**PostgreSQL** + Vue | Python/FastAPI/YOLOv8/ByteTrack/OpenCV |
| Data held | user, subject, scenario, device, **test + summary result** | raw frame telemetry, events, video |
| Data volume | Small/relational | **Large** (its own PostgreSQL) |
| Ownership | The test's "what/who/when" | The test's "measurement/evidence" |

## 2. Data ownership (who stores what)

- **Mişko** → the test's identity and context: which scenario, which subject,
  which operator, which device, status, start/end, and the test's **summary
  metrics** (`result` JSON) + `passed`.
- **CV Service** → the test's raw output: frame-by-frame position/pose telemetry,
  event records (`timestamp · mouse_id · event · lap · duration_s · camera_id`),
  video files. **None of this enters Mişko's Postgres.**
- **Object storage** (MinIO/S3) → video and large artifacts. Mişko and CV only
  keep a **URL reference**, not the file itself.

## 3. Mapping: Test ↔ Capture Session

A Mişko **Test** corresponds to a **capture session** on the CV side. The link
is established by two fields:

- `cameraId` / `cageId` — which camera/cage (the CV tags every event with this).
- Time window — the Test's `startedAt` → `endedAt` range.

> The CV service may run continuously (24/7); a Test simply claims a **time
> slice** of that stream. This way the CV doesn't have to wait on Mişko.

## 4. Test lifecycle (with CV)

A Test is created from a **Scenario + Subject** only. The scenario already carries
the environment (geometry/zones), the paradigm, the metrics to collect and the
expected results - so neither the operator nor the CV chooses those at run time.

```mermaid
sequenceDiagram
    participant Op as Operator (Vue)
    participant M as Mişko API
    participant CV as CV Service
    participant S as Object Storage

    Op->>M: POST /api/tests (scenario, subject, device, cameraId)
    M-->>Op: Test (PENDING)
    Op->>M: PATCH /api/tests/:id (status=RUNNING, startedAt)
    Note over CV: CV is already processing that camera;<br/>events are written to its DB with camera_id
    Op->>M: PATCH /api/tests/:id (status=DONE, endedAt)
    M-->>CV: (optional) test-finished notification / CV polling
    CV->>CV: compute the scenario's metrics for [startedAt, endedAt] + cameraId
    CV->>S: upload video + trajectory
    CV->>M: POST /api/tests/:id/result (service auth) + metrics + artifact URLs
    M->>M: validate, store, evaluate vs scenario (see §4.1)
    M-->>CV: 200 OK (idempotent)
```

### 4.1 CV result flow (inside Mişko)

What happens when the CV pushes a result, step by step:

1. **Service auth** - the `X-Service-Key` header is validated against
   `SERVICE_API_KEY` (no operator JWT). Reject with 401 otherwise.
2. **Idempotency** - if this `captureSessionId` was already recorded, return the
   stored result as a no-op (200). Safe to retry.
3. **Resolve the contract** - load the Test, then its Scenario, then the
   scenario's Environment (and therefore the paradigm), the scenario's **selected
   metric keys**, its **expected results** and its `schemaVersion`. The scenario
   is the single source of truth for what is allowed.
4. **Validate the payload** against that contract:
   - every key in `metrics` must be a **known metric** of the paradigm
     (`isKnownMetricKey`) and **selected by the scenario**; unknown keys are
     rejected.
   - units/types match the metric dictionary; `schemaVersion` is compatible.
5. **Store** - `metrics → Test.result` (structured JSON, incl. `result.qc` and
   `result.artifacts` URLs). Raw video/telemetry never enters Mişko.
6. **Evaluate** - run the scenario's expected results through the acceptance
   engine (`backend/src/config/acceptance.js`) to set `Test.passed`
   (null when the scenario defines no expected results). The CV may *suggest* a
   `passed`, but Mişko's evaluation against the scenario is authoritative.
7. **Finalize** - set `status = DONE` (and `endedAt` if unset); return 200.

QC status is computed/stored separately from the behavioral `passed`.

## 5. Boundary API (contract)

Direction of communication: **CV → Mişko push** (CV is the single source of
truth for the raw data; it produces the metrics and reports them to Mişko). Mişko
does not query the CV.

### 5.1 Result submission (to be implemented)

```
POST /api/tests/:id/result
Authorization: none  →  X-Service-Key: <SERVICE_API_KEY>
Content-Type: application/json
```

Request body:

```json
{
  "captureSessionId": "cv-9f3a...",      // idempotency key
  "cameraId": "cam-1",
  "startedAt": "2026-06-01T15:00:00Z",
  "endedAt":   "2026-06-01T15:05:00Z",
  "metrics": {                            // keys must be metrics the scenario selected
    "distance_cm": 1234.5,
    "time_in_zone_s": 45.2,
    "latency_to_platform_s": 12.0,
    "events": { "ate": 3, "rest": 7 }
  },
  "passed": true,                         // advisory; Mişko re-evaluates vs the scenario
  "artifacts": {
    "videoUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.mp4",
    "trajectoryUrl": "s3://misko/cam-1/2026-06-01/sess-9f3a.parquet"
  }
}
```

What Mişko does: validates `metrics` against the scenario's selected metrics and
the paradigm dictionary, stores `metrics → Test.result` (JSON), evaluates them
against the scenario's expected results to set `Test.passed`, and sets status
`DONE` (see §4.1). If `captureSessionId` was already processed, it returns a no-op
(idempotent).

### 5.2 Authentication (service-to-service)

- Operator endpoints use JWT (existing).
- **Service-to-service** calls use a separate **service key**: an
  `X-Service-Key` header, validated against the env var `SERVICE_API_KEY`.
- The CV service carries no operator JWT; it comes with its own service identity.

## 6. Things that do not cross the boundary

- Raw frame telemetry and event rows → stay in **the CV's PostgreSQL**.
- Video/parquet → **object storage**; Mişko only sees the URL.
- No telemetry/timeseries table is **added** to Mişko's schema (including Timescale).

## 7. Resilience

- **Idempotency:** repeated submission is safe via `captureSessionId`.
- **Retry:** if Mişko is unreachable, the CV retries with exponential backoff;
  it keeps the result marked "not sent" on its side until it succeeds.
- **Independent survival:** if Mişko is down, the CV keeps capturing/writing and
  sends results later. If the CV is down, Mişko's domain management is unaffected.

## 8. Future (not in this version)

- Result delivery via a **message queue** (Redis/RabbitMQ) instead of push.
- **WebSocket/SSE** from the CV to the Vue panel for live monitoring (a separate
  channel that bypasses Mişko).
- A **TimescaleDB** upgrade on the CV side for per-frame telemetry.

---

**Summary:** Mişko is a lightweight/relational system-of-record; the CV service
is an independent vision service that keeps heavy data in its own Postgres. The
single contact point is the **summary + artifact URLs** the CV sends to Mişko at
test end.
