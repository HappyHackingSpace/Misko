/**
 * Paradigm registry (Step 2 - scientific contract).
 *
 * Code-owned ParadigmSpecs. Each paradigm declaratively carries its identity,
 * apparatus/session parameters, zone model, metric sets, acceptance criteria,
 * and QC requirements. This pins down "what the system means by each measurement"
 * before the real AI starts producing data.
 *
 * Source: docs/MEASUREMENTS.md, section 2.
 */
import { UNITS } from "./units.js";
import { metricsForParadigm } from "./metrics.js";

/**
 * Result schema versioning. On result submission (Step 4) these values are
 * embedded in the result; the major version is bumped on backward-incompatible changes.
 */
export const RESULT_SCHEMA_VERSION = 1;

export const PARADIGM_CATEGORIES = Object.freeze({
  LEARNING_MEMORY: "learning_memory",
  ANXIETY: "anxiety",
  MOTOR: "motor",
  SOCIAL: "social",
});

/** Helper that produces a numeric apparatus/session field. */
function num(key, label, unit, { min, max, def, required = true } = {}) {
  return { key, label, type: "number", unit, min, max, default: def, required };
}

/** Helper that produces an enum field. */
function choice(key, label, options, { def, required = true } = {}) {
  return { key, label, type: "enum", options, default: def, required };
}

