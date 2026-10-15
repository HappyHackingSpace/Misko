# Misko analysis worker

A separate Python process that analyzes recorded, fixed-camera video of the
Morris water maze (MWM), the one worker-tracked paradigm, for the Go API, one
animal at a time. The lab's other paradigms (NOVEL_OBJECT, POLE, TREADMILL)
are scored by an observer and never reach a worker. It claims runs, measures the subject's position
in every frame, uploads a trajectory and an annotated video to Cloud Storage
through signed URLs, and submits them. It computes no metrics: the API reads
the stored trajectory and runs the Go metric engine, so the published metrics
and events always come from the uploaded samples.

**Status:** tested only on synthetic video. It has not been validated against a
manually annotated real recording or real Cloud Storage (issue #135); do not use
its results for research until that validation is recorded.

## What it does

A test (one session) has one video and one or more animals, each in its own
arena (up to three arenas can share the camera frame). The API creates one run
per animal of the test (a test subject), so the same video may be claimed once
per animal; each run carries that animal's arena calibration, whose crop is
the arena's region of the frame.

1. `POST /api/worker/claim` with `Authorization: Worker TOKEN`. The job carries
   the run (clip, pinned parameters, `testSubjectId`), `testSubject` (`id` and
   the arena's `environmentRevisionId`), the paradigm contract and the pinned
   calibration of that arena (frame size, crop and homography). The worker never learns the animal's group
   or treatment. A job for a paradigm version outside
   [Supported paradigms](#supported-paradigms) is failed at once with
   `UNSUPPORTED_PARADIGM` (not retryable), before any download or heartbeat. A heartbeat extends the lease every 60
   seconds; if the API says the attempt is stale, the worker stops without
   reporting.
2. Downloads the pinned source generation from the signed `source-url`.
3. Decodes the clip `[clipStartUs, clipEndUs)` with presentation timestamps, so
   variable frame rates and dropped frames keep their real times. Sample times are
   microseconds from the clip start.
4. Detects the subject by background subtraction inside the calibration crop,
   so animals in the other arenas of the same frame are never seen:
   the background is a per-pixel percentile of up to 31 to 62 frames sampled over
   the clip, and only pixels darker than it (or lighter, for a light subject) are
   foreground. The subject's polarity is detected from the frames. The largest
   blob (or, when several compete, the largest one near the previous position)
   gives an intensity-weighted centroid, in full-frame pixels with the frame's
   top-left corner at (0, 0), which is mapped to the calibration's centimeter
   frame with its homography. Confidence is the blob's contrast (mean difference
   / 80, at most 1) times its share of all foreground pixels. A frame without a
   blob, or with a position outside the apparatus (see the table below), is an
   untracked sample. The water tuning is described under
   [Supported paradigms](#supported-paradigms).
5. Writes `trajectory.json` (`misko.trajectory.v1`: `tUs`, `xCm`, `yCm`,
   `tracked`, `confidence`, `inferred`, plus diagnostics under `worker`,
   including the `testSubjectId`, the paradigm, the settings used, the polarity
   and the lost and inferred ratios) and `overlay.mp4` (H.264, the source frames
   with the paradigm's zones, the recent path and the position marker, a ring
   for an inferred position, or a red border when lost). `inferred` is part of
   the trajectory format for positions placed without being seen; MWM never
   infers, so it is always false and the inferred ratio is 0. The overlay is
   drawn in a second pass, once every sample is settled. It keeps each frame's recording time as its
   timestamp: the pair uses `identity-v1`, source offset = clip start, output
   offset = 0.
6. Declares both outputs with size and CRC32C, uploads them with resumable
   uploads, and submits the artifacts and the pair. The API verifies the objects,
   applies QC (`max_lost_frame_ratio`, `min_tracking_confidence`; inferred
   samples are tracked but left out of the confidence mean and reported as an
   inferred ratio) and publishes.
   The first successful run of an animal opens its result for human review;
   only results a reviewer accepts enter the study's statistics.

Failures: an unsupported paradigm (`UNSUPPORTED_PARADIGM: KEY vN`), a corrupt or unreadable video, a frame size that differs from the
calibration, a missing calibration or a clip without frames are reported as
non-retryable; storage and unexpected errors as retryable. Reasons never contain
URLs, and neither the token nor signed URLs are logged.

## Supported paradigms

`SUPPORTED` in `misko_worker/paradigms.py` lists MWM version 1 only. Samples
are always in the centimeter frame the calibration's world points define, so
the calibration decides the coordinates the engine sees:

| Paradigm | Calibration world points | Inside (else untracked) | Overlay zones | Tracking |
|---|---|---|---|---|
| MWM | tank-centred: (0, 0) is the tank centre, x east, y north, for example (r, 0), (0, r), (-r, 0), (0, -r) on the wall | `hypot(x, y) <= tank_diameter_cm / 2` | tank, wall annulus, target quadrant, platform | water |

- **Water (MWM).** Detection is limited to the tank plus 6 cm, so an
  experimenter's hand at the pool side is never tracked. The background is the
  per-pixel median, and a pixel is foreground only when it differs by more than
  18 grey levels plus four times its own robust temporal noise, so ripples and
  shimmering reflections need a larger difference while calm water keeps a low
  threshold; 50 grey levels count as full contrast. Among several blobs the one
  near the previous position wins, so a reflection does not pull the track
  across the tank. Tested with 45 to 50 grey levels of contrast, a dark animal
  in milky water and a light animal in dark water.

## Calibration

Give world points relative to the tank centre, for example the wall at east
(r, 0), north (0, r), west (-r, 0) and south (0, -r) with r = tank diameter / 2,
and enter the platform position in the same frame.

## Known limits

- One subject per tank crop, fixed camera, stable lighting, contrast of at least
  about 40 grey levels between subject and water.
- The subject color is detected from the frames by default ("auto"). Forcing the
  wrong polarity makes the worker look for a subject that is not there, and the
  samples are lost.
- The background on water is the per-pixel median, so a subject floating at one
  spot for more than about half of the clip becomes part of the background and
  is reported as lost, which fails QC.
- Bedding, shadows of the experimenter or a second animal are not handled; they
  can move the centroid. On water only reflections and ripples are handled.
- A video with several arenas is decoded once per animal (three passes: the
  background, detection and the overlay); one multi-arena pass is a later
  optimisation. The synthetic tests cover one arena per frame.

## Run

Register the worker as a lab manager with model version
`misko-tracker-bgsub 2.0.0` and the one capability `MWM` at version 1. Then:

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

The tests render synthetic recordings (a disc in a calm 50 cm tank seen by a
tilted camera, 320 x 240) with known positions and cover position accuracy,
occlusion, low contrast, variable frame rate, dropped frames, clip offsets,
resting subjects, light subjects, corrupt files, overlay timestamps and seeking,
unsupported paradigms (refused without downloading), and the protocol against
an in-process fake API and signed-URL storage. `tests/scenes.py` adds the full
MWM apparatus: a 100 cm tank with a tank-centred calibration, ripples, moving
reflections and a hand at the pool side (dark and light animals), and the
overlay's tank zones. `tests/contract_fixture.py` writes the MWM swim the Go
contract test reads.

Dependencies are pinned in `requirements.txt`. PyAV's binary wheels bundle
FFmpeg with the GPL-licensed libx264 encoder; review the license obligations
before distributing a worker image.
