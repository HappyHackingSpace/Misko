import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";
import { getParadigmSpec } from "../../config/paradigms.js";
import { getLaboratory } from "../lab/lab.service.js";

/**
 * Environment ("Ortam") service.
 *
 * An Environment is a named, persisted instance created FROM a paradigm template.
 * A lab can hold many environments per paradigm (e.g. two distinct Morris water
 * tanks). The paradigm DEFINITION stays in code (src/config/paradigms.js); only
 * the chosen apparatus VALUES are stored, as a full self-contained snapshot in
 * `config`. These physical values are LOCKED at test time and cannot be overridden
 * by a test.
 */

/** Validates a single apparatus value against its code-defined parameter spec. */
function validateApparatusValue(param, value) {
  if (param.type === "number") {
    if (typeof value !== "number" || Number.isNaN(value)) {
      throw ApiError.badRequest(`${param.key} must be numeric`, "environment.paramNumeric", { key: param.key });
    }
    if (param.min != null && value < param.min) {
      throw ApiError.badRequest(`${param.key} must be at least ${param.min}`, "environment.paramMin", { key: param.key, min: param.min });
    }
    if (param.max != null && value > param.max) {
      throw ApiError.badRequest(`${param.key} must be at most ${param.max}`, "environment.paramMax", { key: param.key, max: param.max });
    }
  } else if (param.type === "enum") {
    if (!param.options.includes(value)) {
      throw ApiError.badRequest(`${param.key} invalid option`, "environment.paramOption", { key: param.key });
    }
  }
  return value;
}

/**
 * Builds the full, self-contained config snapshot for an environment. Every
 * apparatus parameter is resolved (provided value if valid, otherwise the spec
 * default); required parameters must end up with a value. Zones are resolved
 * against the final apparatus values so geometry is captured at creation time.
 */
function buildConfig(spec, apparatusInput = {}) {
  if (typeof apparatusInput !== "object" || apparatusInput === null || Array.isArray(apparatusInput)) {
    throw ApiError.badRequest("apparatus must be an object", "environment.apparatusObject");
  }
  const knownKeys = new Set(spec.apparatusParameters.map((p) => p.key));
  for (const key of Object.keys(apparatusInput)) {
    if (!knownKeys.has(key)) {
      throw ApiError.badRequest(`Unknown apparatus parameter: ${key}`, "environment.unknownParam", { key });
    }
  }

  const apparatus = {};
  for (const param of spec.apparatusParameters) {
    const provided = apparatusInput[param.key];
    let value;
    if (provided === undefined || provided === null) {
      value = param.default;
    } else {
      value = validateApparatusValue(param, provided);
    }
    if (value === undefined) {
      if (param.required) {
        throw ApiError.badRequest(`${param.key} is required`, "environment.paramRequired", { key: param.key });
      }
      continue; // optional with no default: leave unset
    }
    apparatus[param.key] = value;
  }

  const zones = typeof spec.zones === "function" ? spec.zones(apparatus) : spec.zones;

  return {
    paradigmKey: spec.key,
    schemaVersion: spec.schemaVersion,
    apparatus,
    zones: zones ?? [],
  };
}

/** Lists environments for the lab with pagination, search, filter and sort. */
export async function listEnvironments(query = {}) {
  const q = buildListQuery(query, {
    searchFields: ["name", "notes"],
    filterFields: { name: "text", paradigmKey: "enum" },
    sortFields: ["name", "paradigmKey", "createdAt"],
  });
  const [data, total] = await Promise.all([
    prisma.environment.findMany({ where: q.where, orderBy: q.orderBy, skip: q.skip, take: q.take }),
    prisma.environment.count({ where: q.where }),
  ]);
  return listResult(data, total, q);
}

/** Fetches a single environment by id (404 if missing). */
export async function getEnvironment(id) {
  const row = await prisma.environment.findUnique({ where: { id } });
  if (!row) throw ApiError.notFound("Environment not found", "environment.notFound");
  return row;
}

/** Creates a named environment instance from a paradigm template. */
export async function createEnvironment(input = {}) {
  const name = typeof input.name === "string" ? input.name.trim() : "";
  if (!name) throw ApiError.badRequest("name must be a non-empty string", "common.nameRequired");

  const spec = getParadigmSpec(input.paradigmKey);
  if (!spec) throw ApiError.badRequest("Unknown paradigm", "environment.unknownParadigm");

  if (input.notes !== undefined && input.notes !== null && typeof input.notes !== "string") {
    throw ApiError.badRequest("notes must be a string", "environment.notesString");
  }

  const config = buildConfig(spec, input.apparatus ?? {});
  const lab = await getLaboratory();

  return prisma.environment.create({
    data: {
      laboratoryId: lab.id,
      name,
      paradigmKey: spec.key,
      config,
      notes: input.notes ?? null,
    },
  });
}

/**
 * Updates an environment. The paradigm is immutable (a different paradigm means a
 * different environment). Name, apparatus values and notes can change; apparatus
 * changes rebuild the full config snapshot.
 */
export async function updateEnvironment(id, input = {}) {
  const existing = await getEnvironment(id);
  const data = {};

  if (input.name !== undefined) {
    const name = typeof input.name === "string" ? input.name.trim() : "";
    if (!name) throw ApiError.badRequest("name must be a non-empty string", "common.nameRequired");
    data.name = name;
  }

  if (input.notes !== undefined) {
    if (input.notes !== null && typeof input.notes !== "string") {
      throw ApiError.badRequest("notes must be a string", "environment.notesString");
    }
    data.notes = input.notes;
  }

  if (input.paradigmKey !== undefined && input.paradigmKey !== existing.paradigmKey) {
    throw ApiError.badRequest("paradigmKey cannot be changed", "environment.paradigmImmutable");
  }

  if (input.apparatus !== undefined) {
    const spec = getParadigmSpec(existing.paradigmKey);
    if (!spec) throw ApiError.badRequest("Unknown paradigm", "environment.unknownParadigm");
    data.config = buildConfig(spec, input.apparatus ?? {});
  }

  if (Object.keys(data).length === 0) return existing;
  return prisma.environment.update({ where: { id }, data });
}

/** Deletes an environment. */
export async function deleteEnvironment(id) {
  await getEnvironment(id); // 404 if missing
  await prisma.environment.delete({ where: { id } });
}
