---
title: Measurement architecture
description: Paradigm specs, metric definitions, MWM normalization, and quality control.
---

Mişko needs stable measurement contracts, not loose result JSON. Every paradigm
must define its parameters, zones, metrics, event types, and quality
requirements before the CV service can produce results that are comparable
across studies and laboratories.

## Architecture principle

Every result passes through four layers:

| Layer | Purpose |
|---|---|
| `ParadigmSpec` | Code-owned contract for parameters, zones, metrics, event types (with CV detect specs), and result schema. |
| `Apparatus` | Lab-owned physical rig definition: geometry, material, surface, and zones in cm. |
| `Calibration` | Pixel-to-cm mapping for a fixed rig or a single test session. |
| `MetricDefinition` | Code-owned definition of each metric key, unit, formula, normalization, and QC dependency. |

CV tracks the animal. Mişko defines the scientific contract.

## Paradigms are templates

A paradigm is a versioned template defined in code. Environments are created
from it, and protocol steps refer to those environments. Every parameter has a
scope that says where it is set:

| Scope | Set on | Required | Examples |
|---|---|---|---|
| `APPARATUS` | Environment | Yes, measured on the physical setup | arena width, tank diameter, pole length |
| `ANALYSIS` | Environment | No, defaults suit most setups | center zone size, tracking gap tolerance, immobility threshold |
| `SESSION` | Protocol step | No, defaults apply | trial cut-off, rotarod start and end speed |

Each parameter carries a label and a help text that says what to measure or what
the value changes. Names, labels, help texts, zone and event names, plain metric
descriptions and the technical definitions (metric definitions, missing-data
rules, zone geometry, event definitions) are written in every supported
language (English and Turkish) next to the definition, and the API returns them
in the language the panel asks for (`Accept-Language`, or `?lang=tr`).
Formulas stay as written, since they are expressions rather than prose. The
published manifests are pinned in English, so a translation can never change
the contract unnoticed. The panel shows ratios as percentages and switches as
on and off.

```ts
interface ParadigmManifest {
  key: string;
  name: string;           // in the response language
  description: string;    // what the test is and what it measures
  category: "LEARNING_MEMORY" | "ANXIETY" | "MOTOR" | "SOCIAL";
  trialTypes: { key: string; label: string }[];
  parameters: {
    key: string;
    scope: "APPARATUS" | "ANALYSIS" | "SESSION";
    label: string;
    help: string;
    unit: "cm" | "cm_s" | "s" | "count" | "ratio" | "rpm" | "boolean";
    valueType: "number" | "integer";
    min: number;
    max: number;
    default: number;
  }[];
  zones: { key: string; label: string; role: string; geometry: string }[];
  metrics: { key: string; label: string; description: string; unit: string;
             definition: string; formula: string; missingWhen: string }[];
  qc: { key: string; operator: string; value: number; unit: string }[];
}
```

Changing a calculation, a range or a default publishes a new paradigm version.
Wording and help texts can be improved without a new version.

## Tracked and observer-scored paradigms

Each of the eleven paradigms gets its inputs in one of two ways. The metric
engine in `backend/internal/paradigms/domain` computes the metrics in both
cases, so a measurement has one definition whatever produced its inputs.

