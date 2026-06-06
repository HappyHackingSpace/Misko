import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";

/**
 * Subject <-> DiseaseModel and Subject <-> Treatment join management
 * (docs/DOMAIN.md §3). A subject can carry many disease models and many
 * treatments at once; each link stores per-subject metadata.
 */

async function ensureSubject(subjectId) {
  const subject = await prisma.subject.findUnique({ where: { id: subjectId } });
  if (!subject) throw ApiError.notFound("Subject not found", "subject.notFound");
  return subject;
}

// --- Disease models ---

export async function listSubjectDiseaseModels(subjectId) {
  await ensureSubject(subjectId);
  return prisma.subjectDiseaseModel.findMany({
    where: { subjectId },
    include: { diseaseModel: true },
    orderBy: { createdAt: "asc" },
  });
}

export async function attachDiseaseModel(subjectId, input) {
  await ensureSubject(subjectId);
  const model = await prisma.diseaseModel.findUnique({ where: { id: input.diseaseModelId } });
  if (!model) throw ApiError.badRequest("Unknown disease model", "diseaseModel.notFound");
  return prisma.subjectDiseaseModel.create({
    data: {
      subjectId,
      diseaseModelId: input.diseaseModelId,
      inducedAt: input.inducedAt ?? null,
      method: input.method ?? null,
      notes: input.notes ?? null,
    },
    include: { diseaseModel: true },
  });
}

export async function detachDiseaseModel(subjectId, linkId) {
  const link = await prisma.subjectDiseaseModel.findUnique({ where: { id: linkId } });
  if (!link || link.subjectId !== subjectId) {
    throw ApiError.notFound("Disease model link not found", "diseaseModel.linkNotFound");
  }
  await prisma.subjectDiseaseModel.delete({ where: { id: linkId } });
}

// --- Treatments ---

export async function listSubjectTreatments(subjectId) {
  await ensureSubject(subjectId);
  return prisma.subjectTreatment.findMany({
    where: { subjectId },
    include: { treatment: true },
    orderBy: { createdAt: "asc" },
  });
}

export async function attachTreatment(subjectId, input) {
  await ensureSubject(subjectId);
  const treatment = await prisma.treatment.findUnique({ where: { id: input.treatmentId } });
  if (!treatment) throw ApiError.badRequest("Unknown treatment", "treatment.notFound");
  return prisma.subjectTreatment.create({
    data: {
      subjectId,
      treatmentId: input.treatmentId,
      dose: input.dose ?? null,
      unit: input.unit ?? null,
      route: input.route ?? null,
      schedule: input.schedule ?? undefined,
      startedAt: input.startedAt ?? null,
      endedAt: input.endedAt ?? null,
    },
    include: { treatment: true },
  });
}

export async function detachTreatment(subjectId, linkId) {
  const link = await prisma.subjectTreatment.findUnique({ where: { id: linkId } });
  if (!link || link.subjectId !== subjectId) {
    throw ApiError.notFound("Treatment link not found", "treatment.linkNotFound");
  }
  await prisma.subjectTreatment.delete({ where: { id: linkId } });
}
