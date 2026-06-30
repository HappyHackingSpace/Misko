import { z } from "zod";

export const createTestSchema = z.object({
  scenarioId: z.string({ required_error: "scenarioId is required" }),
  subjectId: z.string({ required_error: "subjectId is required" }),
  notes: z.string().optional().nullable(),
}).strict();

export const updateTestSchema = z.object({
  status: z.enum(["PENDING", "RUNNING", "DONE", "FAILED"]).optional(),
  notes: z.string().optional().nullable(),
}).strict();

export const submitEnvironmentResultSchema = z.object({
  status: z.enum(["RUNNING", "DONE"]).optional(),
}).strict();

export const addEnvironmentEventSchema = z.object({
  type: z.string({ required_error: "Event type is required" }),
  t: z.number().optional(),
  payload: z.record(z.any()).optional(),
}).strict();