const MWM = {
  key: "MWM",
  name: "Morris Su Tanki",
  category: PARADIGM_CATEGORIES.LEARNING_MEMORY,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "acquisition", label: "Ogrenme (platform var)", hasTarget: true },
    { key: "probe", label: "Prob (platform yok)", hasTarget: false },
  ],
  apparatusParameters: [
    num("tank_diameter_cm", "Tank capi", UNITS.CM, { min: 60, max: 250, def: 120 }),
    num("platform_diameter_cm", "Platform capi", UNITS.CM, { min: 4, max: 20, def: 10 }),
    num("platform_center_x_cm", "Platform merkezi X", UNITS.CM, { min: -125, max: 125, def: 30 }),
    num("platform_center_y_cm", "Platform merkezi Y", UNITS.CM, { min: -125, max: 125, def: -30 }),
    choice("platform_quadrant", "Platform ceyregi", ["NE", "NW", "SE", "SW"], { def: "SE" }),
    num("wall_annulus_width_cm", "Duvar halkasi genisligi", UNITS.CM, { min: 4, max: 30, def: 12 }),
    choice("water_opacity", "Su opakligi", ["opaque", "clear"], { def: "opaque" }),
    num("water_temp_c", "Su sicakligi", UNITS.C, { min: 15, max: 30, def: 22, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 10, max: 300, def: 60 }),
    choice("start_position", "Baslangic konumu", ["N", "E", "S", "W"], { def: "N" }),
    num("trial_index", "Deneme indeksi", UNITS.COUNT, { min: 1, max: 1000, def: 1, required: false }),
  ],
  zones(config = {}) {
    const r = (config.tank_diameter_cm ?? 120) / 2;
    const annulus = config.wall_annulus_width_cm ?? 12;
    return [
      { key: "platform", label: "Platform", type: "circle", role: "target", required: true },
      { key: "target_quadrant", label: "Hedef ceyrek", type: "quadrant", role: "target", required: true },
      { key: "wall_annulus", label: "Duvar halkasi", type: "annulus", role: "periphery", required: false, geometryCm: { outerRadius: r, innerRadius: r - annulus } },
    ];
  },
  suggestedAcceptance: [
    { key: "escape_within_trial", metricKey: "escape_latency_s", operator: "<=", value: "max_trial_duration_s", appliesToTrialTypes: ["acquisition"], overridable: true },
  ],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_dropped_frames_ratio", operator: "<=", value: 0.1, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const OPEN_FIELD = {
  key: "OPEN_FIELD",
  name: "Acik Alan",
  category: PARADIGM_CATEGORIES.ANXIETY,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("arena_width_cm", "Arena genisligi", UNITS.CM, { min: 20, max: 150, def: 50 }),
    num("arena_height_cm", "Arena derinligi", UNITS.CM, { min: 20, max: 150, def: 50 }),
    num("center_fraction", "Merkez orani", UNITS.RATIO, { min: 0.2, max: 0.8, def: 0.5 }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 60, max: 1800, def: 300 }),
    num("immobility_threshold_cm_s", "Hareketsizlik esigi", UNITS.CM_S, { min: 0.1, max: 5, def: 2, required: false }),
  ],
  zones(config = {}) {
    const w = config.arena_width_cm ?? 50;
    const h = config.arena_height_cm ?? 50;
    const f = config.center_fraction ?? 0.5;
    return [
      { key: "center", label: "Merkez", type: "polygon", role: "center", required: true, geometryCm: { width: w * f, height: h * f } },
      { key: "periphery", label: "Cevre", type: "polygon", role: "periphery", required: true, derivedFrom: "center" },
    ];
  },
  suggestedAcceptance: [],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_dropped_frames_ratio", operator: "<=", value: 0.1, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const EPM = {
  key: "EPM",
  name: "Yukseltilmis Arti Labirent",
  category: PARADIGM_CATEGORIES.ANXIETY,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("arm_length_cm", "Kol uzunlugu", UNITS.CM, { min: 20, max: 80, def: 35 }),
    num("arm_width_cm", "Kol genisligi", UNITS.CM, { min: 3, max: 15, def: 6 }),
    num("center_size_cm", "Merkez kare kenari", UNITS.CM, { min: 3, max: 15, def: 6 }),
    num("closed_wall_height_cm", "Kapali kol duvar yuksekligi", UNITS.CM, { min: 5, max: 40, def: 15, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 120, max: 900, def: 300 }),
  ],
  zones() {
    return [
      { key: "open", label: "Acik kollar", type: "polygon", role: "open", required: true },
      { key: "closed", label: "Kapali kollar", type: "polygon", role: "closed", required: true },
      { key: "center", label: "Merkez", type: "polygon", role: "center", required: true },
    ];
  },
  suggestedAcceptance: [],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_dropped_frames_ratio", operator: "<=", value: 0.15, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const ROTAROD = {
  key: "ROTAROD",
  name: "Rotarod",
  category: PARADIGM_CATEGORIES.MOTOR,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "fixed_speed", label: "Sabit hiz", hasTarget: false },
    { key: "accelerating", label: "Hizlanan", hasTarget: false },
  ],
  apparatusParameters: [
    num("rod_diameter_mm", "Cubuk capi", UNITS.MM, { min: 10, max: 80, def: 30, required: false }),
    num("min_rpm", "Min devir", UNITS.RPM, { min: 0, max: 80, def: 4 }),
    num("max_rpm", "Maks devir", UNITS.RPM, { min: 1, max: 100, def: 40 }),
    num("acceleration_duration_s", "Hizlanma suresi", UNITS.S, { min: 30, max: 600, def: 300, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 30, max: 600, def: 300 }),
    choice("rotation_mode", "Donus modu", ["fixed_speed", "accelerating"], { def: "accelerating" }),
  ],
  zones() {
    return [];
  },
  suggestedAcceptance: [
    { key: "minimum_latency", metricKey: "latency_to_fall_s", operator: ">=", value: "study.minimum_latency_s", overridable: true },
  ],
  qc: [{ key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true }],
};

const Y_MAZE = {
  key: "Y_MAZE",
  name: "Y Labirenti",
  category: PARADIGM_CATEGORIES.LEARNING_MEMORY,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "spontaneous", label: "Spontan degisim", hasTarget: false },
    { key: "novel_arm", label: "Yeni kol (2 denemeli)", hasTarget: true },
  ],
  apparatusParameters: [
    num("arm_length_cm", "Kol uzunlugu", UNITS.CM, { min: 20, max: 60, def: 35 }),
    num("arm_width_cm", "Kol genisligi", UNITS.CM, { min: 4, max: 15, def: 7 }),
    num("arm_angle_deg", "Kollar arasi aci", UNITS.DEG, { min: 90, max: 120, def: 120, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 120, max: 900, def: 300 }),
  ],
  zones() {
    return [
      { key: "arm_a", label: "A kolu", type: "polygon", role: "open", required: true },
      { key: "arm_b", label: "B kolu", type: "polygon", role: "open", required: true },
      { key: "arm_c", label: "C kolu", type: "polygon", role: "open", required: true },
      { key: "novel", label: "Yeni kol", type: "polygon", role: "target", required: false },
    ];
  },
  suggestedAcceptance: [],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const NOVEL_OBJECT = {
  key: "NOVEL_OBJECT",
  name: "Yeni Nesne Tanima",
  category: PARADIGM_CATEGORIES.LEARNING_MEMORY,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "familiarization", label: "Alistirma", hasTarget: false },
    { key: "test", label: "Test (yeni nesne)", hasTarget: true },
  ],
  apparatusParameters: [
    num("arena_width_cm", "Arena genisligi", UNITS.CM, { min: 30, max: 100, def: 50 }),
    num("arena_height_cm", "Arena derinligi", UNITS.CM, { min: 30, max: 100, def: 50 }),
    num("object_zone_radius_cm", "Nesne kesif yaricapi", UNITS.CM, { min: 1, max: 10, def: 3 }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 120, max: 900, def: 300 }),
    choice("novel_object_position", "Yeni nesne konumu", ["left", "right"], { def: "right", required: false }),
  ],
  zones() {
    return [
      { key: "novel_object", label: "Yeni nesne", type: "circle", role: "target", required: true },
      { key: "familiar_object", label: "Tanidik nesne", type: "circle", role: "control", required: true },
    ];
  },
  suggestedAcceptance: [],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.75, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 1.5, overridable: true },
  ],
};

