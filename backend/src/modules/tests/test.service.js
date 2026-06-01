import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";

const include = {
  scenario: { select: { id: true, name: true, type: true } },
  subject: { select: { id: true, code: true, groupName: true } },
  operator: { select: { id: true, name: true } },
  device: { select: { id: true, name: true } },
};

export const list = () =>
  prisma.test.findMany({ orderBy: { createdAt: "desc" }, include });

export async function getById(id) {
  const row = await prisma.test.findUnique({ where: { id }, include });
  if (!row) throw ApiError.notFound();
  return row;
}

/**
 * Yeni test oluşturur. Operatör, oturum açan kullanıcıdır.
 */
export function create({ scenarioId, subjectId, deviceId, notes }, operatorId) {
  if (!scenarioId || !subjectId) {
    throw ApiError.badRequest("scenarioId ve subjectId zorunlu");
  }
  return prisma.test.create({
    data: {
      scenarioId,
      subjectId,
      deviceId: deviceId || null,
      notes: notes || null,
      operatorId,
    },
    include,
  });
}

/**
 * Durum/sonuç günceller (başlat, bitir, sonuç yaz).
 */
export function update(id, payload) {
  const { status, startedAt, endedAt, result, passed, notes } = payload;
  const data = {};
  if (status !== undefined) data.status = status;
  if (startedAt !== undefined) data.startedAt = startedAt ? new Date(startedAt) : null;
  if (endedAt !== undefined) data.endedAt = endedAt ? new Date(endedAt) : null;
  if (result !== undefined) data.result = typeof result === "string" ? result : JSON.stringify(result);
  if (passed !== undefined) data.passed = passed;
  if (notes !== undefined) data.notes = notes;
  return prisma.test.update({ where: { id }, data, include });
}

export async function remove(id) {
  await prisma.test.delete({ where: { id } });
}
