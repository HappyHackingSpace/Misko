/**
 * Paradigm registry (Step 2 - scientific contract).
 *
 * Code-owned ParadigmSpecs. Each paradigm declaratively carries its identity,
 * apparatus/session parameters, zone model, metric sets, event types (with CV
 * `detect` specs), and QC requirements. This pins down "what the system means by
 * each measurement" before the real AI starts producing data. Results are data,
 * never a verdict - there is no pass/fail acceptance layer.
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
  name: "Morris Su Tankı",
  category: PARADIGM_CATEGORIES.LEARNING_MEMORY,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "acquisition", label: "Öğrenme (platform var)", hasTarget: true },
    { key: "probe", label: "Prob (platform yok)", hasTarget: false },
  ],
  apparatusParameters: [
    num("tank_diameter_cm", "Tank çapı", UNITS.CM, { min: 60, max: 250, def: 120 }),
    num("platform_diameter_cm", "Platform çapı", UNITS.CM, { min: 4, max: 20, def: 10 }),
    num("platform_center_x_cm", "Platform merkezi X", UNITS.CM, { min: -125, max: 125, def: 30 }),
    num("platform_center_y_cm", "Platform merkezi Y", UNITS.CM, { min: -125, max: 125, def: -30 }),
    choice("platform_quadrant", "Platform çeyreği", ["NE", "NW", "SE", "SW"], { def: "SE" }),
    num("wall_annulus_width_cm", "Duvar halkası genişliği", UNITS.CM, { min: 4, max: 30, def: 12 }),
    choice("water_opacity", "Su opaklığı", ["opaque", "clear"], { def: "opaque" }),
    num("water_temp_c", "Su sıcaklığı", UNITS.C, { min: 15, max: 30, def: 22, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 10, max: 300, def: 60 }),
    choice("start_position", "Başlangıç konumu", ["N", "E", "S", "W"], { def: "N" }),
    num("trial_index", "Deneme indeksi", UNITS.COUNT, { min: 1, max: 1000, def: 1, required: false }),
  ],
  zones(config = {}) {
    const r = (config.tank_diameter_cm ?? 120) / 2;
    const annulus = config.wall_annulus_width_cm ?? 12;
    return [
      { key: "platform", label: "Platform", type: "circle", role: "target", required: true },
      { key: "target_quadrant", label: "Hedef çeyrek", type: "quadrant", role: "target", required: true },
      { key: "wall_annulus", label: "Duvar halkası", type: "annulus", role: "periphery", required: false, geometryCm: { outerRadius: r, innerRadius: r - annulus } },
    ];
  },
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_dropped_frames_ratio", operator: "<=", value: 0.1, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const OPEN_FIELD = {
  key: "OPEN_FIELD",
  name: "Açık Alan",
  category: PARADIGM_CATEGORIES.ANXIETY,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("arena_width_cm", "Arena genişliği", UNITS.CM, { min: 20, max: 150, def: 50 }),
    num("arena_height_cm", "Arena derinliği", UNITS.CM, { min: 20, max: 150, def: 50 }),
    num("center_fraction", "Merkez oranı", UNITS.RATIO, { min: 0.2, max: 0.8, def: 0.5 }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 60, max: 1800, def: 300 }),
    num("immobility_threshold_cm_s", "Hareketsizlik eşiği", UNITS.CM_S, { min: 0.1, max: 5, def: 2, required: false }),
  ],
  zones(config = {}) {
    const w = config.arena_width_cm ?? 50;
    const h = config.arena_height_cm ?? 50;
    const f = config.center_fraction ?? 0.5;
    return [
      { key: "center", label: "Merkez", type: "polygon", role: "center", required: true, geometryCm: { width: w * f, height: h * f } },
      { key: "periphery", label: "Çevre", type: "polygon", role: "periphery", required: true, derivedFrom: "center" },
    ];
  },
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_dropped_frames_ratio", operator: "<=", value: 0.1, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const EPM = {
  key: "EPM",
  name: "Yükseltilmiş Artı Labirent",
  category: PARADIGM_CATEGORIES.ANXIETY,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("arm_length_cm", "Kol uzunluğu", UNITS.CM, { min: 20, max: 80, def: 35 }),
    num("arm_width_cm", "Kol genişliği", UNITS.CM, { min: 3, max: 15, def: 6 }),
    num("center_size_cm", "Merkez kare kenarı", UNITS.CM, { min: 3, max: 15, def: 6 }),
    num("closed_wall_height_cm", "Kapalı kol duvar yüksekliği", UNITS.CM, { min: 5, max: 40, def: 15, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 120, max: 900, def: 300 }),
  ],
  zones() {
    return [
      { key: "open", label: "Açık kollar", type: "polygon", role: "open", required: true },
      { key: "closed", label: "Kapalı kollar", type: "polygon", role: "closed", required: true },
      { key: "center", label: "Merkez", type: "polygon", role: "center", required: true },
    ];
  },
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
    { key: "fixed_speed", label: "Sabit hız", hasTarget: false },
    { key: "accelerating", label: "Hizlanan", hasTarget: false },
  ],
  apparatusParameters: [
    num("rod_diameter_mm", "Çubuk çapı", UNITS.MM, { min: 10, max: 80, def: 30, required: false }),
    num("min_rpm", "Min devir", UNITS.RPM, { min: 0, max: 80, def: 4 }),
    num("max_rpm", "Maks devir", UNITS.RPM, { min: 1, max: 100, def: 40 }),
    num("acceleration_duration_s", "Hızlanma süresi", UNITS.S, { min: 30, max: 600, def: 300, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 30, max: 600, def: 300 }),
    choice("rotation_mode", "Dönüş modu", ["fixed_speed", "accelerating"], { def: "accelerating" }),
  ],
  zones() {
    return [];
  },
  qc: [{ key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true }],
};

const Y_MAZE = {
  key: "Y_MAZE",
  name: "Y Labirenti",
  category: PARADIGM_CATEGORIES.LEARNING_MEMORY,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "spontaneous", label: "Spontan değişim", hasTarget: false },
    { key: "novel_arm", label: "Yeni kol (2 denemeli)", hasTarget: true },
  ],
  apparatusParameters: [
    num("arm_length_cm", "Kol uzunluğu", UNITS.CM, { min: 20, max: 60, def: 35 }),
    num("arm_width_cm", "Kol genişliği", UNITS.CM, { min: 4, max: 15, def: 7 }),
    num("arm_angle_deg", "Kollar arası açı", UNITS.DEG, { min: 90, max: 120, def: 120, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 120, max: 900, def: 300 }),
  ],
  zones() {
    return [
      { key: "arm_a", label: "A kolu", type: "polygon", role: "open", required: true },
      { key: "arm_b", label: "B kolu", type: "polygon", role: "open", required: true },
      { key: "arm_c", label: "C kolu", type: "polygon", role: "open", required: true },
      { key: "novel", label: "Yeni kol", type: "polygon", role: "target", required: false },
    ];
  },
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const NOVEL_OBJECT = {
  key: "NOVEL_OBJECT",
  name: "Yeni Nesne Tanıma",
  category: PARADIGM_CATEGORIES.LEARNING_MEMORY,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "familiarization", label: "Alistirma", hasTarget: false },
    { key: "test", label: "Test (yeni nesne)", hasTarget: true },
  ],
  apparatusParameters: [
    num("arena_width_cm", "Arena genişliği", UNITS.CM, { min: 30, max: 100, def: 50 }),
    num("arena_height_cm", "Arena derinliği", UNITS.CM, { min: 30, max: 100, def: 50 }),
    num("object_zone_radius_cm", "Nesne keşif yarıçapı", UNITS.CM, { min: 1, max: 10, def: 3 }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 120, max: 900, def: 300 }),
    choice("novel_object_position", "Yeni nesne konumu", ["left", "right"], { def: "right", required: false }),
  ],
  zones() {
    return [
      { key: "novel_object", label: "Yeni nesne", type: "circle", role: "target", required: true },
      { key: "familiar_object", label: "Tanıdık nesne", type: "circle", role: "control", required: true },
    ];
  },
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
    { key: "acquisition", label: "Öğrenme", hasTarget: true },
    { key: "probe", label: "Prob (kaçış kutusu yok)", hasTarget: true },
  ],
  apparatusParameters: [
    num("platform_diameter_cm", "Platform çapı", UNITS.CM, { min: 60, max: 150, def: 92 }),
    num("hole_count", "Delik sayısı", UNITS.COUNT, { min: 12, max: 40, def: 20 }),
    num("hole_diameter_cm", "Delik çapı", UNITS.CM, { min: 3, max: 10, def: 5 }),
    // In the 1..hole_count range; the upper bound is the maximum of hole_count (40).
    // Also validated against the hole_count chosen in the apparatus configuration.
    num("target_hole_index", "Hedef delik indeksi", UNITS.COUNT, { min: 1, max: 40, def: 10, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 60, max: 600, def: 180 }),
  ],
  zones() {
    return [
      { key: "target_hole", label: "Hedef delik", type: "circle", role: "target", required: true },
      { key: "platform", label: "Platform", type: "circle", role: "open", required: true },
    ];
  },
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const THREE_CHAMBER = {
  key: "THREE_CHAMBER",
  name: "Üç Bölmeli Sosyallık",
  category: PARADIGM_CATEGORIES.SOCIAL,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "sociability", label: "Sosyallık", hasTarget: true },
    { key: "social_novelty", label: "Sosyal yenilik", hasTarget: true },
  ],
  apparatusParameters: [
    num("chamber_width_cm", "Bölme genişliği", UNITS.CM, { min: 15, max: 40, def: 20 }),
    num("chamber_height_cm", "Bölme derinliği", UNITS.CM, { min: 20, max: 60, def: 40 }),
    num("interaction_zone_radius_cm", "Etkileşim bölgesi yarıçapı", UNITS.CM, { min: 2, max: 12, def: 5 }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 300, max: 900, def: 600 }),
    choice("social_chamber_side", "Sosyal bölme tarafı", ["left", "right"], { def: "left", required: false }),
  ],
  zones() {
    return [
      { key: "social_chamber", label: "Sosyal bölme", type: "polygon", role: "target", required: true },
      { key: "object_chamber", label: "Nesne bölmesi", type: "polygon", role: "control", required: true },
      { key: "center_chamber", label: "Orta bölme", type: "polygon", role: "center", required: true },
      { key: "interaction", label: "Etkileşim bölgesi", type: "circle", role: "target", required: false },
    ];
  },
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.7, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const LIGHT_DARK = {
  key: "LIGHT_DARK",
  name: "Aydınlık/Karanlık Kutu",
  category: PARADIGM_CATEGORIES.ANXIETY,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("box_width_cm", "Kutu genişliği", UNITS.CM, { min: 20, max: 60, def: 40 }),
    num("box_height_cm", "Kutu derinliği", UNITS.CM, { min: 15, max: 40, def: 20 }),
    num("light_fraction", "Aydınlık bölme oranı", UNITS.RATIO, { min: 0.4, max: 0.6, def: 0.5, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 300, max: 900, def: 600 }),
    choice("start_compartment", "Başlangıç bölmesi", ["light", "dark"], { def: "light" }),
  ],
  zones() {
    return [
      { key: "light", label: "Aydınlık bölme", type: "polygon", role: "risk", required: true },
      { key: "dark", label: "Karanlık bölme", type: "polygon", role: "closed", required: true },
    ];
  },
  qc: [
    { key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true },
    { key: "max_calibration_error_cm", operator: "<=", value: 2, overridable: true },
  ],
};

const POLE = {
  key: "POLE",
  name: "Çubuk (Pole) Testi",
  category: PARADIGM_CATEGORIES.MOTOR,
  species: ["mouse", "rat"],
  trialTypes: [{ key: "standard", label: "Standart", hasTarget: false }],
  apparatusParameters: [
    num("pole_length_cm", "Çubuk uzunluğu", UNITS.CM, { min: 30, max: 100, def: 50 }),
    num("pole_diameter_mm", "Çubuk çapı", UNITS.MM, { min: 5, max: 20, def: 10, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 30, max: 120, def: 60 }),
  ],
  zones() {
    return [
      { key: "top", label: "Tepe", type: "line", role: "open", required: false },
      { key: "base", label: "Taban", type: "line", role: "target", required: false },
    ];
  },
  qc: [{ key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true }],
};

const TREADMILL = {
  key: "TREADMILL",
  name: "Koşu Bandı (Treadmill)",
  category: PARADIGM_CATEGORIES.MOTOR,
  species: ["mouse", "rat"],
  trialTypes: [
    { key: "endurance", label: "Dayanıklılık (hızlanan)", hasTarget: false },
    { key: "fixed_speed", label: "Sabit hız", hasTarget: false },
  ],
  apparatusParameters: [
    num("lane_length_cm", "Şerit uzunluğu", UNITS.CM, { min: 20, max: 80, def: 40 }),
    num("min_belt_speed_cm_s", "Min bant hızı", UNITS.CM_S, { min: 1, max: 50, def: 5 }),
    num("max_belt_speed_cm_s", "Maks bant hızı", UNITS.CM_S, { min: 5, max: 120, def: 40 }),
    num("incline_deg", "Eğim", UNITS.DEG, { min: 0, max: 25, def: 0, required: false }),
  ],
  sessionParameters: [
    num("max_trial_duration_s", "Maks deneme süresi", UNITS.S, { min: 60, max: 7200, def: 1800 }),
    choice("speed_mode", "Hız modu", ["endurance", "fixed_speed"], { def: "endurance" }),
  ],
  zones() {
    return [
      { key: "rear_zone", label: "Geri (uyarı) bölgesi", type: "polygon", role: "risk", required: false },
    ];
  },
  qc: [{ key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true }],
};

/**
 * Event types per paradigm - what an operator can log during a run (and what the
 * CV service derives from signals). They drive the manual event-logging UI and
 * feed the metric engine (docs/METRIC_ENGINE.md). Each event carries a time `t`
 * (seconds from run start); `payload` fields: `zone` is picked from the
 * environment's zones, `number` fields are typed in.
 */