| Paradigm | Method | Input | Zones |
|---|---|---|---|
| `OPEN_FIELD` | Tracked (`AUTOMATED` run) | trajectory | from parameters (arena, center) |
| `LIGHT_DARK` | Tracked | trajectory, with inferred samples | from parameters (light, dark) |
| `MWM` | Tracked | trajectory in tank-centred cm | from parameters (platform, target quadrant, wall zone) |
| `EPM` | Tracked | trajectory | calibrated, required: `center`, `open_arms`, `closed_arms` |
| `Y_MAZE` | Tracked | trajectory | calibrated, required: `arm_a`, `arm_b`, `arm_c` |
| `BARNES_MAZE` | Tracked | trajectory | calibrated, required: `target_hole`, `holes` |
| `THREE_CHAMBER` | Tracked | trajectory | calibrated, required: `social_chamber`, `object_chamber`, `center_chamber`; optional: `social_interaction`, `object_interaction` |
| `ROTAROD` | Observer scored (`SCORED` run) | `fall` (point, once) | none |
| `POLE` | Observer scored | `turn_complete`, `base_reached`, `fall` (points, each once) | none |
| `TREADMILL` | Observer scored | `exhaustion` (point, once), `shock` (points) | none |
| `NOVEL_OBJECT` | Observer scored | `exploration` (intervals labelled `novel` or `familiar`) | none |

- **Tracked paradigms** are analysed by a worker. The jobs process queues one
  `AUTOMATED` run per animal once the test's video is verified, the animal's
  calibration is valid and an enabled worker has the paradigm version as a
  capability. The worker uploads a trajectory in the calibration's centimetre
  frame; the API never accepts metrics from it.
- **Observer-scored paradigms** never get worker runs, and a worker cannot
  register them as capabilities. A person marks the input events on the test's
  current video; completing the scoring sheet runs the engine and stores a
  `SCORED` run with the scorer and no worker, calibration or video pair. Event
  times are microseconds from the clip start. An event type that was ticked as
  scored but never occurred counts as not observed; an event type that was not
  scored makes its metrics missing, not zero.

Both kinds of run feed the same per-animal result, which opens `PENDING` for
human review; only accepted results enter the report.

### Zone requirements

Zones come from one of two places:

- **From parameters.** Open field, light/dark and the water maze derive their
  zones from the environment's measurements (for example `center_fraction`,
  `light_fraction`, the platform position). Their calibration world points
  must use the frame those zones are defined in: arena-local from one floor
  corner for open field, box-local from a light-side corner with x toward the
  dark end wall for light/dark, and **tank-centred** for the water maze
  ((0, 0) at the tank centre, x east, y north, so the wall points are (r, 0),
  (0, r), (-r, 0) and (0, -r) with r = tank diameter / 2).
- **Calibrated.** EPM, Y maze, Barnes maze and three-chamber zones are drawn as
  circles or polygons on the camera frame with the calibration. A key may have
  several shapes (two open arms, one shape per Barnes hole). Shapes are stored
  as drawn in pixels and converted to centimetres with the calibration's fitted
  transform; they must lie inside the frame and never change after the
  calibration is saved (migration `014_calibration_zones.sql`). Until a valid
  calibration with every required zone applies, the animal stays
  `WAITING_FOR_CALIBRATION` and is not analysed. The worker limits tracking to
  the convex hull of the zones (plus a margin), so junctions that belong to no
  zone, such as the Y maze centre, are still tracked.

### Inferred samples

In the light/dark box the animal disappears under the dark compartment's lid. A
stretch of frames without any detection is filled when the animal was last seen
in the dark compartment or at its doorway and is next seen there again (or the
clip ends). Those samples are `tracked` with `inferred: true` in
`misko.trajectory.v1`, so the engine counts dark time and transitions. A
disappearance anywhere else stays lost.

## No pass/fail verdict

A lab test result is **data, not a verdict**. Mişko does not compute a behavioral
`passed` flag and there is no acceptance-criteria layer. Whether a value is "good"
is interpretation, which belongs to the analysis layer, not to the recording
system. Only QC (tracking reliability) has a pass/fail meaning, and that is about
data quality, not behavior.

## Event types and CV detection

A run collects **events**; metrics are derived from them. The same event stream
is produced by manual entry today and by the CV service later, so the event layer
is the contract between Mişko and computer vision.

Each event type declares a `detect` spec describing how the CV service derives it
from the trajectory and zones:

