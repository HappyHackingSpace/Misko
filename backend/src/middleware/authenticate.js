import jwt from "jsonwebtoken";
import { config } from "../config/index.js";
import { hasPermission } from "../config/permissions.js";
import { prisma } from "../lib/prisma.js";
import { ApiError } from "../utils/ApiError.js";
import { asyncHandler } from "../utils/asyncHandler.js";

/**
 * Verifies the Bearer token and populates req.user.
 */
export const authenticate = asyncHandler(async (req, _res, next) => {
  const header = req.headers.authorization || "";
  const token = header.startsWith("Bearer ") ? header.slice(7) : null;
  if (!token) throw ApiError.unauthorized("Token required", "auth.tokenRequired");

  let payload;
  try {
    payload = jwt.verify(token, config.jwt.secret);
  } catch {
    throw ApiError.unauthorized("Invalid token", "auth.invalidToken");
  }

  const user = await prisma.user.findUnique({ where: { id: payload.sub } });
  if (!user) throw ApiError.unauthorized("User not found", "auth.userNotFound");

  req.user = { id: user.id, email: user.email, name: user.name, role: user.role };
  next();
});

/**
 * Middleware that enforces specific roles (e.g. requireRole("SUPERADMIN")).
 * Kept for coarse checks; the real authorization gate is `requirePermission`.
 */
export const requireRole = (...roles) =>
  (req, _res, next) => {
    if (!req.user || !roles.includes(req.user.role)) {
      throw ApiError.forbidden("You do not have permission for this action", "auth.forbidden");
    }
    next();
  };

/**
 * Middleware that checks authorization against the in-code permission matrix
 * (e.g. requirePermission("subject:write")). This is the primary authorization gate.
 */
export const requirePermission = (permission) =>
  (req, _res, next) => {
    if (!req.user || !hasPermission(req.user.role, permission)) {
      throw ApiError.forbidden("You do not have permission for this action", "auth.forbidden");
    }
    next();
  };
