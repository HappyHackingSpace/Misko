import { Prisma } from "@prisma/client";
import { ZodError } from "zod";
import { config } from "../config/index.js";
import { ApiError } from "../utils/ApiError.js";

/**
 * Merkezi hata middleware'i. Tüm hatalar tek bir yerde JSON yanıta dönüşür:
 *   { error: string, details?: any }
 */
export function errorHandler(err, _req, res, _next) {
  if (err instanceof ApiError) {
    return res.status(err.statusCode).json({ error: err.message, details: err.details });
  }

  if (err instanceof ZodError) {
    return res.status(400).json({
      error: "Doğrulama hatası",
      details: err.issues.map((i) => ({ path: i.path.join("."), message: i.message })),
    });
  }

  if (err instanceof Prisma.PrismaClientKnownRequestError) {
    if (err.code === "P2002") {
      return res.status(409).json({ error: "Benzersiz alan çakışması", details: err.meta?.target });
    }
    if (err.code === "P2025") {
      return res.status(404).json({ error: "Kayıt bulunamadı" });
    }
    return res.status(400).json({ error: "Veritabanı hatası", details: err.code });
  }

  if (!config.isProd) {
    console.error(err);
  }
  return res.status(500).json({ error: "Sunucu hatası" });
}
