# Misko analysis worker

A separate Python process that analyzes recorded, fixed-camera, single-subject
Open Field video for the Go API. It claims runs, measures the subject's position
in every frame, uploads a trajectory and an annotated video to Cloud Storage
through signed URLs, and submits them. It computes no metrics: the API reads
the stored trajectory and runs the Go metric engine, so the published metrics
and events always come from the uploaded samples.

**Status:** tested only on synthetic video. It has not been validated against a
manually annotated real recording or real Cloud Storage (issue #135); do not use
its results for research until that validation is recorded.

## What it does

1. `POST /api/worker/claim` with `Authorization: Worker TOKEN`. The job carries
   the run (clip, pinned parameters), the paradigm contract and the pinned
   calibration. A heartbeat extends the lease every 60 seconds; if the API says
   the attempt is stale, the worker stops without reporting.
2. Downloads the pinned source generation from the signed `source-url`.
3. Decodes the clip `[clipStartUs, clipEndUs)` with presentation timestamps, so
   variable frame rates and dropped frames keep their real times. Sample times are
   microseconds from the clip start.
4. Detects the subject by background subtraction inside the calibration crop:
   the background is a per-pixel percentile of up to 31 to 62 frames sampled over
   the clip, and only pixels darker than it (or lighter, for a light subject) are
   foreground. The largest blob's intensity-weighted centroid, in full-frame
   pixels with the frame's top-left corner at (0, 0), is mapped to arena
   centimeters with the calibration homography. Confidence is the blob's contrast
   (mean difference / 80, at most 1) times its share of all foreground pixels.
   A frame without a blob, or with a position outside the arena, is an untracked
   sample.
5. Writes `trajectory.json` (`misko.trajectory.v1`: `tUs`, `xCm`, `yCm`,
   `tracked`, `confidence`, plus diagnostics under `worker`) and `overlay.mp4`
   (H.264, the source frames with the arena, the center zone, the recent path and
   the position marker, or a red border when lost). The overlay keeps each frame's
   recording time as its timestamp: the pair uses `identity-v1`, source offset =
   clip start, output offset = 0.
6. Declares both outputs with size and CRC32C, uploads them with resumable
   uploads, and submits the artifacts and the pair. The API verifies the objects,
   applies QC (`max_lost_frame_ratio`, `min_tracking_confidence`) and publishes.

Failures: a corrupt or unreadable video, a frame size that differs from the
calibration, a missing calibration or a clip without frames are reported as
non-retryable; storage and unexpected errors as retryable. Reasons never contain
URLs, and neither the token nor signed URLs are logged.

## Supported scope and known limits

- One subject, fixed camera, stable lighting, contrast of at least about 30 grey
  levels between subject and floor.
- The subject color is detected from the frames by default ("auto"). Forcing the
  wrong polarity makes the worker track the spots where the subject rested, with
  wrong positions that look tracked.
- A subject that stays at one spot for more than about 90% of the clip becomes
  part of the background and is reported as lost, which fails QC.
- Reflections, bedding, shadows of the experimenter or a second animal are not
  handled; they can move the centroid.
- The only capability is `OPEN_FIELD` version 1.

## Run

Register the worker as a lab manager with capability `OPEN_FIELD` version 1 and
model version `misko-open-field-bgsub 1.0.0`, then:

```bash
docker build -t misko-worker worker
docker run --rm -e MISKO_API_URL=https://API_HOST -e MISKO_WORKER_TOKEN=TOKEN misko-worker
```

| Variable | Default | Meaning |
|---|---|---|
| `MISKO_API_URL` | required | API base URL |
| `MISKO_WORKER_TOKEN` | required | Token shown when the worker was registered |
| `MISKO_WORK_DIR` | system temp | Scratch space for the source and outputs |
| `MISKO_POLL_INTERVAL` | `10` | Seconds between claims when nothing is queued |

## Test

```bash
cd worker
python3.14 -m venv .venv
.venv/bin/pip install --only-binary=:all: -r requirements-test.txt
.venv/bin/python -m pytest -q
```

The tests render synthetic recordings (a dark disc on a textured floor seen by
a tilted camera, 320 x 240) with known positions and cover position accuracy,
occlusion, low contrast, variable frame rate, dropped frames, clip offsets,
resting subjects, light subjects, corrupt files, overlay timestamps and seeking,
and the protocol against an in-process fake API and signed-URL storage.

Dependencies are pinned in `requirements.txt`. PyAV's binary wheels bundle
FFmpeg with the GPL-licensed libx264 encoder; review the license obligations
before distributing a worker image.
