# Analysis for every paradigm

Status: done. Date: 2026-10-15. Goal: every published paradigm can be
analysed end to end and its results reviewed and reported.

The metric engine (`backend/internal/paradigms/domain`) already computes all
eleven paradigms from an `Input` of samples, calibrated zone shapes, scored
observed events and the recording duration. What is missing is getting those
inputs:

| Group | Paradigms | Input still missing |
|---|---|---|
| Tracked, zones from parameters | OPEN_FIELD (done), MWM, LIGHT_DARK | a worker that tracks them (water; a dark compartment) |
| Tracked, calibrated zones | EPM, Y_MAZE, BARNES_MAZE, THREE_CHAMBER | zone shapes drawn on the camera frame; worker support |
| Observer scored | ROTAROD, POLE, TREADMILL, NOVEL_OBJECT | scored events marked on the video by a person |

## Work packages (parallel, merged into `feat/fresh-schema`)

### ZB: calibrated zones, backend (`wp/zones`)
- Migration `014_calibration_zones.sql`: zone shapes belong to a calibration
  (environment default or per animal override), keyed by the paradigm's zone
  key; a key may have several shapes (EPM open arms are two polygons). Shapes
  are stored in pixels (as drawn) and in centimetres (converted with the
  calibration's fitted transform). Immutable like calibrations.
- Calibration input gains `zones: [{key, circle: {x, y, radius}} | {key, polygon: [{x, y}, ...]}]`
  in pixels. Validation: keys are calibrated zones of the paradigm version,
  every required calibrated zone is present, shapes are valid (radius > 0,
  polygon >= 3 points, inside the frame). Responses return each zone in
  pixels and centimetres. Calibration status is `WAITING_FOR_CALIBRATION`
  while a required zone is missing.
- Analysis: the claim payload's `calibration` gains `zones` (pixels and cm, for
  tracking limits and the overlay); result submission passes the run's
  calibrated zones to the engine (`Input.Zones`). `TrajectoryAnalyzable`
  accepts paradigms with calibrated zones (they come from the calibration);
  paradigms with input events stay worker-free.

### ZF: visual calibrator, panel (`fe/zones`)
- A calibrator component used by the environment calibration and the per
  animal override: shows a frame of a video (the test's current video, or for
  an environment a video file chosen on the computer and read locally, never
  uploaded), takes frame width and height from it, lets the user click fit and
  check points and type their world coordinates (with presets per paradigm:
  arena corners for boxes, tank centre-relative points for MWM), and draw the
  paradigm's calibrated zones (polygon by clicks, circle by centre and edge)
  with their translated labels, then shows the fit and check errors. Manual
  numeric entry stays available.

### SB: observer scoring, backend (`wp/scoring`)
- Migration `015_scoring.sql`: scored observations per test subject on the
  test's current video (type, kind, label, start and end in microseconds,
  scored by), a scoring sheet per test subject (scored event types, duration,
  status DRAFT or COMPLETED), and analysis runs gaining a `method`
  (`AUTOMATED`, `SCORED`) so a scored run succeeds without a worker or video
  pair and feeds `subject_results` exactly like an automated run.
- API under `/api/tests/{id}/subjects/{testSubjectId}/scoring`: read (with the
  paradigm's input event definitions), add, change and remove events while
  DRAFT, complete (scored types and duration; the engine computes the metrics;
  a SCORED run is created and the result opens PENDING for review), reopen as a
  new sheet version. Permission `test:run`.

### SF: scoring player, panel (`fe/scoring`)
- On the test page, per animal of an observer-scored paradigm: the video with
  frame stepping and playback speed, buttons and keyboard shortcuts for each
  input event (point at the current time, interval start/stop, labels such as
  novel/familiar), an editable event list on a timeline, and "complete
  scoring", after which the result appears in the review queue.

### W: worker for every tracked paradigm (`wp/worker`)
- `SUPPORTED` becomes every tracked paradigm version (OPEN_FIELD, EPM, Y_MAZE,
  BARNES_MAZE, THREE_CHAMBER, MWM, LIGHT_DARK v1); anything else still fails
  with `UNSUPPORTED_PARADIGM`.
- Samples come out in the calibration's centimetre frame for all of them (MWM:
  tank-centred, because its calibration's world points are tank-centred).
- Tracking per paradigm: subject polarity auto-detection; water background for
  MWM; LIGHT_DARK occlusion: while the animal is hidden in the dark
  compartment, positions are carried as inside the dark compartment when it was
  last seen there or at the doorway, and every such sample is marked inferred
  in the trajectory, so QC counts them honestly.
- Overlay draws the paradigm's zones (calibrated shapes or parameter derived).
- Synthetic fixtures and tests per paradigm; README capability list.

## After merging
Register the worker with every tracked capability, rebuild the local stack,
and run one real study per paradigm group end to end (tracked with zones,
MWM, light/dark, observer scored), then extend the Playwright suite.

## Verified (2026-10-15)

All five work packages are merged into `feat/fresh-schema`. An end-to-end study
on the local stack passed for EPM, Y_MAZE, MWM and LIGHT_DARK (tracked by the
worker registered with every tracked capability, model version
`misko-tracker-bgsub 2.0.0`) and for ROTAROD and NOVEL_OBJECT (observer
scored). BARNES_MAZE and
THREE_CHAMBER are covered by the worker's layout tests only, not by a study on
the local stack. The documentation (usage, measurements, installation, domain,
Go backend, English and Turkish) describes the calibrator, zones, scoring and
the worker capabilities.
