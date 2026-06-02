# Misko - Measurement Architecture

> Status: design. Date: 2026-06-02.
> Scope: paradigm specifications, metric definitions, cross-lab normalization,
> and data quality rules for behavioral tests.

This document defines how Misko turns a video-backed behavioral test into a
reproducible scientific result. The goal is to avoid a common failure mode:
recording "some JSON metrics" without a stable definition of what each value
means, which units it uses, whether it can be compared across labs, and whether
the underlying tracking quality is good enough.

## 1. Architecture principle

Every result is produced through four explicit layers:

| Layer | Owner | Purpose |
|---|---|---|
| `ParadigmSpec` | Misko code | Declares parameters, zones, metric definitions, acceptance rules, and result schema. |
| `Apparatus` | Lab data in Misko | Stores the physical rig: geometry, surface, material, zones in cm, and paradigm-specific values. |
| `Calibration` | Misko definition, CV execution | Maps camera pixels to the apparatus coordinate system. |
| `MetricDefinition` | Misko code | Defines each measurement key, unit, calculation, normalization, QC dependencies, and aggregation behavior. |

CV tracks the animal and computes metrics from the trajectory. Misko defines the
scientific contract: which metrics exist, how zones are interpreted, which units
are canonical, and which quality thresholds decide whether a result is reliable.

## 2. Paradigm spec detail pages

Each paradigm must have a detail page generated from a code-backed
`ParadigmSpec`. The page is not marketing text. It is an operational contract
for users, developers, and the CV service.

### 2.1 Required sections per paradigm

| Section | Required content |
|---|---|
| Identity | `key`, display name, category, supported species, supported trial types. |
| Apparatus parameters | Physical values the lab must enter, with units, min/max, defaults, and validation rules. |
| Session parameters | Per-test values such as trial duration, start position, timepoint, trial index, and protocol variant. |
| Zones | Zone keys, geometry type, source of truth, coordinate system, and derivation rule. |
| Metrics | Required and optional metric keys, units, definitions, and normalization rules. |
| Acceptance criteria | Default pass/fail rules and which values can be overridden by the lab or study. |
| QC requirements | Minimum tracking confidence, allowed dropped frames, calibration error, occlusion limits, and lighting checks. |
| Artifacts | Expected video, trajectory, heatmap, calibration image, and debug overlay URLs. |

### 2.2 `ParadigmSpec` shape

```ts
type Unit =
  | "cm"
  | "cm_s"
  | "s"
  | "count"
  | "ratio"
  | "percent"
  | "deg"
  | "rpm"
  | "boolean";

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
  validateApparatus(config: ApparatusConfig): ValidationError[];
  validateSession(config: SessionConfig): ValidationError[];
}
```

### 2.3 Zone model

Zones are always defined in apparatus coordinates, not pixels. The origin and
axes are declared by the apparatus. For circular arenas, Misko should use a
center-origin coordinate system where the tank center is `(0, 0)` and values are
stored in cm.

```ts
interface ZoneDef {
  key: string;
  label: string;
  type: "circle" | "polygon" | "annulus" | "quadrant" | "line";
  role: "target" | "control" | "risk" | "open" | "closed" | "center" | "periphery";
  geometryCm: unknown;
  derivedFrom?: string;
  required: boolean;
}
```

### 2.4 Acceptance criteria

Acceptance criteria are not hardcoded as a single `passed` formula. They are:

- **Optional**: a test with no criteria stays unevaluated (`Test.passed = null`).
- **Per test**: each test stores its own criteria as JSON in `Test.acceptanceCriteria`.
- **User-defined**: the user picks any metric valid for the paradigm, an operator,
  and a threshold.

The paradigm spec only carries `suggestedAcceptance`: optional, code-owned
templates the user can adopt and edit. The user-facing criterion is concrete:

```ts
interface AcceptanceCriterion {
  metricKey: string;                  // a known metric key for the paradigm
  operator: "<" | "<=" | ">" | ">=" | "==" | "!=" | "between";
  value: number | [number, number];  // [min, max] for "between"
}
```

The suggested template shape (display/seed only) keeps richer metadata:

```ts
interface AcceptanceRule {
  key: string;
  metricKey: string;
  operator: "<" | "<=" | ">" | ">=" | "==" | "between";
  value: number | [number, number] | string; // string = symbolic, e.g. "max_trial_duration_s"
  appliesToTrialTypes?: string[];
  overridable: boolean;
}
```

