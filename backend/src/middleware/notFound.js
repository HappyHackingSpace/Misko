import { ApiError } from "../utils/ApiError.js";

/**
 * Produces a 404 for unmatched routes.
 */
export function notFound(req, _res, next) {
  next(ApiError.notFound(`Route not found: ${req.method} ${req.originalUrl}`));
}
