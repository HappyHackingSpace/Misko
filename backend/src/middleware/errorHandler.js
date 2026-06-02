import { Prisma } from "@prisma/client";
import { ZodError } from "zod";
import { config } from "../config/index.js";
import { ApiError } from "../utils/ApiError.js";

/**
 * Central error middleware. All errors are converted to a JSON response in one place:
 *   { error: string, code?: string, details?: any }
 * `code` is a stable identifier the client maps to a localized message; `error`
 * is the English fallback used when the client has no translation for `code`.
 */
export function errorHandler(err, _req, res, _next) {
  if (err instanceof ApiError) {
    return res.status(err.statusCode).json({ error: err.message, code: err.code, details: err.details });
  }

  if (err instanceof ZodError) {
    return res.status(400).json({
      error: "Validation error",
      code: "common.validation",
      details: err.issues.map((i) => ({ path: i.path.join("."), message: i.message })),
    });
  }

  if (err instanceof Prisma.PrismaClientKnownRequestError) {
    if (err.code === "P2002") {
      return res.status(409).json({ error: "Unique field conflict", code: "common.conflict", details: err.meta?.target });
    }
    if (err.code === "P2025") {
      return res.status(404).json({ error: "Record not found", code: "common.notFound" });
    }
    return res.status(400).json({ error: "Database error", code: "common.dbError", details: err.code });
  }

  if (!config.isProd) {
    console.error(err);
  }
  return res.status(500).json({ error: "Server error", code: "common.serverError" });
}
