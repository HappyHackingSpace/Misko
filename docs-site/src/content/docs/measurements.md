---
title: Measurement architecture
description: Paradigm specs, metric definitions, MWM normalization, and quality control.
---

Mişko needs stable measurement contracts, not loose result JSON. Every paradigm
must define its parameters, zones, metrics, acceptance rules, and quality
requirements before the CV service can produce results that are comparable
across studies and laboratories.

## Architecture principle

Every result passes through four layers:

| Layer | Purpose |
|---|---|
| `ParadigmSpec` | Code-owned contract for parameters, zones, metrics, acceptance rules, and result schema. |
| `Apparatus` | Lab-owned physical rig definition: geometry, material, surface, and zones in cm. |
| `Calibration` | Pixel-to-cm mapping for a fixed rig or a single test session. |
| `MetricDefinition` | Code-owned definition of each metric key, unit, formula, normalization, and QC dependency. |

CV tracks the animal. Mişko defines the scientific contract.

## Paradigm spec pages

Each paradigm needs a detail page with:

- Identity: key, name, category, trial types.
- Apparatus parameters: physical values with units, validation, defaults, and min/max values.
- Session parameters: trial duration, start position, trial index, protocol variant.
- Zones: zone keys, geometry type, coordinate system, and derivation rule.
- Metrics: required and optional metric keys, units, definitions, and normalization.
- Suggested acceptance criteria: optional, code-owned templates a user can adopt. Real acceptance criteria are optional and defined per test (see "Acceptance criteria" below).
- QC requirements: tracking confidence, dropped frames, calibration error, occlusion, lighting, and contrast.
- Artifacts: video, trajectory, heatmap, calibration image, and debug overlay URLs.

```ts
interface ParadigmSpec {
  key: "MWM" | "OPEN_FIELD" | "EPM" | "ROTAROD";
  name: string;
  category: "learning_memory" | "anxiety" | "motor" | "social";
  trialTypes: TrialTypeDef[];
  apparatusParameters: FieldDef[];
  sessionParameters: FieldDef[];
  zones(config: ApparatusConfig): ZoneDef[];
  metrics: MetricDefinition[];
  suggestedAcceptance: AcceptanceRule[];
  qc: QualityRequirement[];
}
```

## Acceptance criteria

Acceptance criteria decide a test's behavioral pass/fail line. They are:

- **Optional**: a test with no criteria stays unevaluated (`Test.passed = null`).
- **Per test**: each test stores its own criteria as JSON in `Test.acceptanceCriteria`.
- **User-defined**: the user picks any metric valid for the paradigm, an operator,
  and a threshold. Paradigm specs only ship `suggestedAcceptance` templates that the
  user can adopt and edit.

```ts
interface AcceptanceCriterion {
  metricKey: string;                 // a known metric key for the paradigm
  operator: "<" | "<=" | ">" | ">=" | "==" | "!=" | "between";
  value: number | [number, number]; // [min, max] for "between"
}
```

When a result is submitted, the engine evaluates every criterion against the
metrics and sets `Test.passed`: `true` if all pass, `false` if any fails or its
metric is missing, and `null` when there are no criteria. An explicit `passed`
value in the request overrides automatic evaluation (manual review).

## Localization

The paradigm and metric vocabulary is the scientific contract and stays
Turkish-only in the spec source (the default language). The read-only inspection
API localizes free-text labels and definitions through an optional `?lang=`
query parameter on `GET /api/paradigms`, `GET /api/paradigms/:key` and
`GET /api/paradigms/metrics`. Supported values are `tr` (default) and `en`; an
unknown or missing value falls back to `tr`, and any untranslated label or
definition falls back to its Turkish source. The backend stays the single source
of truth (catalog in `backend/src/config/i18n.js`), so the CV service and the
frontend share the same vocabulary with no drift. Small fixed enums (zone
type/role, species) are translated by the frontend UI dictionary instead.

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

Canonical units: positions and distances in `cm`, speed in `cm_s`, duration in
`s`, weight in `g`, ratios as `0..1`, and counts as `count`. Pixel values stay
out of Mişko result metrics.

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

QC status is separate from behavioral pass/fail. `Test.passed` is the behavioral
acceptance result. `result.qc.status` is the reliability status.

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
