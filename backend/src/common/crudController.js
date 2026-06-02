import { Router } from "express";
import { requirePermission } from "../middleware/authenticate.js";
import { asyncHandler } from "../utils/asyncHandler.js";

/**
 * Bir CRUD servisini standart REST rotalarına bağlar.
 *
 * Okuma (GET) rotaları yalnızca kimlik doğrulaması ister (router üst seviyede
 * `authenticate` ile sarılır). Yazma rotaları (POST/PATCH/DELETE) opsiyonel bir
 * izinle korunabilir.
 *
 * @param {ReturnType<import("./crudService.js").createCrudService>} service
 * @param {{ writePermission?: string }} [options] - yazma rotaları için gerekli izin
 */
export function createCrudRouter(service, options = {}) {
  const router = Router();
  const { writePermission } = options;
  const guard = writePermission ? [requirePermission(writePermission)] : [];

  router.get("/", asyncHandler(async (_req, res) => {
    res.json(await service.list());
  }));

  router.get("/:id", asyncHandler(async (req, res) => {
    res.json(await service.getById(req.params.id));
  }));

  router.post("/", ...guard, asyncHandler(async (req, res) => {
    res.status(201).json(await service.create(req.body));
  }));

  router.patch("/:id", ...guard, asyncHandler(async (req, res) => {
    res.json(await service.update(req.params.id, req.body));
  }));

  router.delete("/:id", ...guard, asyncHandler(async (req, res) => {
    await service.remove(req.params.id);
    res.status(204).end();
  }));

  return router;
}
