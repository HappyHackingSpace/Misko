# Automatic analysis for every paradigm

Status: in progress. Date: 2026-10-18.

The lab has no manual analysis: every paradigm is analysed automatically from
the video, and a person only reviews (accepts or rejects) the result. Today
only MWM is tracked; NOVEL_OBJECT, POLE and TREADMILL are observer scored.
This plan makes all three tracked and removes observer scoring.

## Decisions

- **No observer scoring anywhere.** The scoring context, its API, the scoring
  player, `analysis_runs.method` and the scoring tables are removed. Migration
  `016_drop_scoring.sql` drops what `015_scoring.sql` created (015 may already
  be applied in production, so it is not edited or deleted), restores the
  automated-only run constraints, and the code no longer knows SCORED runs.
- **Same paradigm keys and version 1, same metric keys.** Production has no
  data yet, so the four paradigms stay at version 1. Their definitions change
  (input events become derived events, new zones and parameters) and the golden
  manifests are regenerated.
- **Calibrated zones** carry the apparatus geometry the video needs (objects,
  pole, base, shock grid). The environment calibration draws them once; a per
  subject override on the test video changes them when needed (for example the
  novel object on the other side).
- **The worker tracks the subject's centroid** in the calibration's centimetre
  frame for every paradigm. The metric engine in Go derives every event from
  samples and zones; the worker never decides events.
- Every rule threshold is an ANALYSIS parameter with a standard default, so a
  lab can tune it per environment.

## Paradigm contracts

Coordinates are the calibration's world frame in centimetres. Zone keys are
fixed; all zones below are calibrated (`Calibrated: true`) and required.

### NOVEL_OBJECT (camera above the arena)
- World frame: arena corners, x to the right, y down the image, origin at a
  corner (the existing box preset).
- Zones: `novel_object`, `familiar_object`: one circle (or polygon) each,
  drawn around the object to include the exploration distance (object
  footprint plus about 2 cm). The calibrator offers a "swap novel and
  familiar" action for counterbalanced sessions.
- Parameters: `arena_width_cm`, `arena_height_cm` (APPARATUS, kept);
  `min_bout_s` (ANALYSIS, default 0.3 s: shorter visits are not exploration);
  `min_total_exploration_s` (ANALYSIS, kept); `max_sample_gap_s` (ANALYSIS).
  `object_zone_radius_cm` is removed (the drawn zone replaces it).
- Derived events: `exploration` INTERVAL with label `novel` or `familiar`
  while the centroid is inside that object's zone for at least `min_bout_s`.
- Metrics (keys unchanged): `novel_object_time_s`, `familiar_object_time_s`,
  `total_exploration_time_s`, `discrimination_index`.
- Known limit: exploration is approximated by the body centre inside the
  zone, not by nose direction; sitting on the object counts as exploration.

### POLE (camera at the side of the pole)
- World frame: x horizontal, y = height above the base in cm (base at y = 0,
  top at y = `pole_length_cm`). Calibration points: at least four
  non-collinear points of the rig (for example the base and the top of the pole
  and two marks on the back panel).
- Zones: `pole` (polygon around the pole where the subject climbs) and `base`
  (polygon of the floor area at the foot of the pole).
- Parameters: `pole_length_cm` (APPARATUS), `max_trial_duration_s` (SESSION),
  `turn_descent_cm` (ANALYSIS, default 2 cm: net downward movement from the
  highest point that marks the completed turn), `fall_speed_cm_s` (ANALYSIS,
  default 80 cm/s: downward speed above which leaving the pole is a fall),
  `max_sample_gap_s` (ANALYSIS).
- Derived events: `turn_complete` POINT (first time the centroid is
  `turn_descent_cm` below its highest point so far, while on the pole);
  `base_reached` POINT (first sample inside `base` after the start);
  `fall` POINT (the centroid leaves `pole` outside `base` with a downward speed
  above `fall_speed_cm_s`, or appears in `base` within one sample gap of being
  high on the pole).
- Metrics (keys unchanged): `t_turn_s`, `t_total_s`, `descent_time_s`,
  `descent_speed_cm_s`, `fall_detected`.

### TREADMILL (camera at the side of the lane)
- World frame: x along the lane from the shock grid end (x = 0) to the front
  (x = `lane_length_cm`), y vertical.
- Zones: `shock_grid` (polygon of the grid at the back of the lane).
- Parameters: `lane_length_cm` (APPARATUS), speed protocol and
  `max_trial_duration_s` (SESSION, kept), `exhaustion_dwell_s` (ANALYSIS,
  default 5 s: continuous time on the grid that defines exhaustion),
  `min_shock_dwell_s` (ANALYSIS, default 0.2 s: shorter grid contacts are not
  counted), `max_sample_gap_s` (ANALYSIS).
- Derived events: `shock` POINT at each entry into `shock_grid` lasting at
  least `min_shock_dwell_s`; `exhaustion` POINT at the start of the first stay
  on the grid lasting `exhaustion_dwell_s`.
- Metrics (keys unchanged): `latency_to_exhaustion_s`, `run_time_s`,
  `run_distance_cm`, `shock_count`. Without exhaustion the run lasts the whole
  clip, censored at `max_trial_duration_s`.

## Work packages

- **AB backend (`wp/auto-backend`)**: the three paradigm definitions above in
  `backend/internal/paradigms/domain` (derived events from samples and zones,
  hand-calculated tests per rule, regenerated goldens, EN and TR texts with
  "denek"/"subject"); analysis catalog contracts (every paradigm trajectory
  analysable, required zones); remove the scoring context, routes, OpenAPI,
  bootstrap wiring, `method`/`scoredBy`; migration 016; integration tests per
  paradigm through claim and submit with zones; README.
- **AW worker (`wp/auto-worker`)**: track the three paradigms (SUPPORTED adds
  NOVEL_OBJECT, POLE, TREADMILL v1): calibrated-zone parsing back in (the
  `calibration.zones` payload), inside rule = union hull of the zones plus
  margin or the crop; per apparatus tracking (objects that stay still in the
  arena, a vertical pole seen from the side, a moving belt seen from the side:
  noise-aware threshold like water); overlay draws zones; synthetic scenes and
  pytest per paradigm; README.
- **AF panel (`wp/auto-panel`)**: remove the scoring player, its API module,
  locale files and every SCORED branch; calibrator presets and guidance for the
  three paradigms (NOR box corners plus two object zones with a swap action;
  pole side view with y = height; treadmill lane with x from the grid); TR and
  EN; Playwright specs that used scoring move to automated results.
- After merging: one study per paradigm on the local stack with synthetic
  videos analysed by the real worker, Playwright in CI, docs EN and TR.
