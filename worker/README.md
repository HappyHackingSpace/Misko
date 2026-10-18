# Misko analysis worker

A separate Python process that analyzes recorded, fixed-camera video of the
lab's four paradigms for the Go API, one subject at a time: the Morris water
maze (MWM), novel object recognition (NOVEL_OBJECT), the pole test (POLE) and
the forced treadmill (TREADMILL). It claims runs, measures the subject's position
in every frame, uploads a trajectory and an annotated video to Cloud Storage
through signed URLs, and submits them. It computes no metrics: the API reads
the stored trajectory and runs the Go metric engine, so the published metrics
and events always come from the uploaded samples.

**Status:** tested only on synthetic video. It has not been validated against a
manually annotated real recording or real Cloud Storage (issue #135); do not use
its results for research until that validation is recorded.

## What it does

A test (one session) has one video and one or more subjects, each in its own
arena (up to three arenas can share the camera frame). The API creates one run
per subject of the test (a test subject), so the same video may be claimed once
per subject; each run carries that subject's arena calibration, whose crop is
the arena's region of the frame.

1. `POST /api/worker/claim` with `Authorization: Worker TOKEN`. The job carries
   the run (clip, pinned parameters, `testSubjectId`), `testSubject` (`id` and
   the arena's `environmentRevisionId`), the paradigm contract and the pinned
   calibration of that arena (frame size, crop, homography and the calibrated
   zones, `calibration.zones`: a list of `{key, pixels, centimeters}`, each a
   `{circle: {x, y, radius}}` or a `{polygon: [{x, y}, ...]}`). The worker
   never learns the subject's group or treatment. A job for a paradigm version outside
   [Supported paradigms](#supported-paradigms) is failed at once with
   `UNSUPPORTED_PARADIGM` (not retryable), before any download or heartbeat. A heartbeat extends the lease every 60
   seconds; if the API says the attempt is stale, the worker stops without
   reporting.
2. Downloads the pinned source generation from the signed `source-url`.
3. Decodes the clip `[clipStartUs, clipEndUs)` with presentation timestamps, so
   variable frame rates and dropped frames keep their real times. Sample times are
   microseconds from the clip start.
4. Detects the subject by background subtraction inside the calibration crop,
   so subjects in the other arenas of the same frame are never seen:
   the background is a per-pixel percentile of up to 31 to 62 frames sampled over
   the clip, and only pixels darker than it (or lighter, for a light subject) are
   foreground. Anything that never moves (objects, a pole, a shock grid) is part
   of the background. The subject's polarity is detected from the frames. The largest
   blob (or, when several compete, the largest one near the previous position)
   gives an intensity-weighted centroid, in full-frame pixels with the frame's
   top-left corner at (0, 0), which is mapped to the calibration's centimeter
   frame with its homography. Confidence is the blob's contrast (mean difference
   / 80, at most 1) times its share of all foreground pixels. A frame without a
   blob, or with a position outside the apparatus (see the table below), is an
   untracked sample. The tuning per apparatus is described under
   [Supported paradigms](#supported-paradigms).
5. Writes `trajectory.json` (`misko.trajectory.v1`: `tUs`, `xCm`, `yCm`,
   `tracked`, `confidence`, `inferred`, plus diagnostics under `worker`,
   including the `testSubjectId`, the paradigm, the settings used, the polarity
   and the lost and inferred ratios) and `overlay.mp4` (H.264, the source frames
   with the paradigm's zones, the recent path and the position marker, a ring
   for an inferred position, or a red border when lost). MWM zones come from
   the parameters; the other paradigms draw their calibrated zones as the
   calibrator drew them in pixels. `inferred` is part of
   the trajectory format for positions placed without being seen; no supported
   paradigm infers, so it is always false and the inferred ratio is 0. The overlay is
   drawn in a second pass, once every sample is settled. It keeps each frame's recording time as its
   timestamp: the pair uses `identity-v1`, source offset = clip start, output
   offset = 0.
6. Declares both outputs with size and CRC32C, uploads them with resumable
   uploads, and submits the artifacts and the pair. The API verifies the objects,
   applies QC (`max_lost_frame_ratio`, `min_tracking_confidence`; inferred
   samples are tracked but left out of the confidence mean and reported as an
   inferred ratio) and publishes.
   The first successful run of a subject opens its result for human review;
   only results a reviewer accepts enter the study's statistics.

Failures: an unsupported paradigm (`UNSUPPORTED_PARADIGM: KEY vN`), a corrupt or unreadable video, a frame size that differs from the
calibration, a missing calibration or a clip without frames are reported as
non-retryable; storage and unexpected errors as retryable. Reasons never contain
URLs, and neither the token nor signed URLs are logged.

## Supported paradigms

`SUPPORTED` in `misko_worker/paradigms.py` lists MWM, NOVEL_OBJECT, POLE and
TREADMILL, each at version 1; any other paradigm version is failed with
`UNSUPPORTED_PARADIGM` before the video is downloaded. Samples are always in
the centimeter frame the calibration's world points define, so the calibration
decides the coordinates the engine sees. The worker reports positions only; the
Go engine derives every event (exploration bouts, the turn, base reached, falls,
shocks, exhaustion) from the samples and the zones.

| Paradigm | Camera | Frame (`coordinateFrame`) | Zones | Inside (else untracked) | Tracking |
|---|---|---|---|---|---|
| MWM | above the tank | tank-centred, x east, y north (`TANK_CM`) | from the parameters: tank, wall annulus, target quadrant, platform | `hypot(x, y) <= tank_diameter_cm / 2` | water |
| NOVEL_OBJECT | above the arena | origin at an arena corner, x to the right, y down the image (`CALIBRATION_CM`) | calibrated: `novel_object`, `familiar_object` (object plus about 2 cm) | convex hull of the zones and the arena box (`arena_width_cm` x `arena_height_cm`) plus 3 cm, within the crop | floor |
| POLE | at the side of the pole | x horizontal, y = height above the base, base at y = 0 (`CALIBRATION_CM`) | calibrated: `pole`, `base` | convex hull of the zones plus 3 cm, within the crop | pole |
| TREADMILL | at the side of the lane | x along the lane from the grid end (x = 0) to the front (x = `lane_length_cm`), y vertical (`CALIBRATION_CM`) | calibrated: `shock_grid` | convex hull of the zones, or x within 0 to `lane_length_cm`, plus 3 cm, within the crop | belt |

For the calibrated-zone paradigms detection looks at the inside region plus
6 cm; without any zone (and without the arena box or the lane length) the crop
is the only limit. Inside checks use the zones' centimeter shapes, the same the
engine uses; a zone sent in one unit only is converted with the transform.

- **Water (MWM).** Detection is limited to the tank plus 6 cm, so an
  experimenter's hand at the pool side is never tracked. The background is the
  per-pixel median, and a pixel is foreground only when it differs by more than
  18 grey levels plus four times its own robust temporal noise, so ripples and
  shimmering reflections need a larger difference while calm water keeps a low
  threshold; 50 grey levels count as full contrast. Among several blobs the one
  near the previous position wins, so a reflection does not pull the track
  across the tank. Tested with 45 to 50 grey levels of contrast, a dark subject
  in milky water and a light subject in dark water.
- **Floor (NOVEL_OBJECT).** A fixed threshold of 30 grey levels and the 90th
  percentile background toward the floor. The objects never move, so they are
  background and are never taken for the subject, whatever their color; a
  subject touching or partly covering an object is still seen where it differs
  from the object by 30 grey levels.
- **Pole (POLE).** The floor settings, and a blob up to 35% of the crop
  diagonal from the previous position is still preferred over a larger one, so
  the frames of a fall (about 30 px per frame at 4 px/cm and 25 fps) stay
  tracked. The pole is background; a subject in front of it is seen where it
  differs from the pole.
- **Belt (TREADMILL).** The moving belt texture changes every pixel it covers,
  so, as on water, a pixel is foreground only beyond 25 grey levels plus four
  times its own temporal noise; the belt never becomes foreground while the
  back wall keeps a low threshold. The background is the 75th percentile
  toward the floor, and the static shock grid is background.

## Calibration

The world points decide the frame of every sample, so give them in the frame
of the table above:

- **MWM.** Points relative to the tank centre, for example the wall at east
  (r, 0), north (0, r), west (-r, 0) and south (0, -r) with r = tank
  diameter / 2; enter the platform position in the same frame.
- **NOVEL_OBJECT.** The four arena corners, (0, 0), (width, 0),
  (width, height), (0, height), with y growing down the image. Draw
  `novel_object` and `familiar_object` around each object with the
  exploration distance (the object's footprint plus about 2 cm); swap them for
  a counterbalanced session.
- **POLE.** At least four non-collinear points of the rig in the pole's
  vertical plane, with y the height above the base: the foot (0, 0) and the top
  (0, `pole_length_cm`) of the pole and two marks on the back panel. Draw
  `pole` around the part where the subject climbs (wide enough for its body)
  and `base` over the floor area at the foot, high enough to hold a subject
  sitting there.
- **TREADMILL.** At least four non-collinear points in the lane's side plane,
  x from the grid end (x = 0) to the front (x = `lane_length_cm`), y vertical.
  Draw `shock_grid` over the grid and high enough to hold the body of a subject
  standing on it, since the centroid is what enters the zone.

## Known limits

- One subject per crop, fixed camera, stable lighting, contrast of at least
  about 40 grey levels between subject and its surroundings (30 on a dry
  floor).
- The subject color is detected from the frames by default ("auto"). Forcing the
  wrong polarity makes the worker look for a subject that is not there, and the
  samples are lost.
- **A subject that sits still for long becomes part of the background** and is
  reported as lost, which fails QC. The share of the clip a subject may rest at
  one spot is set by the background percentile: about half of the clip on
  water (median), about 70% on the treadmill (75th percentile), about 90% for
  NOVEL_OBJECT and POLE (90th percentile). The tests cover rests of 60% (beside
  the novel object) and 56% (on the shock grid).
- NOVEL_OBJECT: exploration is the body centre inside an object zone, not the
  nose direction; climbing or sitting on an object counts. A subject that
  moves an object leaves a ghost in the background at its old place.
- POLE: the subject is a blob; head and tail direction are not measured, so the
  turn is only seen as the centroid's net descent (the engine's
  `turn_descent_cm`). Motion blur in a fast fall spreads the blob and lowers
  the contrast, so the fall frames of a real recording may be tracked with a
  lower confidence or lost; the synthetic fall has no blur.
- TREADMILL: the belt's temporal noise is a robust standard deviation; a
  strongly two-tone belt (for example wide black and white stripes) defeats it
  and can produce foreground on the belt. The subject's legs over the belt
  have a high threshold, so the centroid sits slightly higher on the body.
- Bedding, shadows of the experimenter or a second subject are not handled;
  they can move the centroid. On water only reflections and ripples are
  handled.
- A video with several arenas is decoded once per subject (three passes: the
  background, detection and the overlay); one multi-arena pass is a later
  optimisation. The synthetic tests cover one arena per frame.

## Run

Register the worker as a lab manager with model version
`misko-tracker-bgsub 2.1.0` and four capabilities, each at version 1: `MWM`,
`NOVEL_OBJECT`, `POLE` and `TREADMILL`. A worker registered with the earlier
model version (`2.0.0`, MWM only) keeps claiming only MWM runs: register it
again (or update its model version and capabilities) after upgrading the
image, since the model version is recorded with every result. Then:

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
apparatus of each paradigm with its calibration and zones:

- MWM: a 100 cm tank with a tank-centred calibration, ripples, moving
  reflections and a hand at the pool side (dark and light subjects).
- NOVEL_OBJECT: a 40 cm box seen from above with a dark and a light object; the
  subject visits the familiar object, then rests beside the novel one for 60%
  of the clip.
- POLE: a 50 cm pole seen from the side; a descent with a turn, and a free
  fall from the top to the base (about 300 cm/s at the end).
- TREADMILL: a 45 cm lane seen from the side with a textured belt moving at
  20 cm/s and a static grid; the subject runs near the front, drifts back with
  two short grid contacts and stays on the grid for the last 56% of the clip.

`tests/test_zoned_paradigms.py` checks the position error (mean at most
0.3 cm, maximum at most 1 cm) and that what the engine derives from the samples
(time inside each object zone, the turn, the first sample on the base, every
stay on the grid and the exhaustion stay) matches the ground truth within two
frames. `tests/contract_fixture.py` writes the MWM swim the Go contract test
reads. CI runs the suite on ubuntu x86_64; to run it there from macOS:

```bash
cd worker
docker run --rm --platform linux/amd64 -v "$PWD:/w" -w /w python:3.14.3-slim \
  sh -c "pip install -q --only-binary=:all: -r requirements-test.txt && python -m pytest -q"
```

Dependencies are pinned in `requirements.txt`. PyAV's binary wheels bundle
FFmpeg with the GPL-licensed libx264 encoder; review the license obligations
before distributing a worker image.