```ts
interface EventTypeDef {
  type: string;                       // e.g. "zone_enter", "immobile", "fall"
  label: string;
  payload?: Record<string, "number" | "text" | "zone">;
  detect: DetectSpec;
}

type DetectSpec =
  | { kind: "zone_transition"; edge: "enter" | "exit"; zone?: string }
  | { kind: "zone_first_enter"; zone: string }
  | { kind: "speed_below"; threshold_cm_s: number; min_duration_s?: number }
  | { kind: "custom" };               // requires a dedicated detector/model
```

The generic kinds (`zone_transition`, `zone_first_enter`, `speed_below`) are
**data-driven**: a paradigm that only uses them needs no new CV code, declaring
the event types is enough. `custom` flags an event that needs a dedicated
detector (e.g. a rotarod fall, a social interaction). Event types are exposed on
the paradigm detail API (`GET /api/paradigms/:key` -> `eventTypes`).

## Localization

Catalog wording lives in the paradigm definitions in
`backend/internal/paradigms/domain`, written in English and Turkish side by side
(`T("Arena width", "Arena genişliği")`). `GET /api/paradigms`,
`GET /api/paradigms/{key}` and `GET /api/paradigms/{key}/versions/{version}`
answer in the language of `?lang=` (`en` or `tr`), otherwise of
`Accept-Language`, otherwise English, and set `Content-Language`. The panel
sends its chosen language with every request. A test fails when any name,
label, help text or metric description is missing in a language, looks like a
key, or uses an em dash. Small fixed enums (units, categories, scopes, quality
rule names) are translated by the panel's own dictionary.

## Measurement dictionary

The metric dictionary prevents two services from using one name for different
calculations.

```ts
interface MetricDefinition {
  key: string;
  paradigmKeys: string[];
  unit: "cm" | "cm_s" | "s" | "count" | "ratio" | "percent" | "deg" | "rpm" | "boolean";
  valueType: "number" | "integer" | "boolean" | "object";
  required: boolean;
  definition: string;
  formula: string;
  inputs: string[];
  normalization: NormalizationDef | null;
  aggregation: "per_trial" | "per_session" | "per_subject_timepoint" | "study_summary";
  qcDependencies: string[];
}
```

Canonical result units: positions and distances in `cm`, speed in `cm_s`,
duration in `s`, weight in `g`, angles in `deg`, rotation speed in `rpm`, ratios
as `ratio` (`0..1`) or `percent`, event flags as `boolean`, and counts as
`count`. Apparatus parameters reuse this enum and add `mm` (small rig diameters)
and `c` (water temperature in Celsius), which never appear in result metrics.
Pixel values stay out of Mişko result metrics.

## Baseline metrics

| Key | Unit | Definition |
|---|---|---|
| `duration_s` | `s` | Analyzed time window after invalid frames are trimmed. |
| `distance_cm` | `cm` | Total path length in apparatus coordinates. |
| `mean_speed_cm_s` | `cm_s` | Distance divided by duration. |
| `zone_time_s.{zoneKey}` | `s` | Time spent in a declared zone. |
| `zone_entries.{zoneKey}` | `count` | Debounced entries into a declared zone. |
| `latency_to_zone_s.{zoneKey}` | `s` | Time from trial start to first valid zone entry. |
| `path_efficiency_ratio` | `ratio` | Straight-line distance to target divided by actual path length. |

## Paradigm metrics

| Paradigm | Core metrics |
|---|---|
| MWM | Escape latency, path length, swim speed, target quadrant time, platform crossings, thigmotaxis, mean distance to platform. |
| Open Field | Total distance, center time, periphery time, center entries, immobility, mean speed. |
| EPM | Open arm time, closed arm time, open and closed arm entries, latency to open arm, optional risk assessment events. |
| Rotarod | Latency to fall, rpm at fall, trial duration, fall detected, learning slope across repeated trials. |

## MWM normalization

Morris Water Maze varies strongly across labs. A comparable result needs both raw
cm values and normalized values.

Required apparatus fields:

