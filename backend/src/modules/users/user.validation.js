import { z } from "zod";
import { ROLE_LIST } from "../../config/permissions.js";

const ROLES = ROLE_LIST;

export const createUserSchema = z.object({
  name: z.string().min(1, "Name is required"),
  email: z.string().email("Enter a valid email"),
  role: z.enum(ROLES).default("RESEARCHER"),
  // Password optional: if not given, the system generates a strong one and returns it once.
  password: z.string().min(8, "Password must be at least 8 characters").optional(),
});

export const updateUserSchema = z
  .object({
    name: z.string().min(1).optional(),
    role: z.enum(ROLES).optional(),
  })
  .refine((d) => Object.keys(d).length > 0, "No field to update");

export const resetPasswordSchema = z.object({
  // If not given, the system generates one.
  password: z.string().min(8, "Password must be at least 8 characters").optional(),
});
