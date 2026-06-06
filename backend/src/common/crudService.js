import { prisma } from "../lib/prisma.js";
import { ApiError } from "../utils/ApiError.js";
import { buildListQuery, listResult } from "./listQuery.js";

/**
 * Reusable CRUD service factory for simple resources.
 *
 * @param {object} opts
 * @param {string} opts.model    Prisma model name (e.g. "subject")
 * @param {string[]} opts.allowed Whitelist of writable fields
 * @param {object} [opts.include] Prisma include
 * @param {string[]} [opts.searchFields]  text columns for the global search term
 * @param {Record<string,"text"|"enum"|"boolean">} [opts.filterFields]  per-column filters
 * @param {string[]} [opts.sortFields]  columns allowed in `sort`
 */
export function createCrudService({ model, allowed, include, searchFields, filterFields, sortFields }) {
  const delegate = prisma[model];
  const listConfig = { searchFields, filterFields, sortFields };

  const pick = (body = {}) => {
    const out = {};
    for (const key of allowed) {
      if (body[key] !== undefined) out[key] = body[key];
    }
    return out;
  };

  return {
    async list(query = {}) {
      const q = buildListQuery(query, listConfig);
      const [data, total] = await Promise.all([
        delegate.findMany({ where: q.where, orderBy: q.orderBy, skip: q.skip, take: q.take, include }),
        delegate.count({ where: q.where }),
      ]);
      return listResult(data, total, q);
    },

    async getById(id) {
      const row = await delegate.findUnique({ where: { id }, include });
      if (!row) throw ApiError.notFound("Not found", "common.notFound");
      return row;
    },

    create: (body) => delegate.create({ data: pick(body), include }),

    update: (id, body) => delegate.update({ where: { id }, data: pick(body), include }),

    async remove(id) {
      await delegate.delete({ where: { id } });
    },
  };
}
