import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";

/**
 * Laboratory (single-tenant) service.
 *
 * Laboratory is a singleton: the setup wizard (bootstrap-admin.js) creates the
 * single row; here we only read/update, a second laboratory is never created.
 */

// Fields updatable via PATCH (id/createdAt are preserved).
const EDITABLE_FIELDS = ["name", "code", "timezone", "settings"];

/** Returns the Laboratory singleton; 404 if missing (setup has not run yet). */
export async function getLaboratory() {
  const lab = await prisma.laboratory.findFirst({ orderBy: { createdAt: "asc" } });
  if (!lab) throw ApiError.notFound("Laboratory has not been created yet", "lab.notCreated");
  return lab;
}

/** Updates the allowed fields of the singleton. */
export async function updateLaboratory(input) {
  const lab = await getLaboratory();
  const data = {};
  for (const field of EDITABLE_FIELDS) {
    if (input[field] !== undefined) data[field] = input[field];
  }
  if (Object.keys(data).length === 0) return lab;
  return prisma.laboratory.update({ where: { id: lab.id }, data });
}
