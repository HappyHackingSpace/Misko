/**
 * Measurement dictionary (Step 2 - scientific contract).
 *
 * Code-owned MetricDefinition registry. Goal: prevent two services from using
 * the same name for different computations. On result submission (Step 4),
 * undefined metric keys are rejected.
 *
 * Source: docs/MEASUREMENTS.md, section 3.
 *
 * Templated keys (e.g. `zone_time_s.{zoneKey}`) are marked with
 * `templated: true`; the real key is derived from the paradigm's zone definition.
 */
import { UNITS } from "./units.js";

/**
 * @typedef {Object} MetricDefinition
 * @property {string} key                Canonical metric key (or template).
 * @property {string} label              Human-readable label.
 * @property {string[]} paradigmKeys     Paradigms this metric is valid for.
 * @property {string} unit               Canonical unit from UNITS.
 * @property {"number"|"integer"|"boolean"|"object"} valueType
 * @property {boolean} required          Required in the result?
 * @property {boolean} [templated]       Is the key completed with a zone/quadrant?
 * @property {string} definition         What the metric is.
 * @property {string} formula            Computation summary.
 * @property {string[]} inputs           Computation inputs.
 * @property {("per_trial"|"per_session"|"per_subject_timepoint"|"study_summary")} aggregation
 * @property {[number, number]|null} validRange
 * @property {string[]} qcDependencies   QC rules it depends on.
 */

// Paradigm groups - to manage in one place which paradigms baseline metrics
// are valid for. When a new paradigm is added, just add it to the relevant
// group.
const ALL_PARADIGMS = [
  "MWM", "OPEN_FIELD", "EPM", "ROTAROD", "Y_MAZE", "NOVEL_OBJECT",
  "BARNES_MAZE", "THREE_CHAMBER", "LIGHT_DARK", "POLE", "TREADMILL",
];
// Free-moving, path-tracked (locomotion) paradigms.
const LOCOMOTION = [
  "MWM", "OPEN_FIELD", "EPM", "Y_MAZE", "NOVEL_OBJECT",
  "BARNES_MAZE", "THREE_CHAMBER", "LIGHT_DARK", "TREADMILL",
];
// Zone-based arena paradigms.
const ARENA_ZONED = [
  "MWM", "OPEN_FIELD", "EPM", "Y_MAZE", "NOVEL_OBJECT",
  "BARNES_MAZE", "THREE_CHAMBER", "LIGHT_DARK",
];

