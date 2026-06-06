import { Router } from "express";
import { requirePermission } from "../middleware/authenticate.js";
import { validateBody } from "../middleware/validate.js";
import { asyncHandler } from "../utils/asyncHandler.js";

/**
 * Binds a CRUD service to standard REST routes.
 *
 * Read (GET) routes only require authentication (the router is wrapped with
 * `authenticate` at the top level). Write routes (POST/PATCH/DELETE) can be
 * protected with an optional permission and validated against optional Zod schemas.
 *
 * @param {ReturnType<import("./crudService.js").createCrudService>} service
 * @param {object} [options]
 * @param {string} [options.writePermission] - permission required for write routes
 * @param {import("zod").ZodType} [options.createSchema] - validates POST bodies
 * @param {import("zod").ZodType} [options.updateSchema] - validates PATCH bodies
 */
export function createCrudRouter(service, options = {}) {
  const router = Router();
  const { writePermission, createSchema, updateSchema } = options;
  const guard = writePermission ? [requirePermission(writePermission)] : [];
  const validateCreate = createSchema ? [validateBody(createSchema)] : [];
  const validateUpdate = updateSchema ? [validateBody(updateSchema)] : [];

  router.get("/", asyncHandler(async (req, res) => {
    res.json(await service.list(req.query));
  }));

  router.get("/:id", asyncHandler(async (req, res) => {
    res.json(await service.getById(req.params.id));
  }));

  router.post("/", ...guard, ...validateCreate, asyncHandler(async (req, res) => {
    res.status(201).json(await service.create(req.body));
  }));

  router.patch("/:id", ...guard, ...validateUpdate, asyncHandler(async (req, res) => {
    res.json(await service.update(req.params.id, req.body));
  }));

  router.delete("/:id", ...guard, asyncHandler(async (req, res) => {
    await service.remove(req.params.id);
    res.status(204).end();
  }));

  return router;
}
