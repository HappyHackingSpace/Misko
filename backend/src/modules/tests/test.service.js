import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";
import { evaluateAcceptance } from "../../config/acceptance.js";
import { validateMetrics } from "../../config/metrics.js";

/**
 * Test service. A Test = Subject + Scenario (docs/DOMAIN.md §4). The scenario has
 * one or more environments; a run goes environment by environment (start -> enter
 * metrics -> finish). Results are stored per environment in `Test.result`:
 *   { schemaVersion, environments: { [envId]: { status, startedAt, endedAt, metrics } } }
 * Both manual entry and the CV service fill the same metric keys, validated
 * against the paradigm dictionary. `passed` is evaluated per environment against
 * the scenario's expected results once the whole test is done.
 */

const RESULT_SCHEMA_VERSION = 1;

const include = {
  scenario: {
    select: {
      id: true,
      name: true,
      acceptance: true,
      environments: { select: { id: true, name: true, paradigmKey: true } },
    },
  },
  subject: { select: { id: true, code: true, groupName: true } },
  operator: { select: { id: true, name: true } },
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
export async function create({ scenarioId, subjectId, notes }, operatorId) {
  if (!scenarioId || !subjectId) {
    throw ApiError.badRequest("scenarioId and subjectId are required", "test.idsRequired");
  }
  const scenario = await prisma.scenario.findUnique({ where: { id: scenarioId } });
  if (!scenario) throw ApiError.badRequest("Unknown scenario", "scenario.notFound");

  return prisma.test.create({
    data: { scenarioId, subjectId, notes: notes || null, operatorId },
    include,
  });
}

/** Test-level updates: overall status (e.g. cancel -> FAILED) and notes. */
export async function update(id, payload) {
  const { status, notes } = payload;
  const data = {};
  if (status !== undefined) data.status = status;
  if (notes !== undefined) data.notes = notes;
  return prisma.test.update({ where: { id }, data, include });
}

/** Reads the scenario's per-environment acceptance map as a plain object. */
function acceptanceMap(scenario) {
  const a = scenario?.acceptance;
  return a && typeof a === "object" && !Array.isArray(a) ? a : {};
}

/**
 * Derives the test-level status and passed verdict from the per-environment
 * result. Status is DONE only when every scenario environment is DONE; passed is
 * evaluated (per environment, against the scenario's expected results) only then.
 */
function deriveStatusAndPassed(scenario, resultEnvironments) {
  const envs = scenario.environments;
  const statuses = envs.map((e) => resultEnvironments[e.id]?.status ?? "PENDING");
  let status;
  if (statuses.every((s) => s === "DONE")) status = "DONE";
  else if (statuses.some((s) => s === "RUNNING" || s === "DONE")) status = "RUNNING";
  else status = "PENDING";

  let passed = null;
  if (status === "DONE") {
    const acc = acceptanceMap(scenario);
    let anyCriteria = false;
    let allPass = true;
    for (const e of envs) {
      const criteria = Array.isArray(acc[e.id]) ? acc[e.id] : null;
      if (!criteria || !criteria.length) continue;
      anyCriteria = true;
      const metrics = resultEnvironments[e.id]?.metrics ?? null;
      if (evaluateAcceptance(metrics, criteria).passed !== true) allPass = false;
    }
    passed = anyCriteria ? allPass : null;
  }
  return { status, passed };
}

/**
 * Records a run for one environment of the test's scenario: start it, write its
 * metrics, and/or finish it. `metrics` is validated against that environment's
 * paradigm dictionary. Test-level status and passed are recomputed.
 */
export async function submitEnvironmentResult(id, envId, { status: envStatus, metrics } = {}) {
  const test = await prisma.test.findUnique({ where: { id }, include });
  if (!test) throw ApiError.notFound("Not found", "common.notFound");

  const env = test.scenario.environments.find((e) => e.id === envId);
  if (!env) throw ApiError.badRequest("Environment is not part of this test's scenario", "test.envNotInScenario", { envId });

  const result = test.result && typeof test.result === "object" && !Array.isArray(test.result)
    ? { ...test.result }
    : { schemaVersion: RESULT_SCHEMA_VERSION };
  result.schemaVersion = RESULT_SCHEMA_VERSION;
  const environments = { ...(result.environments ?? {}) };
  const entry = { status: "PENDING", ...(environments[envId] ?? {}) };

  if (metrics !== undefined) {
    const { valid, errors } = validateMetrics(env.paradigmKey, metrics ?? {});
    if (!valid) {
      throw ApiError.badRequest(`Invalid metrics: ${errors.join("; ")}`, "test.invalidMetrics", { errors: errors.join("; ") });
    }
    entry.metrics = metrics ?? {};
  }
  if (envStatus === "RUNNING") {
    entry.status = "RUNNING";
    entry.startedAt = entry.startedAt ?? new Date().toISOString();
  } else if (envStatus === "DONE") {
    entry.status = "DONE";
    entry.endedAt = new Date().toISOString();
  }
  environments[envId] = entry;
  result.environments = environments;

  const { status, passed } = deriveStatusAndPassed(test.scenario, environments);
  const data = { result, status, passed };
  if (status !== "PENDING" && !test.startedAt) data.startedAt = new Date();
  if (status === "DONE") data.endedAt = new Date();
  else if (test.endedAt) data.endedAt = null;

  return prisma.test.update({ where: { id }, data, include });
}

export async function remove(id) {
  await prisma.test.delete({ where: { id } });
}