Evaluation: when a result is submitted, every criterion is compared against the
metrics. `Test.passed` becomes `true` if all pass, `false` if any fails or its
metric is missing, and `null` when there are no criteria. An explicit `passed`
in the request overrides automatic evaluation (manual review).

Suggested templates:

| Paradigm | Suggested rule |
|---|---|
| MWM | `escape_latency_s <= max_trial_duration_s` for acquisition trials. |
| Barnes Maze | `primary_latency_s <= max_trial_duration_s` for acquisition trials. |
| Rotarod | `latency_to_fall_s >= study.minimum_latency_s`. |
| Open Field | Usually no pass/fail, only QC pass/fail. |
| EPM | Usually no pass/fail, only QC pass/fail. |

The engine lives in `backend/src/config/acceptance.js`
(`validateAcceptanceCriteria`, `evaluateAcceptance`). Operators are exposed at
`GET /api/paradigms/acceptance-operators`.

### 2.5 Localization

The paradigm and metric vocabulary is the scientific contract and stays
Turkish-only in the spec source (the default language). The read-only inspection
API localizes the free-text labels and definitions on the way out through an
optional `?lang=` query parameter:

- `GET /api/paradigms?lang=en`
- `GET /api/paradigms/:key?lang=en`
- `GET /api/paradigms/metrics?lang=en` (with optional `&paradigm=MWM`)

Supported values are `tr` (default) and `en`. An unknown or missing value falls
back to `tr`, and any label or definition without a translation falls back to its
Turkish source string. This keeps the backend the single source of truth so the
CV service and the frontend share the same localized vocabulary with no drift.
The catalog and helpers live in `backend/src/config/i18n.js`. Small fixed enums
(zone type/role, species) are translated by the frontend UI dictionary instead.

## 3. Measurement dictionary

The measurement dictionary is a code-backed registry of metric definitions. It
prevents two services from using the same name for different calculations.

### 3.1 Required fields

```ts
interface MetricDefinition {
  key: string;
  label: string;
  paradigmKeys: string[];
  unit: Unit;
  valueType: "number" | "integer" | "boolean" | "object";
  required: boolean;
  definition: string;
  formula: string;
  inputs: string[];
  normalization: NormalizationDef | null;
  validRange?: [number, number];
  aggregation: "per_trial" | "per_session" | "per_subject_timepoint" | "study_summary";
  qcDependencies: string[];
}
```

### 3.2 Unit policy

Canonical storage units:

| Quantity | Stored unit | Notes |
|---|---|---|
| Position | `cm` | Pixel values never enter Misko result metrics. |
| Distance | `cm` | Store raw distance and, when useful, normalized distance. |
| Speed | `cm_s` | Computed from cm trajectory and timestamps. |
| Duration | `s` | Milliseconds may be kept in CV telemetry, not in Misko summaries. |
| Weight | `g` | Subject physiology, not CV result. |
| Angles | `deg` | If pose or heading is supported later. |
| Ratios | `ratio` | Store `0..1`, render as percent in UI if needed. |
| Counts | `count` | Entries, crossings, falls, events. |

### 3.3 Baseline metric keys

| Key | Unit | Definition |
|---|---|---|
| `duration_s` | `s` | Analyzed time window after trimming invalid frames. |
| `distance_cm` | `cm` | Total path length in apparatus coordinates. |
| `mean_speed_cm_s` | `cm_s` | `distance_cm / duration_s`. |
| `max_speed_cm_s` | `cm_s` | Maximum smoothed instantaneous speed. |
| `immobility_s` | `s` | Time below the paradigm-specific movement threshold. |
| `zone_time_s.{zoneKey}` | `s` | Time spent inside a declared zone. |
| `zone_entries.{zoneKey}` | `count` | Number of entries into a declared zone after debounce. |
| `latency_to_zone_s.{zoneKey}` | `s` | Time from trial start to first valid entry into a declared zone. |
| `path_efficiency_ratio` | `ratio` | Straight-line distance to target divided by actual path length, if a target exists. |

### 3.4 Paradigm metric sets

#### MWM

