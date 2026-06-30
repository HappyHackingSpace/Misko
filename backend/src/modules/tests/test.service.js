import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";
import { getParadigmSpec, RESULT_SCHEMA_VERSION } from "../../config/paradigms.js";
import { computeMetrics, validateEvent } from "../../config/metricEngine.js";

/**
 * Test service. A Test = Subject + Scenario (docs/METRIC_ENGINE.md). The scenario
 * has one or more environments; a run goes environment by environment. The
 * operator logs EVENTS; the paradigm's engine derives the METRIC RESULTS from
 * them (live on each event, finalized on Finish). The CV service feeds the same
 * engine. Results are data - there is no pass/fail verdict.
 *   result = { schemaVersion, environments: { [envId]:
 *     { status, startedAt, endedAt, events: [...], cvInputs?: {...}, metrics: {...} } } }
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
};

export async function list(query = {}) {
  const q = buildListQuery(query, {
    searchFields: ["notes"],
    filterFields: { status: "enum" },
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
  const [scenario, subject] = await Promise.all([
    prisma.scenario.findUnique({ where: { id: scenarioId } }),
    prisma.subject.findUnique({ where: { id: subjectId } }),
  ]);
  if (!scenario) throw ApiError.badRequest("Unknown scenario", "scenario.notFound");
  if (!subject) throw ApiError.badRequest("Unknown subject", "subject.notFound");

  return prisma.test.create({
    data: { scenarioId, subjectId, notes: notes || null, operatorId },
    include,
  });
}

/**
 * Valid test-level status transitions. The environment-level lifecycle
 * (PENDING -> RUNNING -> DONE) is managed by submitEnvironmentResult;
 * this gate covers manual overrides at the test level (e.g. cancel).
 */
const VALID_STATUS_TRANSITIONS = {
  PENDING: ["RUNNING", "FAILED"],
  RUNNING: ["DONE", "FAILED"],
  DONE: ["RUNNING"],       // re-open
  FAILED: ["PENDING"],     // retry
};

/** Test-level updates: overall status (e.g. cancel -> FAILED) and notes. */
export async function update(id, payload) {
  const { status, notes } = payload;
  const data = {};

  if (status !== undefined) {
    const test = await prisma.test.findUnique({ where: { id }, select: { status: true } });
    if (!test) throw ApiError.notFound("Not found", "common.notFound");

    const allowed = VALID_STATUS_TRANSITIONS[test.status] ?? [];
    if (!allowed.includes(status)) {
      throw ApiError.badRequest(
        `Cannot transition from ${test.status} to ${status}`,
        "test.invalidTransition",
        { from: test.status, to: status },
      );
    }
    data.status = status;
  }

  if (notes !== undefined) data.notes = notes;
  return prisma.test.update({ where: { id }, data, include });
}

/**
 * Derives the test-level status from the per-environment result. DONE only when
 * every scenario environment is DONE; RUNNING once any has started.
 */
function deriveStatus(scenario, resultEnvironments) {
  const statuses = scenario.environments.map((e) => resultEnvironments[e.id]?.status ?? "PENDING");
  if (statuses.every((s) => s === "DONE")) return "DONE";
  if (statuses.some((s) => s === "RUNNING" || s === "DONE")) return "RUNNING";
  return "PENDING";
}

/**
 * Loads a test, mutates one environment's result entry via `mutate(entry, env)`,
 * re-derives that environment's metrics from its events (the paradigm engine),
 * recomputes test-level status, and persists. Shared by all run operations.
 */
async function mutateEnvironment(id, envId, mutate) {
  const test = await prisma.test.findUnique({ where: { id }, include });
  if (!test) throw ApiError.notFound("Not found", "common.notFound");
  const env = test.scenario.environments.find((e) => e.id === envId);
  if (!env) throw ApiError.badRequest("Environment is not part of this test's scenario", "test.envNotInScenario", { envId });

  const base = test.result && typeof test.result === "object" && !Array.isArray(test.result) ? test.result : {};
  const result = { ...base, schemaVersion: RESULT_SCHEMA_VERSION };
  const environments = { ...(result.environments ?? {}) };
  const entry = { status: "PENDING", events: [], ...(environments[envId] ?? {}) };

  mutate(entry, env);

  // Re-derive this environment's metrics from its events (live).
  entry.metrics = computeMetrics(env.paradigmKey, entry.events ?? [], entry.cvInputs ?? {});
  environments[envId] = entry;
  result.environments = environments;

  const status = deriveStatus(test.scenario, environments);
  const data = { result, status };
  if (status !== "PENDING" && !test.startedAt) data.startedAt = new Date();
  if (status === "DONE") data.endedAt = new Date();
  else if (test.endedAt) data.endedAt = null;

  return prisma.test.update({ where: { id }, data, include });
}

/** Start or finish one environment's run. */
export function submitEnvironmentResult(id, envId, { status: envStatus } = {}) {
  return mutateEnvironment(id, envId, (entry) => {
    if (envStatus === "RUNNING") {
      entry.status = "RUNNING";
      entry.startedAt = entry.startedAt ?? new Date().toISOString();
    } else if (envStatus === "DONE") {
      entry.status = "DONE";
      entry.endedAt = new Date().toISOString();
    }
  });
}

/** Appends an event to one environment's run; metrics are re-derived live. */
export function addEnvironmentEvent(id, envId, event) {
  return mutateEnvironment(id, envId, (entry, env) => {
    const spec = getParadigmSpec(env.paradigmKey);
    const error = validateEvent(spec?.eventTypes ?? [], event);
    if (error) throw ApiError.badRequest(`Invalid event: ${error}`, "test.invalidEvent", { error });
    entry.events = [...(entry.events ?? []), {
      type: event.type,
      t: typeof event.t === "number" ? event.t : 0,
      payload: event.payload ?? {},
    }];
    if (entry.status === "PENDING") {
      entry.status = "RUNNING";
      entry.startedAt = entry.startedAt ?? new Date().toISOString();
    }
  });
}

/** Removes the event at `index` from one environment's run; metrics re-derived. */
export function removeEnvironmentEvent(id, envId, index) {
  return mutateEnvironment(id, envId, (entry) => {
    const events = [...(entry.events ?? [])];
    if (index < 0 || index >= events.length) throw ApiError.badRequest("Event index out of range", "test.eventIndex");
    events.splice(index, 1);
    entry.events = events;
  });
}

export async function remove(id) {
  await prisma.test.delete({ where: { id } });
}
