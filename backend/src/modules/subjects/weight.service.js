import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";

/**
 * WeightLog service. Weight logs are a time series nested under a subject
 * (see docs/DOMAIN.md §2). Canonical unit is grams (backend/src/config/units.js).
 */

async function ensureSubject(subjectId) {
  const subject = await prisma.subject.findUnique({ where: { id: subjectId } });
  if (!subject) throw ApiError.notFound("Subject not found", "subject.notFound");
  return subject;
}

/** Lists a subject's weight logs, oldest first (so charts read left to right). */
export async function listWeights(subjectId) {
  await ensureSubject(subjectId);
  return prisma.weightLog.findMany({
    where: { subjectId },
    orderBy: { measuredAt: "asc" },
  });
}

/** Adds a weight log entry for a subject. `input` is already Zod-validated. */
export async function addWeight(subjectId, input) {
  await ensureSubject(subjectId);
  return prisma.weightLog.create({
    data: {
      subjectId,
      grams: input.grams,
      measuredAt: input.measuredAt ?? undefined,
      notes: input.notes ?? null,
    },
  });
}

/** Deletes a weight log, ensuring it belongs to the given subject. */
export async function deleteWeight(subjectId, weightId) {
  const log = await prisma.weightLog.findUnique({ where: { id: weightId } });
  if (!log || log.subjectId !== subjectId) {
    throw ApiError.notFound("Weight log not found", "weight.notFound");
  }
  await prisma.weightLog.delete({ where: { id: weightId } });
}
