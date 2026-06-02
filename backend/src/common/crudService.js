import { prisma } from "../lib/prisma.js";
import { ApiError } from "../utils/ApiError.js";

/**
 * Reusable CRUD service factory for simple resources.
 *
 * @param {object} opts
 * @param {string} opts.model    Prisma model name (e.g. "device")
 * @param {string[]} opts.allowed Whitelist of writable fields
 * @param {object} [opts.include] Prisma include
 */
export function createCrudService({ model, allowed, include }) {
  const delegate = prisma[model];

  const pick = (body = {}) => {
    const out = {};
    for (const key of allowed) {
      if (body[key] !== undefined) out[key] = body[key];
    }
    return out;
  };

  return {
    list: () => delegate.findMany({ orderBy: { createdAt: "desc" }, include }),

    async getById(id) {
      const row = await delegate.findUnique({ where: { id }, include });
      if (!row) throw ApiError.notFound();
      return row;
    },

    create: (body) => delegate.create({ data: pick(body), include }),

    update: (id, body) => delegate.update({ where: { id }, data: pick(body), include }),

    async remove(id) {
      await delegate.delete({ where: { id } });
    },
  };
}
