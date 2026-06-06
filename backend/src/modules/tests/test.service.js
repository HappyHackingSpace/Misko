import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";
import { evaluateAcceptance } from "../../config/acceptance.js";

/**
 * Test service. A Test = Subject + Scenario, run once (docs/DOMAIN.md §4). The
 * scenario carries the environments (and thus paradigms), the metrics and the
 * expected results. When a result is written, metrics are evaluated against the
 * scenario's acceptance to set `passed`.
 */

const include = {
  scenario: {
    select: {
      id: true,
      name: true,
      environments: { select: { id: true, name: true, paradigmKey: true } },
    },
  },
  subject: { select: { id: true, code: true, groupName: true } },
  operator: { select: { id: true, name: true } },
  device: { select: { id: true, name: true } },
};

export async function list(query = {}) {
  const q = buildListQuery(query, {
    searchFields: ["notes"],
    filterFields: { status: "enum", passed: "boolean" },
    sortFields: ["status", "createdAt"],
  });
  const [data, total] = await Promise.all([
    prisma.test.findMany({ where: q.where, orderBy: q.orderBy, skip: q.skip, take: q.take, include }),
    prisma.test.count({ where: q.where }),
  ]);
  return listResult(data, total, q);
}

export async function getById(id) {
  const row = await prisma.test.findUnique({ where: { id }, include });
  if (!row) throw ApiError.notFound("Not found", "common.notFound");
  return row;
}

/** Creates a new test (Subject + Scenario). The operator is the logged-in user. */
export async function create({ scenarioId, subjectId, deviceId, notes }, operatorId) {
  if (!scenarioId || !subjectId) {
    throw ApiError.badRequest("scenarioId and subjectId are required", "test.idsRequired");
  }
  const scenario = await prisma.scenario.findUnique({ where: { id: scenarioId } });
  if (!scenario) throw ApiError.badRequest("Unknown scenario", "scenario.notFound");

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
 * Updates status/result (start, finish, write result). `result` is structured
 * JSON. When a result is written and `passed` is not given explicitly, metrics
 * are evaluated against the test's scenario's acceptance criteria. With no
 * acceptance, passed = null.
 */
export async function update(id, payload) {
  const { status, startedAt, endedAt, result, passed, notes } = payload;
  const data = {};
  if (status !== undefined) data.status = status;
  if (startedAt !== undefined) data.startedAt = startedAt ? new Date(startedAt) : null;
  if (endedAt !== undefined) data.endedAt = endedAt ? new Date(endedAt) : null;
  if (result !== undefined) data.result = result ?? null;
  if (notes !== undefined) data.notes = notes;

  if (passed !== undefined) {
    data.passed = passed; // manual override
  } else if (result !== undefined) {
    const test = await prisma.test.findUnique({
      where: { id },
      select: { scenario: { select: { acceptance: true } } },
    });
    // Acceptance is per-environment: { [environmentId]: [criteria] }. Flatten all
    // of the scenario's environments' expected results to evaluate this result.
    const acc = test?.scenario?.acceptance;
    const criteria = acc && typeof acc === "object" && !Array.isArray(acc)
      ? Object.values(acc).flat()
      : null;
    data.passed = evaluateAcceptance(result ?? null, criteria).passed;
  }

  return prisma.test.update({ where: { id }, data, include });
}

export async function remove(id) {
  await prisma.test.delete({ where: { id } });
}