// Each event type carries a `detect` spec: how the CV service should derive it
// from the trajectory/zones. `zone_transition`/`speed_below`/`zone_first_enter`
// are GENERIC (no per-paradigm CV code - declaration is enough). `custom` means a
// dedicated detector/model is required (e.g. a fall, or a pose-based behaviour).
const ZONE_INOUT = [
  { type: "zone_enter", label: "Bölgeye giriş", payload: { zone: "zone" }, detect: { kind: "zone_transition", edge: "enter" } },
  { type: "zone_exit", label: "Bölgeden çıkış", payload: { zone: "zone" }, detect: { kind: "zone_transition", edge: "exit" } },
];
const IMMOBILE = { type: "immobile", label: "Hareketsizlik", payload: { seconds: "number" }, detect: { kind: "speed_below", threshold_cm_s: 2, min_duration_s: 1 } };
const PARADIGM_EVENT_TYPES = Object.freeze({
  OPEN_FIELD: [...ZONE_INOUT, IMMOBILE],
  EPM: [...ZONE_INOUT, { type: "risk_assessment", label: "Risk değerlendirmesi", detect: { kind: "custom" } }],
  LIGHT_DARK: [...ZONE_INOUT, { type: "transition", label: "Aydınlık/karanlık geçişi", detect: { kind: "zone_transition", edge: "enter" } }],
  MWM: [...ZONE_INOUT, { type: "platform_reached", label: "Platforma ulaşma", detect: { kind: "zone_first_enter", zone: "platform" } }, { type: "platform_cross", label: "Platform geçişi", detect: { kind: "zone_transition", edge: "enter", zone: "platform" } }],
  Y_MAZE: [...ZONE_INOUT],
  NOVEL_OBJECT: [{ type: "interaction", label: "Nesne etkileşimi", payload: { object: "text", seconds: "number" }, detect: { kind: "custom" } }],
  BARNES_MAZE: [...ZONE_INOUT, { type: "target_hole", label: "Hedef deliğe ulaşma", detect: { kind: "zone_first_enter", zone: "target" } }, { type: "error", label: "Hata (delik)", payload: { kind: "text" }, detect: { kind: "custom" } }],
  THREE_CHAMBER: [...ZONE_INOUT, { type: "interaction", label: "Etkilesim", payload: { seconds: "number" }, detect: { kind: "custom" } }],
  ROTAROD: [{ type: "fall", label: "Düşme", payload: { rpm: "number" }, detect: { kind: "custom" } }],
  POLE: [{ type: "fall", label: "Düşme", detect: { kind: "custom" } }],
  TREADMILL: [{ type: "shock", label: "Şok", detect: { kind: "custom" } }, { type: "exhaustion", label: "Tükenme", detect: { kind: "custom" } }],
});

/** Attach metric sets + event types to the paradigm (single source: metrics.js). */
function withMetrics(spec) {
  return Object.freeze({
    ...spec,
    schemaVersion: RESULT_SCHEMA_VERSION,
    metrics: metricsForParadigm(spec.key),
    eventTypes: PARADIGM_EVENT_TYPES[spec.key] ?? [],
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
