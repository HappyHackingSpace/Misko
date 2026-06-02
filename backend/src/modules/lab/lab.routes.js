import { Router } from "express";
import { asyncHandler } from "../../utils/asyncHandler.js";
import { ApiError } from "../../utils/ApiError.js";
import { requirePermission } from "../../middleware/authenticate.js";
import { PERMISSIONS } from "../../config/permissions.js";
import { getLaboratory, updateLaboratory } from "./lab.service.js";

/**
 * Laboratory singleton management.
 *
 * Reads are open to all roles (`*:read`, with `authenticate` at the top layer).
 * Writes are protected by the `lab:configure` permission (SUPERADMIN/LAB_MANAGER).
 */
export const labRouter = Router();

// Laboratory info
labRouter.get(
  "/",
  asyncHandler(async (_req, res) => {
    res.json(await getLaboratory());
  }),
);

// Update laboratory info (branding/settings)
labRouter.patch(
  "/",
  requirePermission(PERMISSIONS.LAB_CONFIGURE),
  asyncHandler(async (req, res) => {
    const body = req.body ?? {};
    if (body.name !== undefined && (typeof body.name !== "string" || body.name.trim() === "")) {
      throw ApiError.badRequest("name must be a non-empty string");
    }
    if (body.code !== undefined && body.code !== null && typeof body.code !== "string") {
      throw ApiError.badRequest("code must be a string or null");
    }
    if (body.timezone !== undefined && typeof body.timezone !== "string") {
      throw ApiError.badRequest("timezone must be a string");
    }
    if (body.settings !== undefined && (typeof body.settings !== "object" || body.settings === null || Array.isArray(body.settings))) {
      throw ApiError.badRequest("settings must be an object");
    }
    res.json(await updateLaboratory(body));
  }),
);