const BARNES_MAZE = {
  key: "BARNES_MAZE",
  name: "Barnes Labirenti",
  category: PARADIGM_CATEGORIES.LEARNING_MEMORY,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "acquisition", label: "Ogrenme", hasTarget: true },
    { key: "probe", label: "Prob (kacis kutusu yok)", hasTarget: true },
  ],
  apparatusParameters: [
    num("platform_diameter_cm", "Platform capi", UNITS.CM, { min: 60, max: 150, def: 92 }),
    num("hole_count", "Delik sayisi", UNITS.COUNT, { min: 12, max: 40, def: 20 }),
    num("hole_diameter_cm", "Delik capi", UNITS.CM, { min: 3, max: 10, def: 5 }),
    // In the 1..hole_count range; the upper bound is the maximum of hole_count (40).
    // Also validated against the hole_count chosen in the apparatus configuration.
    num("target_hole_index", "Hedef delik indeksi", UNITS.COUNT, { min: 1, max: 40, def: 10, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 60, max: 600, def: 180 }),
  ],
  zones() {
    return [
      { key: "target_hole", label: "Hedef delik", type: "circle", role: "target", required: true },
      { key: "platform", label: "Platform", type: "circle", role: "open", required: true },
    ];
  },
  suggestedAcceptance: [
    { key: "reach_within_trial", metricKey: "primary_latency_s", operator: "<=", value: "max_trial_duration_s", appliesToTrialTypes: ["acquisition"], overridable: true },
  ],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const THREE_CHAMBER = {
  key: "THREE_CHAMBER",
  name: "Uc Bolmeli Sosyallik",
  category: PARADIGM_CATEGORIES.SOCIAL,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "sociability", label: "Sosyallik", hasTarget: true },
    { key: "social_novelty", label: "Sosyal yenilik", hasTarget: true },
  ],
  apparatusParameters: [
    num("chamber_width_cm", "Bolme genisligi", UNITS.CM, { min: 15, max: 40, def: 20 }),
    num("chamber_height_cm", "Bolme derinligi", UNITS.CM, { min: 20, max: 60, def: 40 }),
    num("interaction_zone_radius_cm", "Etkilesim bolgesi yaricapi", UNITS.CM, { min: 2, max: 12, def: 5 }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 300, max: 900, def: 600 }),
    choice("social_chamber_side", "Sosyal bolme tarafi", ["left", "right"], { def: "left", required: false }),
  ],
  zones() {
    return [
      { key: "social_chamber", label: "Sosyal bolme", type: "polygon", role: "target", required: true },
      { key: "object_chamber", label: "Nesne bolmesi", type: "polygon", role: "control", required: true },
      { key: "center_chamber", label: "Orta bolme", type: "polygon", role: "center", required: true },
      { key: "interaction", label: "Etkilesim bolgesi", type: "circle", role: "target", required: false },
    ];
  },
  suggestedAcceptance: [],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const LIGHT_DARK = {
  key: "LIGHT_DARK",
  name: "Aydinlik/Karanlik Kutu",
  category: PARADIGM_CATEGORIES.ANXIETY,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("box_width_cm", "Kutu genisligi", UNITS.CM, { min: 20, max: 60, def: 40 }),
    num("box_height_cm", "Kutu derinligi", UNITS.CM, { min: 15, max: 40, def: 20 }),
    num("light_fraction", "Aydinlik bolme orani", UNITS.RATIO, { min: 0.4, max: 0.6, def: 0.5, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 300, max: 900, def: 600 }),
    choice("start_compartment", "Baslangic bolmesi", ["light", "dark"], { def: "light" }),
  ],
  zones() {
    return [
      { key: "light", label: "Aydinlik bolme", type: "polygon", role: "risk", required: true },
      { key: "dark", label: "Karanlik bolme", type: "polygon", role: "closed", required: true },
    ];
  },
  suggestedAcceptance: [],
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const POLE = {
  key: "POLE",
  name: "Cubuk (Pole) Testi",
  category: PARADIGM_CATEGORIES.MOTOR,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("pole_length_cm", "Cubuk uzunlugu", UNITS.CM, { min: 30, max: 100, def: 50 }),
    num("pole_diameter_mm", "Cubuk capi", UNITS.MM, { min: 5, max: 20, def: 10, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 30, max: 120, def: 60 }),
  ],
  zones() {
    return [
      { key: "top", label: "Tepe", type: "line", role: "open", required: false },
      { key: "base", label: "Taban", type: "line", role: "target", required: false },
    ];
  },
  suggestedAcceptance: [],
  qc: [{ key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true }],
};

