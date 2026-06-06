import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";
import { validateAcceptanceCriteria } from "../../config/acceptance.js";

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
/**
 * Acceptance is PER-ENVIRONMENT: a map `{ [environmentId]: [criteria] }`. Each
 * environment defines its own expected results, validated against that
 * environment's paradigm metrics. Every key must be one of the scenario's
 * environments.
 */
function validateAcceptance(acceptance, environments) {
  if (acceptance == null) return;
  if (typeof acceptance !== "object" || Array.isArray(acceptance)) {
    throw ApiError.badRequest("acceptance must be an object keyed by environment id", "scenario.acceptanceObject");
  }
  const envById = new Map(environments.map((e) => [e.id, e]));
  for (const [envId, criteria] of Object.entries(acceptance)) {
    const env = envById.get(envId);
    if (!env) {
      throw ApiError.badRequest("Acceptance references an environment not in this scenario", "scenario.acceptanceUnknownEnv", { envId });
    }
    const { valid, errors } = validateAcceptanceCriteria(criteria, { paradigmKey: env.paradigmKey });
    if (!valid) {
      throw ApiError.badRequest(`Invalid acceptance for ${env.name}: ${errors.join("; ")}`, "test.invalidAcceptance", { errors: errors.join("; ") });
    }
  }
}

/** Drops acceptance entries whose environment is no longer part of the scenario. */
function pruneAcceptance(acceptance, environments) {
  if (acceptance == null || typeof acceptance !== "object") return acceptance;
  const ids = new Set(environments.map((e) => e.id));
  const out = {};
  for (const [envId, criteria] of Object.entries(acceptance)) {
    if (ids.has(envId)) out[envId] = criteria;
  }
  return out;
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
    // Environment set changed: drop acceptance for removed environments.
    data.acceptance = pruneAcceptance(existing.acceptance ?? null, environments);
  }

  return prisma.scenario.update({ where: { id }, data, include });
}

export async function deleteScenario(id) {
  await getScenario(id);
  await prisma.scenario.delete({ where: { id } });
}
