import { z } from "zod";
import { ROLE_LIST } from "../../config/permissions.js";

const ROLES = ROLE_LIST;

export const createUserSchema = z.object({
  name: z.string().min(1, "Ad zorunlu"),
  email: z.string().email("Geçerli bir e-posta girin"),
  role: z.enum(ROLES).default("RESEARCHER"),
  // Şifre opsiyonel: verilmezse sistem güçlü bir şifre üretip bir kez döner.
  password: z.string().min(8, "Şifre en az 8 karakter olmalı").optional(),
});

export const updateUserSchema = z
  .object({
    name: z.string().min(1).optional(),
    role: z.enum(ROLES).optional(),
  })
  .refine((d) => Object.keys(d).length > 0, "Güncellenecek alan yok");

export const resetPasswordSchema = z.object({
  // Verilmezse sistem üretir.
  password: z.string().min(8, "Şifre en az 8 karakter olmalı").optional(),
});