const TREADMILL = {
  key: "TREADMILL",
  name: "Kosu Bandi (Treadmill)",
  category: PARADIGM_CATEGORIES.MOTOR,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "endurance", label: "Dayaniklilik (hizlanan)", hasTarget: false },
    { key: "fixed_speed", label: "Sabit hiz", hasTarget: false },
  ],
  apparatusParameters: [
    num("lane_length_cm", "Serit uzunlugu", UNITS.CM, { min: 20, max: 80, def: 40 }),
    num("min_belt_speed_cm_s", "Min bant hizi", UNITS.CM_S, { min: 1, max: 50, def: 5 }),
    num("max_belt_speed_cm_s", "Maks bant hizi", UNITS.CM_S, { min: 5, max: 120, def: 40 }),
    num("incline_deg", "Egim", UNITS.DEG, { min: 0, max: 25, def: 0, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme suresi", UNITS.S, { min: 60, max: 7200, def: 1800 }),
    choice("speed_mode", "Hiz modu", ["endurance", "fixed_speed"], { def: "endurance" }),
  ],
  zones() {
    return [
      { key: "rear_zone", label: "Geri (uyari) bolgesi", type: "polygon", role: "risk", required: false },
    ];
  },
  suggestedAcceptance: [],
  qc: [{ key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true }],
};

/** Attach metric sets to the paradigm (single source: metrics.js). */
function withMetrics(spec) {
  return Object.freeze({
    ...spec,
    schemaVersion: RESULT_SCHEMA_VERSION,
    metrics: metricsForParadigm(spec.key),
  });
}

/** @type {Record<string, object>} */
export const PARADIGM_SPECS = Object.freeze({
  MWM: withMetrics(MWM),
  OPEN_FIELD: withMetrics(OPEN_FIELD),
  EPM: withMetrics(EPM),
  ROTAROD: withMetrics(ROTAROD),
  Y_MAZE: withMetrics(Y_MAZE),
  NOVEL_OBJECT: withMetrics(NOVEL_OBJECT),
  BARNES_MAZE: withMetrics(BARNES_MAZE),
  THREE_CHAMBER: withMetrics(THREE_CHAMBER),
  LIGHT_DARK: withMetrics(LIGHT_DARK),
  POLE: withMetrics(POLE),
  TREADMILL: withMetrics(TREADMILL),
});

export const PARADIGM_KEYS = Object.freeze(Object.keys(PARADIGM_SPECS));

/** Fetches a single paradigm spec by key. */
export function getParadigmSpec(key) {
  return PARADIGM_SPECS[key] ?? null;
}

/** Summary for the list view (before entering the detail page). */
export function listParadigmSummaries() {
  return PARADIGM_KEYS.map((key) => {
    const s = PARADIGM_SPECS[key];
    return {
      key: s.key,
      name: s.name,
      category: s.category,
      species: s.species,
      trialTypes: s.trialTypes,
      metricCount: s.metrics.length,
    };
  });
}
