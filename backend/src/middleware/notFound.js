import { ApiError } from "../utils/ApiError.js";

/**
 * Produces a 404 for unmatched routes.
 */
export function notFound(req, _res, next) {
  const safeUrl = encodeURI(req.originalUrl);
  next(
    ApiError.notFound(`Route not found: ${req.method} ${safeUrl}`, "common.routeNotFound", {
      method: req.method,
      url: safeUrl,
    }),
  );
}