| Key | Unit | Notes |
|---|---|---|
| `escape_latency_s` | `s` | Time to first sustained platform-zone entry. |
| `path_length_cm` | `cm` | Total swim path until platform or trial end. |
| `mean_swim_speed_cm_s` | `cm_s` | Used to separate learning effects from motor impairment. |
| `platform_latency_s` | `s` | Alias-safe explicit target-zone latency. |
| `platform_crossings_count` | `count` | Probe trials only, crossings through previous platform zone. |
| `target_quadrant_time_ratio` | `ratio` | Time in target quadrant divided by valid analyzed duration. |
| `quadrant_time_s.{quadrant}` | `s` | NE, NW, SE, SW or lab-defined labels. |
| `thigmotaxis_time_ratio` | `ratio` | Time in annulus near tank wall. |
| `mean_distance_to_platform_cm` | `cm` | Average proximity to target, robust for probe trials. |
| `heading_error_deg` | `deg` | Optional, requires heading or smoothed path vector. |

#### Open Field

| Key | Unit | Notes |
|---|---|---|
| `distance_cm` | `cm` | Total locomotion. |
| `center_time_ratio` | `ratio` | Time in center zone divided by valid duration. |
| `periphery_time_ratio` | `ratio` | Time in periphery divided by valid duration. |
| `center_entries_count` | `count` | Debounced center entries. |
| `immobility_s` | `s` | Movement below threshold. |
| `mean_speed_cm_s` | `cm_s` | Locomotor control metric. |

#### EPM

| Key | Unit | Notes |
|---|---|---|
| `open_arm_time_ratio` | `ratio` | Time in open arms divided by valid duration. |
| `closed_arm_time_ratio` | `ratio` | Time in closed arms divided by valid duration. |
| `open_arm_entries_count` | `count` | Debounced open arm entries. |
| `closed_arm_entries_count` | `count` | Debounced closed arm entries. |
| `latency_to_open_arm_s` | `s` | First entry into an open arm. |
| `risk_assessment_count` | `count` | Optional event if behavior classifier exists. |

#### Rotarod

| Key | Unit | Notes |
|---|---|---|
| `latency_to_fall_s` | `s` | Time from trial start to fall event. |
| `rpm_at_fall` | `rpm` | Derived from mode and elapsed time. |
| `trial_duration_s` | `s` | May equal max duration if the subject does not fall. |
| `fall_detected` | `boolean` | Whether a fall event was detected. |
| `learning_slope` | `ratio` | Study aggregation across repeated trials, not a single-trial CV metric. |

## 4. MWM normalization strategy

Morris Water Maze needs special handling because every lab may have a different
tank diameter, platform diameter, platform location, water contrast, camera
height, and start-position protocol. The system should compare behavior, not
hardware.

### 4.1 Required MWM apparatus fields

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

### 4.2 Coordinate normalization

All MWM trajectories should be projected into cm and then normalized by tank
radius:

```txt
radius_cm = tank_diameter_cm / 2
x_norm = x_cm / radius_cm
y_norm = y_cm / radius_cm
distance_norm = distance_cm / tank_diameter_cm
platform_distance_norm = distance_to_platform_cm / tank_diameter_cm
```

This makes coordinates comparable across a 100 cm tank and a 150 cm tank while
preserving raw cm values for direct lab reports.

### 4.3 Zone derivation

| Zone | Derivation |
|---|---|
| `tank` | Circle with radius `tank_diameter_cm / 2`. |
| `platform` | Circle at `platform_center_cm`, radius `platform_diameter_cm / 2`. |
| `platform_proximity` | Circle around the platform, default radius `platform_diameter_cm * 2`. |
| `target_quadrant` | Quadrant containing `platform_center_cm`. |
| `opposite_quadrant` | Quadrant opposite target. |
| `wall_annulus` | Annulus from `radius_cm - wall_annulus_width_cm` to `radius_cm`. |
| `center_zone` | Circle centered at tank origin, default radius `radius_cm * 0.5`. |

### 4.4 Trial-type handling

| Trial type | Primary metrics |
|---|---|
| `acquisition_hidden_platform` | Escape latency, path length, swim speed, path efficiency. |
| `visible_platform` | Vision/motor control: escape latency, swim speed, gross path quality. |
| `probe_no_platform` | Target quadrant ratio, platform crossings, mean distance to platform. |
| `reversal` | Same as acquisition, but target platform changes and old target metrics may be retained. |

