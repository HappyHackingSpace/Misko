import { Router } from "express";
import { asyncHandler } from "../../utils/asyncHandler.js";
import { requirePermission } from "../../middleware/authenticate.js";
import { validateBody } from "../../middleware/validate.js";
import { PERMISSIONS } from "../../config/permissions.js";
import {
  listEnvironments,
  getEnvironment,
  createEnvironment,
  updateEnvironment,
  deleteEnvironment,
} from "./environment.service.js";
import { createEnvironmentSchema, updateEnvironmentSchema } from "./environment.validation.js";

/**
 * Environment ("Ortam") CRUD.
 *
 * Reads are open to all roles (`*:read`, with `authenticate` at the top layer).
 * Writes require `apparatus:write` (RESEARCHER and above). An environment is a
 * named instance of a paradigm template with locked physical values.
 */
export const environmentRouter = Router();

environmentRouter.get(
  "/",
  asyncHandler(async (req, res) => {
    res.json(await listEnvironments(req.query));
  }),
);

environmentRouter.get(
  "/:id",
  asyncHandler(async (req, res) => {
    res.json(await getEnvironment(req.params.id));
  }),
);

environmentRouter.post(
  "/",
  requirePermission(PERMISSIONS.APPARATUS_WRITE),
  validateBody(createEnvironmentSchema),
  asyncHandler(async (req, res) => {
    res.status(201).json(await createEnvironment(req.body ?? {}));
  }),
);

environmentRouter.patch(
  "/:id",
  requirePermission(PERMISSIONS.APPARATUS_WRITE),
  validateBody(updateEnvironmentSchema),
  asyncHandler(async (req, res) => {
    res.json(await updateEnvironment(req.params.id, req.body ?? {}));
  }),
);

environmentRouter.delete(
  "/:id",
  requirePermission(PERMISSIONS.APPARATUS_WRITE),
  asyncHandler(async (req, res) => {
    await deleteEnvironment(req.params.id);
    res.status(204).end();
  }),
);
