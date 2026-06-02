import jwt from "jsonwebtoken";
import { config } from "../config/index.js";
import { hasPermission } from "../config/permissions.js";
import { prisma } from "../lib/prisma.js";
import { ApiError } from "../utils/ApiError.js";
import { asyncHandler } from "../utils/asyncHandler.js";

/**
 * Bearer token doğrular ve req.user'ı doldurur.
 */
export const authenticate = asyncHandler(async (req, _res, next) => {
  const header = req.headers.authorization || "";
  const token = header.startsWith("Bearer ") ? header.slice(7) : null;
  if (!token) throw ApiError.unauthorized("Token gerekli");

  let payload;
  try {
    payload = jwt.verify(token, config.jwt.secret);
  } catch {
    throw ApiError.unauthorized("Geçersiz token");
  }

  const user = await prisma.user.findUnique({ where: { id: payload.sub } });
  if (!user) throw ApiError.unauthorized("Kullanıcı bulunamadı");

  req.user = { id: user.id, email: user.email, name: user.name, role: user.role };
  next();
});

/**
 * Belirli rolleri zorunlu kılan middleware (örn. requireRole("SUPERADMIN")).
 * Kaba kontroller için kalır; asıl yetki kapısı `requirePermission`'dır.
 */
export const requireRole = (...roles) =>
  (req, _res, next) => {
    if (!req.user || !roles.includes(req.user.role)) {
      throw ApiError.forbidden("Bu işlem için yetkiniz yok");
    }
    next();
  };

/**
 * Koda gömülü izin matrisine göre yetki kontrolü yapan middleware
 * (örn. requirePermission("subject:write")). Birincil yetki kapısı budur.
 */
export const requirePermission = (permission) =>
  (req, _res, next) => {
    if (!req.user || !hasPermission(req.user.role, permission)) {
      throw ApiError.forbidden("Bu işlem için yetkiniz yok");
    }
    next();
  };
