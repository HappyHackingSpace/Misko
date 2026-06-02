/**
 * Kabul kriteri motoru (Step 2 - bilimsel kontrat).
 *
 * Kabul kriterleri bir testin davranissal "gecti/kaldi" cizgisini tanimlar.
 * Tasarim ilkeleri:
 *   - Opsiyonel: kriteri olmayan test degerlendirilmez (passed = null).
 *   - Test bazli: her test kendi kriter listesini JSON olarak tasir.
 *   - Kullanici tanimli: kullanici paradigmanin metrikleri arasindan secer,
 *     operator ve esik degeri belirler.
 *
 * Paradigma spec'leri `suggestedAcceptance` ile opsiyonel sablon onerir;
 * kullanici bunlari benimseyip duzenleyebilir veya sifirdan yazabilir.
 *
 * Kullanici kriteri (somut) sekli:
 *   { metricKey, operator, value, label? }
 * `between` operatoru icin value [min, max] dizisidir.
 */
import { isKnownMetricKey, getMetricDefinition, metricsForParadigm } from "./metrics.js";

/** Desteklenen karsilastirma operatorleri. */
export const ACCEPTANCE_OPERATORS = Object.freeze([
  "<", "<=", ">", ">=", "==", "!=", "between",
]);

/** Bir operatorun aralik (iki degerli) operatoru olup olmadigi. */
function isRangeOperator(operator) {
  return operator === "between";
}

/**
 * Tek bir karsilastirmayi uygular.
 * @param {number} actual    Olculen deger.
 * @param {string} operator  ACCEPTANCE_OPERATORS icinden.
 * @param {number|[number, number]} value
 * @returns {boolean}
 */
export function compare(actual, operator, value) {
  switch (operator) {
    case "<": return actual < value;
    case "<=": return actual <= value;
    case ">": return actual > value;
    case ">=": return actual >= value;
    case "==": return actual === value;
    case "!=": return actual !== value;
    case "between": {
      const [min, max] = value;
      return actual >= min && actual <= max;
    }
    default: return false;
  }
}

/**
 * Sonuc metrik nesnesinden bir anahtari cozer. Hem duz noktali anahtarlari
 * (`zone_time_s.center`) hem de ic ice nesneleri destekler.
 * @returns {number|undefined}
 */
export function resolveMetricValue(metrics, metricKey) {
  if (metrics == null || typeof metrics !== "object") return undefined;
  if (Object.prototype.hasOwnProperty.call(metrics, metricKey)) {
    return metrics[metricKey];
  }
  // Noktali yol: ic ice nesnede gez.
  const parts = metricKey.split(".");
  let cur = metrics;
  for (const part of parts) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = cur[part];
  }
  return cur;
}

/**
 * Kullanici tanimli kabul kriter listesini dogrular.
 * @param {Array} criteria
 * @param {{ paradigmKey?: string }} [opts] paradigmKey verilirse metrik
 *   anahtari yalnizca o paradigmanin metrikleriyle sinirlanir; aksi halde
 *   global metrik sozlugune gore dogrulanir.
 * @returns {{ valid: boolean, errors: string[] }}
 */
export function validateAcceptanceCriteria(criteria, opts = {}) {
  const errors = [];
  if (criteria == null) return { valid: true, errors };
  if (!Array.isArray(criteria)) {
    return { valid: false, errors: ["acceptanceCriteria bir dizi olmali"] };
  }

  let allowedKeys = null;
  if (opts.paradigmKey) {
    const defs = metricsForParadigm(opts.paradigmKey);
    if (defs.length === 0) {
      return { valid: false, errors: [`bilinmeyen paradigma: ${opts.paradigmKey}`] };
    }
    allowedKeys = new Set(defs.map((m) => m.key));
  }

  criteria.forEach((c, i) => {
    const at = `kriter[${i}]`;
    if (c == null || typeof c !== "object") {
      errors.push(`${at}: nesne olmali`);
      return;
    }
    const { metricKey, operator, value } = c;

    // metricKey
    if (typeof metricKey !== "string" || !metricKey) {
      errors.push(`${at}: metricKey zorunlu`);
    } else if (allowedKeys) {
      const root = metricKey.includes(".") ? metricKey.slice(0, metricKey.indexOf(".")) : metricKey;
      if (!allowedKeys.has(metricKey) && !allowedKeys.has(root)) {
        errors.push(`${at}: ${metricKey} bu paradigmanin metrigi degil`);
      }
    } else if (!isKnownMetricKey(metricKey)) {
      errors.push(`${at}: bilinmeyen metrik anahtari ${metricKey}`);
    }

    // operator
    if (!ACCEPTANCE_OPERATORS.includes(operator)) {
      errors.push(`${at}: gecersiz operator ${operator}`);
    }

    // value
    if (isRangeOperator(operator)) {
      if (!Array.isArray(value) || value.length !== 2 ||
          typeof value[0] !== "number" || typeof value[1] !== "number") {
        errors.push(`${at}: between icin value [min, max] olmali`);
      } else if (value[0] > value[1]) {
        errors.push(`${at}: between icin min <= max olmali`);
      }
    } else if (operator !== undefined && typeof value !== "number") {
      errors.push(`${at}: value sayisal olmali`);
    }
  });

  return { valid: errors.length === 0, errors };
}

/**
 * Olculen metriklere gore kabul kriterlerini degerlendirir.
 * @param {Object} metrics    Sonuc metrik nesnesi.
 * @param {Array} criteria    Kullanici tanimli kriterler.
 * @returns {{ passed: boolean|null, results: Array }}
 *   Kriteri yoksa passed = null (degerlendirilmedi). Aksi halde tum
 *   kriterler saglaniyorsa true.
 */
export function evaluateAcceptance(metrics, criteria) {
  if (!Array.isArray(criteria) || criteria.length === 0) {
    return { passed: null, results: [] };
  }

  const results = criteria.map((c) => {
    const { metricKey, operator, value } = c;
    const actual = resolveMetricValue(metrics, metricKey);
    const def = getMetricDefinition(
      metricKey.includes(".") ? metricKey.slice(0, metricKey.indexOf(".")) : metricKey,
    );
    if (typeof actual !== "number") {
      return {
        metricKey, operator, value, actual: actual ?? null,
        unit: def ? def.unit : null,
        passed: false, reason: "metric_missing",
      };
    }
    return {
      metricKey, operator, value, actual,
      unit: def ? def.unit : null,
      passed: compare(actual, operator, value), reason: null,
    };
  });

  const passed = results.every((r) => r.passed);
  return { passed, results };
}
