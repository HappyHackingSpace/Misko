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

/**
 * Paradigma ve olcum sozlugu salt-okunur uclari (Step 2).
 *
 * Arastirmacilar bir test calistirmadan once paradigma detay sayfalarini ve
 * metrik tanimlarini inceleyebilir. Tum roller `*:read` iznine sahip oldugu
 * icin ek izin gardi gerekmez; ust katmanda `authenticate` uygulanir.
 */
export const paradigmRouter = Router();

// Kanonik birim listesi
paradigmRouter.get(
  "/units",
  asyncHandler(async (_req, res) => {
    res.json(UNIT_LIST);
  }),
);

// Tum olcum sozlugu (opsiyonel ?paradigm=MWM filtresi)
paradigmRouter.get(
  "/metrics",
  asyncHandler(async (req, res) => {
    const { paradigm } = req.query;
    res.json(paradigm ? metricsForParadigm(paradigm) : METRIC_DEFINITIONS);
  }),
);

// Paradigma ozet listesi
paradigmRouter.get(
  "/",
  asyncHandler(async (_req, res) => {
    res.json(listParadigmSummaries());
  }),
);

// Tek paradigma detay (operasyonel kontrat)
paradigmRouter.get(
  "/:key",
  asyncHandler(async (req, res) => {
    const spec = getParadigmSpec(req.params.key);
    if (!spec) throw ApiError.notFound("Paradigma bulunamadi");
    const { zones, ...rest } = spec;
    res.json({ ...rest, zones: typeof zones === "function" ? zones({}) : zones });
  }),
);
