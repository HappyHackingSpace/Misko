import { z } from "zod";

export const createScenarioSchema = z.object({
  name: z.string({ required_error: "Name is required" }).min(1),
  description: z.string().optional().nullable(),
  sessionParams: z.record(z.any()).optional(),
  environmentIds: z.array(z.string()).optional(),
}).strict();

export const updateScenarioSchema = z.object({
  name: z.string().min(1).optional(),
  description: z.string().optional().nullable(),
  sessionParams: z.record(z.any()).optional().nullable(),
  environmentIds: z.array(z.string()).optional(),
}).strict();
