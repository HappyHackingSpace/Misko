import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";

/**
 * Scenario service - the central experiment definition (docs/DOMAIN.md §3).
 *
 * A scenario references one or more Environments (N-N) and may therefore span one
 * or more paradigms. A Test = Subject + Scenario; a run goes environment by
 * environment and records each environment's metric results. Scenarios do not
 * carry pass/fail criteria - results are data, interpreted at the analysis layer.
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

  return prisma.scenario.create({
    data: {
      name,
      description: input.description ?? null,
      sessionParams: input.sessionParams ?? undefined,
      environments: { connect: environments.map((e) => ({ id: e.id })) },
    },
    include,
  });
}

export async function updateScenario(id, input = {}) {
  await getScenario(id);
  const data = {};

  if (input.name !== undefined) {
    const name = typeof input.name === "string" ? input.name.trim() : "";
    if (!name) throw ApiError.badRequest("name must be a non-empty string", "common.nameRequired");
    data.name = name;
  }
  if (input.description !== undefined) data.description = input.description;
  if (input.sessionParams !== undefined) data.sessionParams = input.sessionParams ?? undefined;

  if (input.environmentIds !== undefined) {
    const environments = await loadEnvironments(input.environmentIds);
    data.environments = { set: environments.map((e) => ({ id: e.id })) };
  }

  return prisma.scenario.update({ where: { id }, data, include });
}

export async function deleteScenario(id) {
  await getScenario(id);
  await prisma.scenario.delete({ where: { id } });
}
