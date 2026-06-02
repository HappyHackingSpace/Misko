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
    // Tekrarli/dizi query (?paradigm=a&paradigm=b) string degildir; includes(...)
    // sessizce bos kume dondurmesin diye reddet.
    if (paradigm !== undefined && typeof paradigm !== "string") {
      throw ApiError.badRequest("paradigm tek bir metin degeri olmali");
    }
    const lang = normalizeLang(req.query.lang);
    const list = paradigm ? metricsForParadigm(paradigm) : METRIC_DEFINITIONS;
    res.json(localizeMetricList(list, lang));
  }),
);

// Kabul kriteri operatorleri (kriter olusturucu icin)
paradigmRouter.get(
  "/acceptance-operators",
  asyncHandler(async (_req, res) => {
    res.json(ACCEPTANCE_OPERATORS);
  }),
);

// Paradigma ozet listesi
paradigmRouter.get(
  "/",
  asyncHandler(async (req, res) => {
    const lang = normalizeLang(req.query.lang);
    res.json(listParadigmSummaries().map((s) => localizeSummary(s, lang)));
  }),
);

// Tek paradigma detay (operasyonel kontrat)
paradigmRouter.get(
  "/:key",
  asyncHandler(async (req, res) => {
    const spec = getParadigmSpec(req.params.key);
    if (!spec) throw ApiError.notFound("Paradigma bulunamadi");
    const lang = normalizeLang(req.query.lang);
    const { zones, ...rest } = spec;
    const resolved = {
      ...rest,
      zones: typeof zones === "function" ? zones({}) : zones,
    };
    res.json(localizeDetail(resolved, lang));
  }),
);
