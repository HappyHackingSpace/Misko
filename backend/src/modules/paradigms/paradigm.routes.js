import { Router } from "express";
import { asyncHandler } from "../../utils/asyncHandler.js";
import { ApiError } from "../../utils/ApiError.js";
import {
  listParadigmSummaries,
  getParadigmSpec,
} from "../../config/paradigms.js";
import {
  METRIC_DEFINITIONS,
  metricsForParadigm,
} from "../../config/metrics.js";
import { UNIT_LIST } from "../../config/units.js";
import { ACCEPTANCE_OPERATORS } from "../../config/acceptance.js";
import {
  normalizeLang,
  localizeSummary,
  localizeDetail,
  localizeMetricList,
} from "../../config/i18n.js";

/**
 * Read-only endpoints for the paradigm and measurement dictionary (Step 2).
 *
 * Paradigms are code-owned templates. Researchers review paradigm detail pages
 * and metric definitions before creating an environment or running a test. Since
 * all roles have the `*:read` permission, no extra permission guard is needed;
 * `authenticate` is applied at the top layer.
 */
export const paradigmRouter = Router();

// Canonical unit list
paradigmRouter.get(
  "/units",
  asyncHandler(async (_req, res) => {
    res.json(UNIT_LIST);
  }),
);

// Full measurement dictionary (optional ?paradigm=MWM filter)
paradigmRouter.get(
  "/metrics",
  asyncHandler(async (req, res) => {
    const { paradigm } = req.query;
    // A repeated/array query (?paradigm=a&paradigm=b) is not a string; reject it
    // so includes(...) does not silently return an empty set.
    if (paradigm !== undefined && typeof paradigm !== "string") {
      throw ApiError.badRequest("paradigm must be a single string value");
    }
    const lang = normalizeLang(req.query.lang);
    const list = paradigm ? metricsForParadigm(paradigm) : METRIC_DEFINITIONS;
    res.json(localizeMetricList(list, lang));
  }),
);

// Acceptance criteria operators (for the criteria builder)
paradigmRouter.get(
  "/acceptance-operators",
  asyncHandler(async (_req, res) => {
    res.json(ACCEPTANCE_OPERATORS);
  }),
);

// Paradigm summary list (read-only template catalog)
paradigmRouter.get(
  "/",
  asyncHandler(async (req, res) => {
    const lang = normalizeLang(req.query.lang);
    res.json(listParadigmSummaries().map((s) => localizeSummary(s, lang)));
  }),
);

// Single paradigm detail (operational contract)
paradigmRouter.get(
  "/:key",
  asyncHandler(async (req, res) => {
    const spec = getParadigmSpec(req.params.key);
    if (!spec) throw ApiError.notFound("Paradigm not found");
    const lang = normalizeLang(req.query.lang);
    const { zones, ...rest } = spec;
    const resolved = {
      ...rest,
      zones: typeof zones === "function" ? zones({}) : zones,
    };
    res.json(localizeDetail(resolved, lang));
  }),
);