```json
{
  "tank_diameter_cm": 120,
  "tank_center_cm": { "x": 0, "y": 0 },
  "platform_diameter_cm": 10,
  "platform_center_cm": { "x": 30, "y": -30 },
  "platform_quadrant": "SE",
  "water_opacity": "opaque",
  "water_temp_c": 22,
  "surface_color": "white",
  "start_positions": ["N", "E", "S", "W"],
  "wall_annulus_width_cm": 12
}
```

Coordinate normalization:

```txt
radius_cm = tank_diameter_cm / 2
x_norm = x_cm / radius_cm
y_norm = y_cm / radius_cm
distance_norm = distance_cm / tank_diameter_cm
platform_distance_norm = distance_to_platform_cm / tank_diameter_cm
```

Cross-lab reports should compare only compatible trial types and protocol
versions, prefer normalized spatial metrics, keep raw cm values for within-lab
reproducibility, and always report swim speed beside latency.

## Quality control

QC status is about data reliability only. There is no behavioral pass/fail: the
behavioral result is the metric data itself. `result.qc.status` is the
reliability status.

| QC key | Unit | Default action |
|---|---|---|
| `tracking_confidence_mean` | `ratio` | Warn below `0.80`. |
| `tracking_confidence_p05` | `ratio` | Review below `0.50`. |
| `dropped_frame_ratio` | `ratio` | Warn above `0.05`, fail above `0.15`. |
| `calibration_error_cm_mean` | `cm` | Warn above `1.0 cm`. |
| `calibration_error_cm_max` | `cm` | Review above `2.0 cm`. |
| `occlusion_time_ratio` | `ratio` | Warn above `0.10`. |
| `out_of_bounds_time_ratio` | `ratio` | Fail above `0.02`. |
| `lighting_warning` | `boolean` | Manual review. |
| `contrast_warning` | `boolean` | Manual review. |

QC statuses: `PASS`, `WARN`, `REVIEW_REQUIRED`, `FAIL`.

### Rules applied today

The table above is the target model. The rules the API applies to a tracked run
today are those of the paradigm version (`trajectoryQC`):

| Rule | Limit | Measured on |
|---|---|---|
| `min_tracking_confidence` | `>= 0.7` | mean confidence of observed samples (tracked and not inferred) |
| `max_lost_frame_ratio` | `<= 0.1` | share of samples that are not tracked; inferred samples count as tracked |
| `max_calibration_error_cm` | `<= 2` cm | the calibration's check points, when it is saved |

Inferred samples never raise or lower the confidence mean. The worker records
their share in the trajectory's diagnostics (`worker.inferredRatio`), and a
paradigm version may cap it with `max_inferred_ratio`; no published version sets
that rule yet. A run that breaks a rule fails with a `QC_FAILED` reason and publishes
nothing. Scored runs have no trajectory, so these tracking rules do not apply
to them; the reviewer checks the scored events instead.

## Result shape

`Test.result` should become structured JSON in the domain migration.

```json
{
  "schemaVersion": "misko.result.v1",
  "paradigmKey": "MWM",
  "protocolVersion": "mwm.v1",
  "trialType": "acquisition_hidden_platform",
  "metrics": {
    "escape_latency_s": 18.4,
    "path_length_cm": 735.2,
    "path_length_norm": 6.13,
    "mean_swim_speed_cm_s": 39.9,
    "thigmotaxis_time_ratio": 0.22
  },
  "qc": {
    "status": "PASS",
    "tracking_confidence_mean": 0.94,
    "dropped_frame_ratio": 0.01,
    "calibration_error_cm_mean": 0.42
  },
  "artifacts": {
    "videoUrl": "s3://misko/tests/t-1/video.mp4",
    "trajectoryUrl": "s3://misko/tests/t-1/trajectory.parquet",
    "heatmapUrl": "s3://misko/tests/t-1/heatmap.png"
  }
}
```