/** @type {MetricDefinition[]} */
const BASELINE = [
  {
    key: "duration_s",
    label: "Analiz süresi",
    paradigmKeys: ALL_PARADIGMS,
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Geçersiz kareler kırpıldıktan sonra analiz edilen zaman penceresi.",
    formula: "last_valid_t - first_valid_t",
    inputs: ["timestamps", "valid_frame_mask"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["min_tracking_confidence", "max_dropped_frames_ratio"],
  },
  {
    key: "distance_cm",
    label: "Toplam yol",
    paradigmKeys: LOCOMOTION,
    unit: UNITS.CM,
    valueType: "number",
    required: true,
    definition: "Apparatus koordinatlarında toplam yol uzunluğu.",
    formula: "sum(|p[i] - p[i-1]|) for valid frames",
    inputs: ["trajectory_cm", "valid_frame_mask"],
    aggregation: "per_trial",
    validRange: [0, 1000000],
    qcDependencies: ["max_calibration_error_cm", "min_tracking_confidence"],
  },
  {
    key: "mean_speed_cm_s",
    label: "Ortalama hız",
    paradigmKeys: LOCOMOTION,
    unit: UNITS.CM_S,
    valueType: "number",
    required: false,
    definition: "Analiz penceresi boyunca ortalama hareket hızı.",
    formula: "distance_cm / duration_s",
    inputs: ["distance_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 10000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "max_speed_cm_s",
    label: "Maksimum hız",
    paradigmKeys: ARENA_ZONED,
    unit: UNITS.CM_S,
    valueType: "number",
    required: false,
    definition: "Yumuşatılmış anlık hızın maksimumu.",
    formula: "max(smoothed_instantaneous_speed)",
    inputs: ["trajectory_cm", "timestamps"],
    aggregation: "per_trial",
    validRange: [0, 10000],
    qcDependencies: ["max_calibration_error_cm", "min_tracking_confidence"],
  },
  {
    key: "immobility_s",
    label: "Hareketsizlik süresi",
    paradigmKeys: ["OPEN_FIELD", "EPM", "LIGHT_DARK"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    definition: "Paradigmaya özgü hareket eşiğinin altında geçirilen süre.",
    formula: "sum(dt where speed < immobility_threshold_cm_s)",
    inputs: ["instantaneous_speed", "timestamps"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "zone_time_s",
    label: "Bölge süresi",
    paradigmKeys: ARENA_ZONED,
    unit: UNITS.S,
    valueType: "number",
    required: false,
    templated: true,
    definition: "Tanımlı bir bölge içinde geçirilen süre (anahtar: zone_time_s.{zoneKey}).",
    formula: "sum(dt where position in zone)",
    inputs: ["trajectory_cm", "zone_geometry_cm", "timestamps"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "zone_entries",
    label: "Bölge girişleri",
    paradigmKeys: ARENA_ZONED,
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    templated: true,
    definition: "Debounce sonrası bir bölgeye giriş sayısı (anahtar: zone_entries.{zoneKey}).",
    formula: "count(transitions outside->inside after debounce)",
    inputs: ["trajectory_cm", "zone_geometry_cm", "entry_debounce_s"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "latency_to_zone_s",
    label: "Bölgeye varış gecikmesi",
    paradigmKeys: ["MWM", "EPM", "BARNES_MAZE", "LIGHT_DARK"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    templated: true,
    definition: "Deneme başlangıcından bir bölgeye ilk geçerli girişe kadar geçen süre (anahtar: latency_to_zone_s.{zoneKey}).",
    formula: "first_valid_entry_t - trial_start_t",
    inputs: ["trajectory_cm", "zone_geometry_cm", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "path_efficiency_ratio",
    label: "Yol verimliliği",
    paradigmKeys: ["MWM", "BARNES_MAZE"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Hedefe düz çizgi mesafesinin gerçek yol uzunluğuna oranı.",
    formula: "straight_line_to_target_cm / distance_cm",
    inputs: ["trajectory_cm", "target_center_cm"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
];

/** @type {MetricDefinition[]} */
const MWM = [
  {
    key: "escape_latency_s",
    label: "Kaçış gecikmesi",
    paradigmKeys: ["MWM"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "İlk sürekli platform-bölgesi girişine kadar geçen süre.",
    formula: "first_sustained_platform_entry_t - trial_start_t",
    inputs: ["trajectory_cm", "platform_zone_cm", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 300],
    qcDependencies: ["max_calibration_error_cm", "min_tracking_confidence"],
  },
  {
    key: "path_length_cm",
    label: "Yüzme yolu",
    paradigmKeys: ["MWM"],
    unit: UNITS.CM,
    valueType: "number",
    required: true,
    definition: "Platforma veya deneme sonuna kadar toplam yüzme yolu.",
    formula: "sum(|p[i] - p[i-1]|) until platform or trial end",
    inputs: ["trajectory_cm", "platform_zone_cm"],
    aggregation: "per_trial",
    validRange: [0, 1000000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "mean_swim_speed_cm_s",
    label: "Ortalama yüzme hızı",
    paradigmKeys: ["MWM"],
    unit: UNITS.CM_S,
    valueType: "number",
    required: false,
    definition: "Öğrenme etkisini motor bozukluktan ayırmak için kullanılır.",
    formula: "path_length_cm / duration_s",
    inputs: ["path_length_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 200],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "platform_crossings_count",
    label: "Platform geçişleri",
    paradigmKeys: ["MWM"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Sadece prob denemeleri: eski platform bölgesinden geçiş sayısı.",
    formula: "count(crossings through former platform zone)",
    inputs: ["trajectory_cm", "platform_zone_cm"],
    aggregation: "per_trial",
    validRange: [0, 1000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "target_quadrant_time_ratio",
    label: "Hedef çeyrek süresi oranı",
    paradigmKeys: ["MWM"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Hedef çeyrekte geçen sürenin geçerli analiz süresine oranı.",
    formula: "quadrant_time_s.target / duration_s",
    inputs: ["trajectory_cm", "quadrant_geometry_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "quadrant_time_s",
    label: "Çeyrek süresi",
    paradigmKeys: ["MWM"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    templated: true,
    definition: "Bir çeyrekte geçen süre (anahtar: quadrant_time_s.{quadrant}, ör. NE/NW/SE/SW).",
    formula: "sum(dt where position in quadrant)",
    inputs: ["trajectory_cm", "quadrant_geometry_cm"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "thigmotaxis_time_ratio",
    label: "Tigmotaksi süresi oranı",
    paradigmKeys: ["MWM"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Tank duvarına yakın halka bölgede geçen süre oranı.",
    formula: "annulus_time_s / duration_s",
    inputs: ["trajectory_cm", "wall_annulus_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "mean_distance_to_platform_cm",
    label: "Platforma ortalama mesafe",
    paradigmKeys: ["MWM"],
    unit: UNITS.CM,
    valueType: "number",
    required: false,
    definition: "Hedefe ortalama yakınlık; prob denemelerinde sağlam bir ölçüt.",
    formula: "mean(|position - platform_center_cm|)",
    inputs: ["trajectory_cm", "platform_center_cm"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "heading_error_deg",
    label: "Yönelim hatası",
    paradigmKeys: ["MWM"],
    unit: UNITS.DEG,
    valueType: "number",
    required: false,
    definition: "Opsiyonel; heading veya yumuşatılmış yol vektörü gerektirir.",
    formula: "angle(path_vector, vector_to_target)",
    inputs: ["trajectory_cm", "platform_center_cm"],
    aggregation: "per_trial",
    validRange: [0, 180],
    qcDependencies: ["min_tracking_confidence"],
  },
];

/** @type {MetricDefinition[]} */
const OPEN_FIELD = [
  {
    key: "center_time_ratio",
    label: "Merkez süresi oranı",
    paradigmKeys: ["OPEN_FIELD"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: true,
    definition: "Merkez bölgede geçen sürenin geçerli süreye oranı.",
    formula: "zone_time_s.center / duration_s",
    inputs: ["trajectory_cm", "center_zone_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "periphery_time_ratio",
    label: "Çevre süresi oranı",
    paradigmKeys: ["OPEN_FIELD"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Çevre bölgede geçen sürenin geçerli süreye oranı.",
    formula: "zone_time_s.periphery / duration_s",
    inputs: ["trajectory_cm", "periphery_zone_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "center_entries_count",
    label: "Merkez girişleri",
    paradigmKeys: ["OPEN_FIELD"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Debounce sonrası merkez bölge girişleri.",
    formula: "count(transitions periphery->center after debounce)",
    inputs: ["trajectory_cm", "center_zone_cm", "entry_debounce_s"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
];

/** @type {MetricDefinition[]} */
const EPM = [
  {
    key: "open_arm_time_ratio",
    label: "Açık kol süresi oranı",
    paradigmKeys: ["EPM"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: true,
    definition: "Açık kollarda geçen sürenin geçerli süreye oranı.",
    formula: "zone_time_s.open / duration_s",
    inputs: ["trajectory_cm", "open_arm_zone_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "closed_arm_time_ratio",
    label: "Kapalı kol süresi oranı",
    paradigmKeys: ["EPM"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Kapalı kollarda geçen sürenin geçerli süreye oranı.",
    formula: "zone_time_s.closed / duration_s",
    inputs: ["trajectory_cm", "closed_arm_zone_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "open_arm_entries_count",
    label: "Açık kol girişleri",
    paradigmKeys: ["EPM"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Debounce sonrası açık kol girişleri.",
    formula: "count(transitions ->open arm after debounce)",
    inputs: ["trajectory_cm", "open_arm_zone_cm", "entry_debounce_s"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "closed_arm_entries_count",
    label: "Kapalı kol girişleri",
    paradigmKeys: ["EPM"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Debounce sonrası kapalı kol girişleri.",
    formula: "count(transitions ->closed arm after debounce)",
    inputs: ["trajectory_cm", "closed_arm_zone_cm", "entry_debounce_s"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "latency_to_open_arm_s",
    label: "Açık kola varış gecikmesi",
    paradigmKeys: ["EPM"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    definition: "Bir açık kola ilk giriş süresi.",
    formula: "first_open_arm_entry_t - trial_start_t",
    inputs: ["trajectory_cm", "open_arm_zone_cm", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "risk_assessment_count",
    label: "Risk değerlendirme sayısı",
    paradigmKeys: ["EPM"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Opsiyonel; davranış sınıflandırıcı varsa olay sayısı.",
    formula: "count(risk_assessment_events)",
    inputs: ["behavior_events"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["min_tracking_confidence"],
  },
];

/** @type {MetricDefinition[]} */
const ROTAROD = [
  {
    key: "latency_to_fall_s",
    label: "Düşme gecikmesi",
    paradigmKeys: ["ROTAROD"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Deneme başlangıcından düşme olayına kadar geçen süre.",
    formula: "fall_event_t - trial_start_t",
    inputs: ["fall_event_t", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 3600],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "rpm_at_fall",
    label: "Düşme anındaki devir",
    paradigmKeys: ["ROTAROD"],
    unit: UNITS.RPM,
    valueType: "number",
    required: false,
    definition: "Mod ve geçen süreden türetilir.",
    formula: "rpm(mode, latency_to_fall_s)",
    inputs: ["latency_to_fall_s", "rotation_mode"],
    aggregation: "per_trial",
    validRange: [0, 100],
    qcDependencies: [],
  },
  {
    key: "trial_duration_s",
    label: "Deneme süresi",
    paradigmKeys: ["ROTAROD"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    definition: "Denek düşmezse maksimum süreye eşit olabilir.",
    formula: "min(fall_event_t - trial_start_t, max_trial_duration_s)",
    inputs: ["fall_event_t", "trial_start_t", "max_trial_duration_s"],
    aggregation: "per_trial",
    validRange: [0, 3600],
    qcDependencies: [],
  },
  {
    key: "fall_detected",
    label: "Düşme algılandı",
    paradigmKeys: ["ROTAROD", "POLE"],
    unit: UNITS.BOOLEAN,
    valueType: "boolean",
    required: false,
    definition: "Bir düşme olayının algılanıp algılanmadığı.",
    formula: "fall_event_t != null",
    inputs: ["fall_event_t"],
    aggregation: "per_trial",
    validRange: null,
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "learning_slope",
    label: "Öğrenme eğimi",
    paradigmKeys: ["ROTAROD"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Tekrarlı denemeler arasında çalışma seviyesinde toplanır; tek deneme CV metriği değildir.",
    formula: "slope(latency_to_fall_s over trial_index)",
    inputs: ["latency_to_fall_s", "trial_index"],
    aggregation: "study_summary",
    validRange: null,
    qcDependencies: [],
  },
];

/** @type {MetricDefinition[]} - Y Maze (spatial working memory) */
const Y_MAZE = [
  {
    key: "spontaneous_alternation_ratio",
    label: "Spontan değişim oranı",
    paradigmKeys: ["Y_MAZE"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: true,
    definition: "Ardışık üçlü kol dizilerindeki doğru değişim oranı.",
    formula: "alternations / (total_arm_entries - 2)",
    inputs: ["arm_entry_sequence"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "total_arm_entries_count",
    label: "Toplam kol girişi",
    paradigmKeys: ["Y_MAZE"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: true,
    definition: "Tüm kollara toplam giriş sayısı; lokomotor aktivite göstergesi.",
    formula: "count(arm_entries)",
    inputs: ["arm_entry_sequence"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "novel_arm_time_ratio",
    label: "Yeni kol süresi oranı",
    paradigmKeys: ["Y_MAZE"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "İki denemeli protokolde yeni kolda geçen süre oranı.",
    formula: "zone_time_s.novel / duration_s",
    inputs: ["trajectory_cm", "novel_arm_zone_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
];

/** @type {MetricDefinition[]} - Novel Object Recognition (NOR) */
const NOVEL_OBJECT = [
  {
    key: "novel_object_time_s",
    label: "Yeni nesne keşif süresi",
    paradigmKeys: ["NOVEL_OBJECT"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Yeni nesneyi aktif keşfetme süresi (burun nesneye yönelik).",
    formula: "sum(dt where exploring novel object)",
    inputs: ["trajectory_cm", "object_zones_cm", "head_direction"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "familiar_object_time_s",
    label: "Tanıdık nesne keşif süresi",
    paradigmKeys: ["NOVEL_OBJECT"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Tanıdık nesneyi aktif keşfetme süresi.",
    formula: "sum(dt where exploring familiar object)",
    inputs: ["trajectory_cm", "object_zones_cm", "head_direction"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "discrimination_index",
    label: "Ayrım indeksi",
    paradigmKeys: ["NOVEL_OBJECT"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Tanıma belleği ölçütü; -1 (tanıdık) ile +1 (yeni) arasında.",
    formula: "(novel - familiar) / (novel + familiar)",
    inputs: ["novel_object_time_s", "familiar_object_time_s"],
    aggregation: "per_trial",
    validRange: [-1, 1],
    qcDependencies: [],
  },
  {
    key: "total_exploration_time_s",
    label: "Toplam keşif süresi",
    paradigmKeys: ["NOVEL_OBJECT"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    definition: "Her iki nesneyi keşfetme süresinin toplamı.",
    formula: "novel_object_time_s + familiar_object_time_s",
    inputs: ["novel_object_time_s", "familiar_object_time_s"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: [],
  },
];

/** @type {MetricDefinition[]} - Barnes Maze (spatial memory, dry) */
const BARNES_MAZE = [
  {
    key: "primary_latency_s",
    label: "Birincil gecikme",
    paradigmKeys: ["BARNES_MAZE"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Hedef deliğe ilk ulaşma süresi.",
    formula: "first_target_hole_visit_t - trial_start_t",
    inputs: ["trajectory_cm", "target_hole_cm", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 3600],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "primary_errors_count",
    label: "Birincil hata",
    paradigmKeys: ["BARNES_MAZE"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Hedef deliğe ulaşmadan önce yapılan yanlış delik ziyaretleri.",
    formula: "count(non_target_hole_visits before first target visit)",
    inputs: ["trajectory_cm", "hole_zones_cm"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "total_errors_count",
    label: "Toplam hata",
    paradigmKeys: ["BARNES_MAZE"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Deneme boyunca toplam yanlış delik ziyareti.",
    formula: "count(non_target_hole_visits)",
    inputs: ["trajectory_cm", "hole_zones_cm"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
];

/** @type {MetricDefinition[]} - Three-Chamber Sociability */
const THREE_CHAMBER = [
  {
    key: "social_chamber_time_s",
    label: "Sosyal bölme süresi",
    paradigmKeys: ["THREE_CHAMBER"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Uyaran fareyi içeren bölmede geçen süre.",
    formula: "zone_time_s.social_chamber",
    inputs: ["trajectory_cm", "chamber_zones_cm"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "object_chamber_time_s",
    label: "Nesne bölmesi süresi",
    paradigmKeys: ["THREE_CHAMBER"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Boş kafes/nesne bulunan bölmede geçen süre.",
    formula: "zone_time_s.object_chamber",
    inputs: ["trajectory_cm", "chamber_zones_cm"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "sociability_index",
    label: "Sosyallık indeksi",
    paradigmKeys: ["THREE_CHAMBER"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: false,
    definition: "Sosyal tercih ölçütü; -1 (nesne) ile +1 (sosyal) arasında.",
    formula: "(social - object) / (social + object)",
    inputs: ["social_chamber_time_s", "object_chamber_time_s"],
    aggregation: "per_trial",
    validRange: [-1, 1],
    qcDependencies: [],
  },
  {
    key: "interaction_time_s",
    label: "Yakın etkileşim süresi",
    paradigmKeys: ["THREE_CHAMBER"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    definition: "Uyaran kafesi etrafındaki etkileşim bölgesinde geçen süre.",
    formula: "zone_time_s.interaction",
    inputs: ["trajectory_cm", "interaction_zone_cm"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
];

/** @type {MetricDefinition[]} - Light/Dark Box (anxiety) */
const LIGHT_DARK = [
  {
    key: "light_time_ratio",
    label: "Aydınlık bölme süresi oranı",
    paradigmKeys: ["LIGHT_DARK"],
    unit: UNITS.RATIO,
    valueType: "number",
    required: true,
    definition: "Aydınlık bölmede geçen sürenin geçerli süreye oranı.",
    formula: "zone_time_s.light / duration_s",
    inputs: ["trajectory_cm", "light_zone_cm", "duration_s"],
    aggregation: "per_trial",
    validRange: [0, 1],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "light_entries_count",
    label: "Aydınlık bölme girişleri",
    paradigmKeys: ["LIGHT_DARK"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Debounce sonrası aydınlık bölmeye giriş sayısı.",
    formula: "count(transitions dark->light after debounce)",
    inputs: ["trajectory_cm", "light_zone_cm", "entry_debounce_s"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "latency_to_dark_s",
    label: "Karanlığa giriş gecikmesi",
    paradigmKeys: ["LIGHT_DARK"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    definition: "Aydınlık başlangıçtan karanlık bölmeye ilk giriş süresi.",
    formula: "first_dark_entry_t - trial_start_t",
    inputs: ["trajectory_cm", "dark_zone_cm", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 86400],
    qcDependencies: ["max_calibration_error_cm"],
  },
  {
    key: "transitions_count",
    label: "Bölme geçişleri",
    paradigmKeys: ["LIGHT_DARK"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Aydınlık ve karanlık bölmeler arası toplam geçiş sayısı.",
    formula: "count(light<->dark transitions)",
    inputs: ["trajectory_cm", "light_zone_cm", "dark_zone_cm"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["max_calibration_error_cm"],
  },
];

/** @type {MetricDefinition[]} - Pole Test (motor/bradykinesia) */
const POLE = [
  {
    key: "t_turn_s",
    label: "Dönme süresi",
    paradigmKeys: ["POLE"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Tepede aşağı dönmeyi tamamlama süresi.",
    formula: "turn_complete_t - trial_start_t",
    inputs: ["pose_or_orientation", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 120],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "t_total_s",
    label: "Toplam iniş süresi",
    paradigmKeys: ["POLE"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Tabana ulaşana kadar geçen toplam süre.",
    formula: "base_reached_t - trial_start_t",
    inputs: ["trajectory_cm", "trial_start_t"],
    aggregation: "per_trial",
    validRange: [0, 120],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "descent_speed_cm_s",
    label: "İniş hızı",
    paradigmKeys: ["POLE"],
    unit: UNITS.CM_S,
    valueType: "number",
    required: false,
    definition: "Ortalama dikey iniş hızı.",
    formula: "pole_length_cm / t_total_s",
    inputs: ["pole_length_cm", "t_total_s"],
    aggregation: "per_trial",
    validRange: [0, 1000],
    qcDependencies: ["max_calibration_error_cm"],
  },
];

/** @type {MetricDefinition[]} - Treadmill (endurance/gait) */
const TREADMILL = [
  {
    key: "run_time_s",
    label: "Koşu süresi",
    paradigmKeys: ["TREADMILL"],
    unit: UNITS.S,
    valueType: "number",
    required: true,
    definition: "Bitkinlik veya deneme sonuna kadar aktif koşu süresi.",
    formula: "exhaustion_t - trial_start_t",
    inputs: ["trial_start_t", "exhaustion_t"],
    aggregation: "per_trial",
    validRange: [0, 7200],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "run_distance_cm",
    label: "Koşu mesafesi",
    paradigmKeys: ["TREADMILL"],
    unit: UNITS.CM,
    valueType: "number",
    required: false,
    definition: "Bant hızı ve koşu süresinden türetilen toplam mesafe.",
    formula: "integral(belt_speed_cm_s dt) over run_time_s",
    inputs: ["belt_speed_profile", "run_time_s"],
    aggregation: "per_trial",
    validRange: [0, 10000000],
    qcDependencies: [],
  },
  {
    key: "latency_to_exhaustion_s",
    label: "Bitkinlik gecikmesi",
    paradigmKeys: ["TREADMILL"],
    unit: UNITS.S,
    valueType: "number",
    required: false,
    definition: "Bitkinlik kriterine ulaşana kadar geçen süre.",
    formula: "exhaustion_t - trial_start_t",
    inputs: ["trial_start_t", "exhaustion_t"],
    aggregation: "per_trial",
    validRange: [0, 7200],
    qcDependencies: ["min_tracking_confidence"],
  },
  {
    key: "shock_count",
    label: "Uyarı/şok sayısı",
    paradigmKeys: ["TREADMILL"],
    unit: UNITS.COUNT,
    valueType: "integer",
    required: false,
    definition: "Bitkinlik kriteri olarak sayılan geri bölge temas/uyarı sayısı.",
    formula: "count(shock_grid_contacts)",
    inputs: ["rear_zone_contacts"],
    aggregation: "per_trial",
    validRange: [0, 100000],
    qcDependencies: ["min_tracking_confidence"],
  },
];

/** @type {MetricDefinition[]} */
export const METRIC_DEFINITIONS = Object.freeze(
  [
    ...BASELINE,
    ...MWM,
    ...OPEN_FIELD,
    ...EPM,
    ...ROTAROD,
    ...Y_MAZE,
    ...NOVEL_OBJECT,
    ...BARNES_MAZE,
    ...THREE_CHAMBER,
    ...LIGHT_DARK,
    ...POLE,
    ...TREADMILL,
  ].map((m) => Object.freeze({ normalization: null, ...m })),
);

const BY_KEY = new Map(METRIC_DEFINITIONS.map((m) => [m.key, m]));

/** Fetches a single metric definition by key (including the template root key). */
export function getMetricDefinition(key) {
  return BY_KEY.get(key) ?? null;
}

/** Returns the metric definitions belonging to a paradigm. */
export function metricsForParadigm(paradigmKey) {
  return METRIC_DEFINITIONS.filter((m) => m.paradigmKeys.includes(paradigmKey));
}

function inRange(def, v) {
  if (!def.validRange) return true;
  return v >= def.validRange[0] && v <= def.validRange[1];
}

/** Checks one value against its metric definition, collecting errors. */
function checkMetricValue(def, key, value, errors) {
  // Templated metrics (e.g. zone_time_s) are stored as a { zoneKey: number } map.
  if (def.templated) {
    if (typeof value !== "object" || value == null || Array.isArray(value)) {
      errors.push(`${key} must be an object of zone values`);
      return;
    }
    for (const [zk, zv] of Object.entries(value)) {
      if (typeof zv !== "number" || !Number.isFinite(zv)) errors.push(`${key}.${zk} must be numeric`);
      else if (!inRange(def, zv)) errors.push(`${key}.${zk} is out of range`);
    }
    return;
  }
  switch (def.valueType) {
    case "boolean":
      if (typeof value !== "boolean") errors.push(`${key} must be a boolean`);
      break;
    case "integer":
      if (typeof value !== "number" || !Number.isInteger(value)) errors.push(`${key} must be an integer`);
      else if (!inRange(def, value)) errors.push(`${key} is out of range`);
      break;
    default: // "number"
      if (typeof value !== "number" || !Number.isFinite(value)) errors.push(`${key} must be numeric`);
      else if (!inRange(def, value)) errors.push(`${key} is out of range`);
  }
}

/**
 * Validates a result metrics object against a paradigm's metric dictionary.
 * Every key must be a metric of the paradigm; values must match `valueType` and
 * fall within `validRange`. Templated (`object`) metrics hold a `{ zoneKey:
 * number }` map. Partial entry is allowed - not every metric need be present
 * (`required` is not enforced here), but unknown keys are rejected. This is the
 * single contract both manual entry and the CV service validate against.
 * @returns {{ valid: boolean, errors: string[] }}
 */
export function validateMetrics(paradigmKey, metrics) {
  const errors = [];
  if (metrics == null) return { valid: true, errors };
  if (typeof metrics !== "object" || Array.isArray(metrics)) {
    return { valid: false, errors: ["metrics must be an object"] };
  }
  const allowed = new Map(metricsForParadigm(paradigmKey).map((m) => [m.key, m]));
  for (const [key, value] of Object.entries(metrics)) {
    const def = allowed.get(key);
    if (!def) errors.push(`${key} is not a metric of ${paradigmKey}`);
    else checkMetricValue(def, key, value, errors);
  }
  return { valid: errors.length === 0, errors };
}

/**
 * Tells whether a result key is defined in the dictionary.
 * Templated keys (`zone_time_s.center`) are matched against the root key
 * (`zone_time_s`).
 */
export function isKnownMetricKey(key) {
  if (BY_KEY.has(key)) return true;
  const dot = key.indexOf(".");
  if (dot === -1) return false;
  const root = key.slice(0, dot);
  const def = BY_KEY.get(root);
  return Boolean(def && def.templated);
}
