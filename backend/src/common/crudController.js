import { Router } from "express";
import { requirePermission } from "../middleware/authenticate.js";
import { asyncHandler } from "../utils/asyncHandler.js";

/**
 * Binds a CRUD service to standard REST routes.
 *
 * Read (GET) routes only require authentication (the router is wrapped with
 * `authenticate` at the top level). Write routes (POST/PATCH/DELETE) can be
 * protected with an optional permission.
 *
 * @param {ReturnType<import("./crudService.js").createCrudService>} service
 * @param {{ writePermission?: string }} [options] - permission required for write routes
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