### 4.5 Cross-lab comparison rules

1. Compare only tests with the same trial type and compatible protocol version.
2. Use normalized spatial metrics for cross-lab reports.
3. Keep raw cm metrics for within-lab reproducibility.
4. Report swim speed beside latency, because motor impairment can mimic poor learning.
5. Flag incompatible apparatuses when platform diameter ratio differs beyond a configured threshold.
6. Store `protocolVersion` on the study or test so metric changes do not silently mix old and new definitions.

## 5. Quality control metrics

QC metrics are not optional decoration. They decide whether a result can be
trusted, whether it needs manual review, or whether it should be excluded from
analysis.

### 5.1 QC result shape

```json
{
  "qc": {
    "status": "PASS",
    "tracking_confidence_mean": 0.94,
    "tracking_confidence_p05": 0.81,
    "dropped_frame_ratio": 0.01,
    "calibration_error_cm_mean": 0.42,
    "calibration_error_cm_max": 0.91,
    "occlusion_time_ratio": 0.02,
    "out_of_bounds_time_ratio": 0.0,
    "lighting_warning": false,
    "contrast_warning": false,
    "manual_review_required": false
  }
}
```

### 5.2 QC metric definitions

| Key | Unit | Meaning | Default action |
|---|---|---|---|
| `tracking_confidence_mean` | `ratio` | Mean tracker confidence over valid frames. | Warn below `0.80`. |
| `tracking_confidence_p05` | `ratio` | Fifth percentile confidence, catches unstable tracking. | Review below `0.50`. |
| `dropped_frame_ratio` | `ratio` | Missing or unusable frames divided by expected frames. | Warn above `0.05`, fail above `0.15`. |
| `calibration_error_cm_mean` | `cm` | Mean reprojection error from calibration reference points. | Warn above `1.0 cm`. |
| `calibration_error_cm_max` | `cm` | Worst reference-point error. | Review above `2.0 cm`. |
| `occlusion_time_ratio` | `ratio` | Time the animal is partially or fully hidden. | Warn above `0.10`. |
| `out_of_bounds_time_ratio` | `ratio` | Time projected outside the apparatus boundary. | Fail above `0.02`. |
| `lighting_warning` | `boolean` | Frame brightness or glare outside configured range. | Manual review. |
| `contrast_warning` | `boolean` | Animal and background contrast too low. | Manual review. |

### 5.3 QC status

| Status | Meaning |
|---|---|
| `PASS` | Metrics are usable without manual review. |
| `WARN` | Metrics are stored, but UI marks the run as lower confidence. |
| `REVIEW_REQUIRED` | Metrics are stored but excluded from default study summaries until reviewed. |
| `FAIL` | Metrics should not be used for scientific analysis. |

`Test.passed` should remain the behavioral pass/fail or acceptance result. QC
status is separate and should live under `result.qc.status`.

## 6. Result contract

Misko should store a structured JSON result, not a stringified arbitrary object.
The exact database type should be Prisma `Json` once the domain model migration
lands.

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
  "zones": {
    "platform": { "time_s": 1.8, "entries_count": 1 },
    "target_quadrant": { "time_s": 13.2 }
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
    "heatmapUrl": "s3://misko/tests/t-1/heatmap.png",
    "debugOverlayUrl": "s3://misko/tests/t-1/debug.mp4"
  }
}
```

## 7. Implementation order

1. Add `MetricDefinition` and `ParadigmSpec` interfaces in code.
2. Implement the MWM spec first, because it has the hardest normalization case.
3. Add Open Field, EPM, and Rotarod specs with explicit metric dictionaries.
4. Change `Test.result` from stringified JSON to structured JSON in the domain migration.
5. Add result validation on `POST /api/tests/:id/result`.
6. Add QC display and filtering in the UI.
7. Generate docs-site paradigm pages from the registry or keep them manually synced until generation is built.

## 8. Open decisions

1. Whether paradigm detail pages are generated from code or maintained manually.
2. Whether QC thresholds are fixed per paradigm or overrideable per study.
3. Whether MWM quadrant labels are fixed as `NE/NW/SE/SW` or configurable per lab camera orientation.
4. Whether trajectory artifacts use Parquet, CSV, or both for non-technical labs.
