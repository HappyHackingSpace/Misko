import { ApiError } from "../utils/ApiError.js";

/**
 * Eşleşmeyen rotalar için 404 üretir.
 */
export function notFound(req, _res, next) {
  next(ApiError.notFound(`Rota bulunamadı: ${req.method} ${req.originalUrl}`));
}
