/**
 * Paradigma kayit defteri (Step 2 - bilimsel kontrat).
 *
 * Kod sahipli ParadigmSpec'ler. Her paradigma; kimlik, apparatus/oturum
 * parametreleri, bolge (zone) modeli, metrik kumeleri, kabul kriterleri ve
 * QC gereksinimlerini deklaratif olarak tasir. Bu, gercek AI veri uretmeye
 * baslamadan once "sistem her olcumle neyi kastediyor" sorusunu sabitler.
 *
 * Kaynak: docs/MEASUREMENTS.md, bolum 2.
 */
import { UNITS } from "./units.js";
import { metricsForParadigm } from "./metrics.js";

/**
 * Sonuc sema surumlemesi. Sonuc gonderiminde (Step 4) bu degerler sonuca
 * gomulur; geriye donuk uyumsuz degisikliklerde major surum artirilir.
 */
export const RESULT_SCHEMA_VERSION = 1;

export const PARADIGM_CATEGORIES = Object.freeze({
  LEARNING_MEMORY: "learning_memory",
  ANXIETY: "anxiety",
  MOTOR: "motor",
  SOCIAL: "social",
});

/** Sayisal apparatus/oturum alani ureten yardimci. */
function num(key, label, unit, { min, max, def, required = true } = {}) {
  return { key, label, type: "number", unit, min, max, default: def, required };
}

/** Secimli alan ureten yardimci. */
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
    num("water_temp_c", "Su sicakligi", UNITS.S, { min: 15, max: 30, def: 22, required: false }),
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
  acceptance: [
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
  acceptance: [],
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
  acceptance: [],
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
    num("rod_diameter_mm", "Cubuk capi", UNITS.CM, { min: 10, max: 80, def: 30, required: false }),
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
  acceptance: [
    { key: "minimum_latency", metricKey: "latency_to_fall_s", operator: ">=", value: "study.minimum_latency_s", overridable: true },
  ],
  qc: [{ key: "min_tracking_confidence", operator: ">=", value: 0.6, overridable: true }],
};

/** Metrik kumelerini paradigmaya bagla (tek kaynak: metrics.js). */
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
});

export const PARADIGM_KEYS = Object.freeze(Object.keys(PARADIGM_SPECS));

/** Tek bir paradigma spec'ini anahtariyla getirir. */
export function getParadigmSpec(key) {
  return PARADIGM_SPECS[key] ?? null;
}

/** Liste gorunumu icin ozet (detay sayfasina girmeden once). */
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
