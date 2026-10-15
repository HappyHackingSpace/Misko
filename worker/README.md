# Misko analysis worker

A separate Python process that analyzes recorded, fixed-camera video of every
tracked paradigm for the Go API, one animal at a time. It claims runs, measures the subject's position
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
   calibration of that arena (frame size, crop, homography and, for paradigms
   with calibrated zones, `zones`). The worker never learns the animal's group
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
   untracked sample. Per paradigm tuning is described under
   [Supported paradigms](#supported-paradigms).
5. Writes `trajectory.json` (`misko.trajectory.v1`: `tUs`, `xCm`, `yCm`,
   `tracked`, `confidence`, `inferred`, plus diagnostics under `worker`,
   including the `testSubjectId`, the paradigm, the settings used, the polarity
   and the lost and inferred ratios) and `overlay.mp4` (H.264, the source frames
   with the paradigm's zones, the recent path and the position marker, a ring
   for an inferred position, or a red border when lost). The overlay is drawn in
   a second pass, after inference. It keeps each frame's recording time as its
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

`SUPPORTED` in `misko_worker/paradigms.py` lists version 1 of OPEN_FIELD, EPM,
Y_MAZE, BARNES_MAZE, THREE_CHAMBER, MWM and LIGHT_DARK. Samples are always in
the centimeter frame the calibration's world points define, so the calibration
decides the coordinates the engine sees:

| Paradigm | Calibration world points | Inside (else untracked) | Overlay zones | Tracking |
|---|---|---|---|---|
| OPEN_FIELD | arena-local, origin at one arena corner, x along `arena_width_cm`, y along `arena_height_cm` | `0 <= x <= width`, `0 <= y <= height` | arena outline and center zone (`center_fraction`) | default |
| LIGHT_DARK | box-local, origin at the light-side corner, x from the light end wall to the dark end wall | `0 <= x <= box_width_cm`, `0 <= y <= box_height_cm` | light and dark compartments (`light_fraction`) | default, plus occlusion inference |
| MWM | tank-centred: (0, 0) is the tank centre, x east, y north, for example (r, 0), (0, r), (-r, 0), (0, -r) on the wall | `hypot(x, y) <= tank_diameter_cm / 2` | tank, wall annulus, target quadrant, platform | water |
| EPM, Y_MAZE, BARNES_MAZE, THREE_CHAMBER | any frame (for example centred on the maze); the zones are drawn on the frame and carried by the calibration | within 3 cm of the convex hull of every calibrated zone; without zones, anywhere in the crop | the calibrated shapes as drawn | default, detection limited to the hull plus 6 cm |

- **Calibrated zones.** The claim's `calibration.zones` is read as
  `{key: [{"pixels": shape, "cm": shape}, ...]}`, where a shape is
  `{"circle": {"x", "y", "radius"}}` or `{"polygon": [{"x", "y"}, ...]}`. A list
  of `{key, ...}` entries, `{"pixels": {key: [...]}, "cm": {key: [...]}}` and
  bare shapes (pixels as drawn) are accepted too; a shape given in one unit is
  converted with the calibration transform. The convex hull keeps the junctions
  between zones inside: the Y maze center (in no arm zone) and the Barnes
  platform between its holes are tracked, not lost.
- **Water (MWM).** Detection is limited to the tank plus 6 cm, so an
  experimenter's hand at the pool side is never tracked. The background is the
  per-pixel median, and a pixel is foreground only when it differs by more than
  18 grey levels plus four times its own robust temporal noise, so ripples and
  shimmering reflections need a larger difference while calm water keeps a low
  threshold; 50 grey levels count as full contrast. Among several blobs the one
  near the previous position wins, so a reflection does not pull the track
  across the tank. Tested with 45 to 50 grey levels of contrast, a dark animal
  in milky water and a light animal in dark water.
- **Occlusion (LIGHT_DARK).** A camera above the box sees the light
  compartment; the animal disappears under the dark compartment's lid. A
  stretch of frames without any detection is filled when the animal was last
  seen in the dark compartment or at its doorway (within 5 cm of the partition
  on the light side) and is next seen there again, or the clip ends: its
  position is carried where it was last seen in the dark compartment, or 2 cm
  inside the doorway. Those samples are `tracked` with `inferred: true` and
  confidence 0.5, so the engine counts dark time and transitions while QC
  reports the inferred share honestly. A disappearance anywhere else stays lost.

## Calibration per paradigm

- OPEN_FIELD and LIGHT_DARK: click the four inside corners of the floor and
  give them as (0, 0), (width, 0), (width, height), (0, height); for LIGHT_DARK
  (0, 0) is a light-side corner and x runs toward the dark end wall.
- MWM: give world points relative to the tank centre, for example the wall at
  east (r, 0), north (0, r), west (-r, 0) and south (0, -r) with r = tank
  diameter / 2, and enter the platform position in the same frame.
- Calibrated-zone paradigms: fit the calibration in any frame (maze-centred is
  convenient) and draw every required zone; the crop should cover the
  apparatus. Without zones the worker still tracks inside the crop, but the
  engine reports zone metrics missing.

## Known limits

- One subject per arena crop, fixed camera, stable lighting, contrast of at least
  about 30 grey levels between subject and floor (about 40 on water).
- The subject color is detected from the frames by default ("auto"). Forcing the
  wrong polarity makes the worker track the spots where the subject rested, with
  wrong positions that look tracked.
- A subject that stays at one spot for more than about 90% of the clip becomes
  part of the background and is reported as lost, which fails QC. On water the
  background is the median, so floating at one spot for more than half of the
  clip does the same.
- Bedding, shadows of the experimenter or a second animal are not handled; they
  can move the centroid. On water only reflections and ripples are handled.
- At the light/dark doorway only the lit part of the animal is seen, so its
  centroid sits up to a body radius toward the light side until it vanishes.
  EPM closed-arm walls that hide the animal from a tilted camera are not
  modelled: frames where it is hidden are lost.
- A video with several arenas is decoded once per animal (three passes: the
  background, detection and the overlay); one multi-arena pass is a later
  optimisation. The synthetic tests cover one arena per frame.

## Run

Register the worker as a lab manager with model version
`misko-tracker-bgsub 2.0.0` and every tracked capability, each at version 1:
`OPEN_FIELD`, `EPM`, `Y_MAZE`, `BARNES_MAZE`, `THREE_CHAMBER`, `MWM` and
`LIGHT_DARK` (the API accepts capabilities with calibrated zones once their
shapes come with the calibration). Then:

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

The tests render synthetic recordings (a disc on a textured floor seen by a
tilted camera, 320 x 240) with known positions and cover position accuracy,
occlusion, low contrast, variable frame rate, dropped frames, clip offsets,
resting subjects, light subjects, corrupt files, overlay timestamps and seeking,
unsupported paradigms (refused without downloading), and the protocol against
an in-process fake API and signed-URL storage. `tests/scenes.py` adds one
apparatus per paradigm: a plus maze with calibrated arms (EPM), a Y maze whose
center is in no zone, a round tank with a tank-centred calibration, ripples,
moving reflections and a hand at the pool side (MWM, dark and light animals),
and a light/dark box whose dark half is under a lid (LIGHT_DARK); Barnes and
three-chamber layouts are tested on their zone hulls.

Dependencies are pinned in `requirements.txt`. PyAV's binary wheels bundle
FFmpeg with the GPL-licensed libx264 encoder; review the license obligations
before distributing a worker image.
