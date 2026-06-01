import { Router } from "express";
import { asyncHandler } from "../utils/asyncHandler.js";

/**
 * Bir CRUD servisini standart REST rotalarına bağlar.
 * @param {ReturnType<import("./crudService.js").createCrudService>} service
 */
export function createCrudRouter(service) {
  const router = Router();

  router.get("/", asyncHandler(async (_req, res) => {
    res.json(await service.list());
  }));

  router.get("/:id", asyncHandler(async (req, res) => {
    res.json(await service.getById(req.params.id));
  }));

  router.post("/", asyncHandler(async (req, res) => {
    res.status(201).json(await service.create(req.body));
  }));

  router.patch("/:id", asyncHandler(async (req, res) => {
    res.json(await service.update(req.params.id, req.body));
  }));

  router.delete("/:id", asyncHandler(async (req, res) => {
    await service.remove(req.params.id);
    res.status(204).end();
  }));

  return router;
}
