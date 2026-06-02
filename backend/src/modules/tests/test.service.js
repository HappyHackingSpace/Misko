import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { validateAcceptanceCriteria, evaluateAcceptance } from "../../config/acceptance.js";

/** Converts a result/criteria field into a JSON object (accepts a string or object). */
function asObject(value) {
  if (value == null) return null;
  if (typeof value === "string") {
    try {
      return JSON.parse(value);
    } catch {
      return null;
    }
  }
  return typeof value === "object" ? value : null;
}

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
 * Creates a new test. The operator is the logged-in user.
 */
export function create({ scenarioId, subjectId, deviceId, notes }, operatorId) {
  if (!scenarioId || !subjectId) {
    throw ApiError.badRequest("scenarioId and subjectId are required");
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
 * Updates status/result (start, finish, write result).
 *
 * Acceptance criteria (acceptanceCriteria) are optional and per-test.
 * - If criteria are given, they are validated and stored as JSON.
 * - When a result is written and `passed` is not given explicitly, it is
 *   evaluated automatically against the stored/new criteria. With no criteria, passed = null.
 */
export async function update(id, payload) {
  const { status, startedAt, endedAt, result, passed, notes, acceptanceCriteria } = payload;
  const data = {};
  if (status !== undefined) data.status = status;
  if (startedAt !== undefined) data.startedAt = startedAt ? new Date(startedAt) : null;
  if (endedAt !== undefined) data.endedAt = endedAt ? new Date(endedAt) : null;
  if (result !== undefined) data.result = typeof result === "string" ? result : JSON.stringify(result);
  if (notes !== undefined) data.notes = notes;

  // Acceptance criteria: validate and store.
  let criteria;
  if (acceptanceCriteria !== undefined) {
    criteria = acceptanceCriteria == null ? null : asObject(acceptanceCriteria) ?? acceptanceCriteria;
    const { valid, errors } = validateAcceptanceCriteria(criteria);
    if (!valid) throw ApiError.badRequest(`Invalid acceptance criteria: ${errors.join("; ")}`);
    data.acceptanceCriteria = criteria == null ? null : JSON.stringify(criteria);
  }

  // Auto-evaluate passed: when a result was written and passed was not given explicitly.
  if (passed !== undefined) {
    data.passed = passed; // manual override
  } else if (result !== undefined) {
    let activeCriteria = criteria;
    if (activeCriteria === undefined) {
      const existing = await prisma.test.findUnique({
        where: { id },
        select: { acceptanceCriteria: true },
      });
      activeCriteria = asObject(existing?.acceptanceCriteria);
    }
    const metrics = asObject(result);
    const evald = evaluateAcceptance(metrics, activeCriteria);
    data.passed = evald.passed;
  }

  return prisma.test.update({ where: { id }, data, include });
}

export async function remove(id) {
  await prisma.test.delete({ where: { id } });
}
