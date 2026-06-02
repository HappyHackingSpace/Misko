/**
 * Acceptance criteria engine (Step 2 - scientific contract).
 *
 * Acceptance criteria define a test's behavioral "pass/fail" line.
 * Design principles:
 *   - Optional: a test without criteria is not evaluated (passed = null).
 *   - Per-test: each test carries its own criteria list as JSON.
 *   - User-defined: the user picks among the paradigm's metrics and sets
 *     the operator and threshold value.
 *
 * Paradigm specs suggest an optional template via `suggestedAcceptance`;
 * the user can adopt and edit it or write one from scratch.
 *
 * Concrete user criterion shape:
 *   { metricKey, operator, value, label? }
 * For the `between` operator, value is a [min, max] array.
 */
import { isKnownMetricKey, getMetricDefinition, metricsForParadigm } from "./metrics.js";

/** Supported comparison operators. */
export const ACCEPTANCE_OPERATORS = Object.freeze([
  "<", "<=", ">", ">=", "==", "!=", "between",
]);

/** Whether an operator is a range (two-valued) operator. */
function isRangeOperator(operator) {
  return operator === "between";
}

/**
 * Applies a single comparison.
 * @param {number} actual    Measured value.
 * @param {string} operator  One of ACCEPTANCE_OPERATORS.
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
 * Resolves a key from the result metric object. Supports both flat dotted keys
 * (`zone_time_s.center`) and nested objects.
 * @returns {number|undefined}
 */
export function resolveMetricValue(metrics, metricKey) {
  if (metrics == null || typeof metrics !== "object") return undefined;
  if (Object.prototype.hasOwnProperty.call(metrics, metricKey)) {
    return metrics[metricKey];
  }
  // Dotted path: walk the nested object.
  const parts = metricKey.split(".");
  let cur = metrics;
  for (const part of parts) {
    if (cur == null || typeof cur !== "object") return undefined;
    cur = cur[part];
  }
  return cur;
}

/**
 * Validates a user-defined acceptance criteria list.
 * @param {Array} criteria
 * @param {{ paradigmKey?: string }} [opts] If paradigmKey is given, the metric
 *   key is restricted to that paradigm's metrics only; otherwise it is
 *   validated against the global metric dictionary.
 * @returns {{ valid: boolean, errors: string[] }}
 */
export function validateAcceptanceCriteria(criteria, opts = {}) {
  const errors = [];
  if (criteria == null) return { valid: true, errors };
  if (!Array.isArray(criteria)) {
    return { valid: false, errors: ["acceptanceCriteria must be an array"] };
  }

  let allowedKeys = null;
  if (opts.paradigmKey) {
    const defs = metricsForParadigm(opts.paradigmKey);
    if (defs.length === 0) {
      return { valid: false, errors: [`unknown paradigm: ${opts.paradigmKey}`] };
    }
    allowedKeys = new Set(defs.map((m) => m.key));
  }

  criteria.forEach((c, i) => {
    const at = `criterion[${i}]`;
    if (c == null || typeof c !== "object") {
      errors.push(`${at}: must be an object`);
      return;
    }
    const { metricKey, operator, value } = c;

    // metricKey
    if (typeof metricKey !== "string" || !metricKey) {
      errors.push(`${at}: metricKey is required`);
    } else if (allowedKeys) {
      const root = metricKey.includes(".") ? metricKey.slice(0, metricKey.indexOf(".")) : metricKey;
      if (!allowedKeys.has(metricKey) && !allowedKeys.has(root)) {
        errors.push(`${at}: ${metricKey} is not a metric of this paradigm`);
      }
    } else if (!isKnownMetricKey(metricKey)) {
      errors.push(`${at}: unknown metric key ${metricKey}`);
    }

    // operator
    if (!ACCEPTANCE_OPERATORS.includes(operator)) {
      errors.push(`${at}: invalid operator ${operator}`);
    }

    // value
    if (isRangeOperator(operator)) {
      if (!Array.isArray(value) || value.length !== 2 ||
          !Number.isFinite(value[0]) || !Number.isFinite(value[1])) {
        errors.push(`${at}: value for between must be [min, max]`);
      } else if (value[0] > value[1]) {
        errors.push(`${at}: for between min <= max is required`);
      }
    } else if (operator !== undefined && !Number.isFinite(value)) {
      errors.push(`${at}: value must be numeric`);
    }
  });

  return { valid: errors.length === 0, errors };
}

/**
 * Evaluates acceptance criteria against the measured metrics.
 * @param {Object} metrics    Result metric object.
 * @param {Array} criteria    User-defined criteria.
 * @returns {{ passed: boolean|null, results: Array }}
 *   If there are no criteria, passed = null (not evaluated). Otherwise true
 *   when all criteria are satisfied.
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
