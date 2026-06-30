import { z } from "zod";

export const createEnvironmentSchema = z.object({
  name: z.string({ required_error: "Name is required" }).min(1),
  paradigmKey: z.string({ required_error: "paradigmKey is required" }),
  apparatus: z.record(z.any()).optional().nullable(),
  notes: z.string().optional().nullable(),
}).strict();

export const updateEnvironmentSchema = z.object({
  name: z.string().min(1).optional(),
  paradigmKey: z.string().optional(),
  apparatus: z.record(z.any()).optional().nullable(),
  notes: z.string().optional().nullable(),
}).strict();
