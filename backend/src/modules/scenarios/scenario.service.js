import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";
import { validateAcceptanceCriteria } from "../../config/acceptance.js";
import { metricsForParadigm } from "../../config/metrics.js";

/**
 * Scenario service - the central experiment definition (docs/DOMAIN.md §3).
 *
 * A scenario references one or more Environments (N-N) and may therefore span one
 * or more paradigms. Its `acceptance` (expected results) is validated against the
 * UNION of those paradigms' metric dictionaries. A Test = Subject + Scenario.
 */

const include = {
  environments: { select: { id: true, name: true, paradigmKey: true } },
  _count: { select: { tests: true } },
};

/** Loads the given environments (404 on any missing) and returns them. */
async function loadEnvironments(environmentIds) {
  if (environmentIds === undefined) return undefined;
  if (!Array.isArray(environmentIds)) {
    throw ApiError.badRequest("environmentIds must be an array", "scenario.environmentsArray");
  }
  const unique = [...new Set(environmentIds)];
  const envs = await prisma.environment.findMany({ where: { id: { in: unique } } });
  if (envs.length !== unique.length) {
    throw ApiError.badRequest("One or more environments not found", "scenario.unknownEnvironment");
  }
  return envs;
}

/** The set of metric keys allowed for a scenario = union over its environments' paradigms. */
function allowedMetricKeys(environments) {
  const keys = new Set();
  for (const env of environments) {
    for (const m of metricsForParadigm(env.paradigmKey)) keys.add(m.key);
  }
  return keys;
}

/** Validates the acceptance list structurally and against the scenario's paradigms. */
function validateAcceptance(acceptance, environments) {
  if (acceptance == null) return;
  const { valid, errors } = validateAcceptanceCriteria(acceptance);
  if (!valid) {
    throw ApiError.badRequest(`Invalid acceptance: ${errors.join("; ")}`, "test.invalidAcceptance", { errors: errors.join("; ") });
  }
  const allowed = allowedMetricKeys(environments);
  for (const c of acceptance) {
    const root = c.metricKey.includes(".") ? c.metricKey.slice(0, c.metricKey.indexOf(".")) : c.metricKey;
    if (!allowed.has(c.metricKey) && !allowed.has(root)) {
      throw ApiError.badRequest(
        `${c.metricKey} is not a metric of this scenario's paradigms`,
        "scenario.metricNotInParadigm",
        { metricKey: c.metricKey },
      );
    }
  }
}

export async function listScenarios(query = {}) {
  const q = buildListQuery(query, {
    searchFields: ["name", "description"],
    filterFields: { name: "text" },
    sortFields: ["name", "createdAt"],
  });
  const [data, total] = await Promise.all([
    prisma.scenario.findMany({ where: q.where, orderBy: q.orderBy, skip: q.skip, take: q.take, include }),
    prisma.scenario.count({ where: q.where }),
  ]);
  return listResult(data, total, q);
}

export async function getScenario(id) {
  const row = await prisma.scenario.findUnique({ where: { id }, include });
  if (!row) throw ApiError.notFound("Scenario not found", "scenario.notFound");
  return row;
}

export async function createScenario(input = {}) {
  const name = typeof input.name === "string" ? input.name.trim() : "";
  if (!name) throw ApiError.badRequest("name must be a non-empty string", "common.nameRequired");

  const environments = (await loadEnvironments(input.environmentIds)) ?? [];
  validateAcceptance(input.acceptance ?? null, environments);

  return prisma.scenario.create({
    data: {
      name,
      description: input.description ?? null,
      acceptance: input.acceptance ?? undefined,
      sessionParams: input.sessionParams ?? undefined,
      environments: { connect: environments.map((e) => ({ id: e.id })) },
    },
    include,
  });
}

export async function updateScenario(id, input = {}) {
  const existing = await getScenario(id);
  const data = {};

  if (input.name !== undefined) {
    const name = typeof input.name === "string" ? input.name.trim() : "";
    if (!name) throw ApiError.badRequest("name must be a non-empty string", "common.nameRequired");
    data.name = name;
  }
  if (input.description !== undefined) data.description = input.description;
  if (input.sessionParams !== undefined) data.sessionParams = input.sessionParams ?? undefined;

  // Resolve the environment set that will be in effect after this update, so
  // acceptance is validated against the right paradigms.
  const environments = input.environmentIds !== undefined
    ? await loadEnvironments(input.environmentIds)
    : existing.environments;
  if (input.environmentIds !== undefined) {
    data.environments = { set: environments.map((e) => ({ id: e.id })) };
  }

  if (input.acceptance !== undefined) {
    validateAcceptance(input.acceptance ?? null, environments);
    data.acceptance = input.acceptance ?? null;
  } else if (input.environmentIds !== undefined) {
    // Environment set changed: re-validate the existing acceptance against it.
    validateAcceptance(existing.acceptance ?? null, environments);
  }

  return prisma.scenario.update({ where: { id }, data, include });
}

export async function deleteScenario(id) {
  await getScenario(id);
  await prisma.scenario.delete({ where: { id } });
}
