---
title: Integration (analysis workers)
description: How an analysis worker claims a job, reads the video, uploads its outputs and has the API compute the result.
---

Video analysis runs in **workers**: processes that authenticate against the
Mişko API, take one job at a time and upload what they produced. A worker has
no database of its own and is never called by the API; it only calls the
`/api/worker` routes below. The bundled worker lives in the `worker/` folder
(see [Installation](../installation/#analysis-worker)), but any program that
follows this protocol can take its place.

## What a job is

A job analyses one video with one paradigm version, its parameters and, when
the paradigm needs one, a copy of a calibration. Its inputs never change:
analysing again creates a new job and the earlier ones stay. A job is requested
by an animal of a test or by a paradigm trial; the job never names the
animal's group or treatment, so a worker cannot be biased by them.

The `jobs` process queues a job for every animal of a test once the study is
running or completed, the animal is not excluded, the test's video is verified
and the animal's arena is calibrated with every zone it needs. Paradigm trials
queue one when their video and calibration are ready, and again whenever
someone analyses the trial with changed thresholds.

## Registering a worker

A lab manager (or superadmin) registers a worker with
`POST /api/analysis/workers`, giving its name, model version and the paradigm
versions it can analyse (its capabilities). Versions outside the paradigm
catalog are refused with `worker.unknownCapability`. The answer contains the
worker's token **once**; store it in the worker's secret configuration
(`MISKO_WORKER_TOKEN` for the bundled worker).
`POST /api/analysis/workers/{id}/disable` revokes the token.

Every worker call sends `Authorization: Worker <token>`. The token is accepted
only on `/api/worker` routes, and user tokens are not accepted there.

## The protocol

1. **Claim.** `POST /api/worker/claim` leases the oldest queued job whose
   paradigm version the worker supports, or answers `204` when there is none.
   The answer carries the job (video, clip, pinned parameters), the paradigm
   contract, the calibration snapshot (frame size, the arena's crop, the
   homography to centimeters and the calibrated zones), the output prefix and
   the trajectory format to produce. It also carries the **attempt** number:
   every later call names it, and a call with an old attempt is refused with
   `409`.
2. **Heartbeat.** `POST /api/worker/runs/{runId}/heartbeat` extends the lease.
   A lease lasts 5 minutes; the bundled worker renews it every 60 seconds.
3. **Read the video.** `POST /api/worker/runs/{runId}/source-url` returns a
   short-lived signed URL to the exact video version the job pinned.
4. **Upload the outputs.** For each output,
   `POST /api/worker/runs/{runId}/outputs` declares its kind, file name,
   content type, size and CRC32C, and returns a signed upload URL under the
   attempt's prefix (`jobs/{jobId}/attempts/{attempt}/`). A result needs
   exactly one `TRAJECTORY` (`application/json`) and one `ANALYZED_VIDEO`
   (`video/mp4` or `video/webm`); a `THUMBNAIL` image is optional.
5. **Submit.** `POST /api/worker/runs/{runId}/result` names the uploaded
   outputs, the model version and how the analysed video lines up with the
   original (`pair`: the offsets and the time mapping version). The request
   has no metrics and no events; the API refuses them.
6. **Or fail.** `POST /api/worker/runs/{runId}/failure` reports a reason and
   whether it is retryable.

The trajectory is `misko.trajectory.v1` JSON: the job id, the attempt, the
recording duration and equal-length arrays of sample times (`tUs`,
microseconds from the clip start), positions in centimeters (`xCm`, `yCm`),
`tracked`, `confidence` (0 to 1) and `inferred`. Other members, such as the
worker's own diagnostics, are kept but not read.

## What the API does with a result

- **Verifies the objects.** Each declared output must exist with the declared
  size, checksum and content type, under this job and attempt.
- **Checks the trajectory.** It must belong to this job and attempt, match the
  recording duration, have strictly increasing times and positions for every
  tracked sample. It may be at most 64 MiB (about four hours at 30 fps).
- **Applies quality rules.** The paradigm's `max_lost_frame_ratio` is checked
  against untracked samples and `min_tracking_confidence` against the mean
  confidence of tracked samples. Inferred samples count as tracked but are
  left out of the confidence mean. A trajectory that fails is `FAILED` with a
  `QC_FAILED` reason and is not retried; the quality figures are kept either
  way.
- **Computes the result.** The Go metric engine computes every metric and event
  from the trajectory with the job's pinned parameters and calibration. One
  definition of each measurement lives in the API, whatever produced the
  tracking. See [Measurement architecture](../measurements/).
- **Publishes atomically.** Metrics, events, quality and outputs appear
  together. Delivering the same accepted result again returns the job without
  publishing anything new.

A test animal's first successful result waits for human review; only results a
reviewer accepts count in the study report. Paradigm trial results never reach
review, reports or the dashboard.

## Retries and leases

A job may be attempted 3 times. A retryable failure, or a lease that expires
without a heartbeat, returns the job to the queue while attempts remain;
after the last one the job is `FAILED` (`LEASE_EXPIRED` for an expired lease).
A non-retryable failure ends the job at once. Because every call names its
attempt, a worker that lost its lease cannot overwrite the work of the worker
that took the job over.

## Calibration proposals

Workers also answer calibration proposals. The panel uploads one reference
frame (JPEG or PNG, at most 700 KiB) with `POST /api/calibration-proposals`;
when no job is queued, a worker leases a proposal for a paradigm version it
supports with `POST /api/worker/proposals/claim` (the frame comes in the
answer) and returns the detection with
`POST /api/worker/proposals/{id}/result`, naming its attempt. A lease lasts 2
minutes; an expired one is claimed again, at most 3 claims in total. A proposal is only
a suggestion: the researcher confirms every point and saves through the
calibration routes. Proposals are deleted after a day.

## Storage

Videos and outputs live in object storage, never in the database: a local
folder served by the API (`LOCAL_STORAGE_DIR`) or a private Google Cloud
Storage bucket (`GCS_BUCKET`). Workers only see short-lived signed URLs, and
the database keeps references to the objects.
