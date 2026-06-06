import { Router } from "express";
import { asyncHandler } from "../../utils/asyncHandler.js";
import { requirePermission } from "../../middleware/authenticate.js";
import { PERMISSIONS } from "../../config/permissions.js";
import {
  listScenarios,
  getScenario,
  createScenario,
  updateScenario,
  deleteScenario,
} from "./scenario.service.js";

/**
 * Scenario CRUD - the central experiment definition (docs/DOMAIN.md §3).
 * Reads need only auth; writes need `apparatus:write` (RESEARCHER and above).
 */
export const scenarioRouter = Router();

scenarioRouter.get("/", asyncHandler(async (req, res) => {
  res.json(await listScenarios(req.query));
}));

scenarioRouter.get("/:id", asyncHandler(async (req, res) => {
  res.json(await getScenario(req.params.id));
}));

scenarioRouter.post("/", requirePermission(PERMISSIONS.APPARATUS_WRITE), asyncHandler(async (req, res) => {
  res.status(201).json(await createScenario(req.body ?? {}));
}));

scenarioRouter.patch("/:id", requirePermission(PERMISSIONS.APPARATUS_WRITE), asyncHandler(async (req, res) => {
  res.json(await updateScenario(req.params.id, req.body ?? {}));
}));

scenarioRouter.delete("/:id", requirePermission(PERMISSIONS.APPARATUS_WRITE), asyncHandler(async (req, res) => {
  await deleteScenario(req.params.id);
  res.status(204).end();
}));
