import bcrypt from "bcryptjs";
import { config } from "../../config/index.js";
import { PRIVILEGED_ROLES, isPrivilegedRole } from "../../config/permissions.js";
import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { buildListQuery, listResult } from "../../common/listQuery.js";
import { generateStrongPassword } from "../../utils/password.js";
import { toPublicUser } from "../auth/auth.service.js";

const hash = (pw) => bcrypt.hash(pw, config.bcryptRounds);

export async function list(query = {}) {
  const q = buildListQuery(query, {
    searchFields: ["name", "email"],
    filterFields: { name: "text", email: "text", role: "enum" },
    sortFields: ["name", "email", "role", "createdAt"],
  });
  const [users, total] = await Promise.all([
    prisma.user.findMany({ where: q.where, orderBy: q.orderBy, skip: q.skip, take: q.take }),
    prisma.user.count({ where: q.where }),
  ]);
  return listResult(users.map(toPublicUser), total, q);
}

export async function getById(id) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("User not found", "user.notFound");
  return toPublicUser(user);
}

/**
 * Creates a user internally by an ADMIN. If no password is given the system generates one;
 * the generated/assigned plaintext password is returned to the caller **once** via `generatedPassword`.
 */
export async function create({ name, email, role, password }) {
  const plain = password || generateStrongPassword();
  try {
    const user = await prisma.user.create({
      data: { name, email, role, password: await hash(plain) },
    });
    return { user: toPublicUser(user), generatedPassword: password ? undefined : plain };
  } catch (error) {
    if (error?.code === "P2002") {
      throw ApiError.conflict("This email is already registered", "user.emailExists");
    }
    throw error;
  }
}

export async function update(id, data) {
  // Run the privileged-user guard and the write in one transaction so concurrent
  // demotions cannot both pass the count check and drive privileged users to zero.
  return prisma.$transaction(async (tx) => {
    const user = await tx.user.findUnique({ where: { id } });
    if (!user) throw ApiError.notFound("User not found", "user.notFound");

    // Prevent demoting the last privileged user (with user:manage) to an unprivileged role.
    if (isPrivilegedRole(user.role) && data.role && !isPrivilegedRole(data.role)) {
      await assertNotLastPrivileged(tx);
    }

    const updated = await tx.user.update({ where: { id }, data });
    return toPublicUser(updated);
  });
}

export async function resetPassword(id, password) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("User not found", "user.notFound");

  const plain = password || generateStrongPassword();
  await prisma.user.update({ where: { id }, data: { password: await hash(plain) } });
  return { generatedPassword: password ? undefined : plain };
}

export async function remove(id, currentUserId) {
  return prisma.$transaction(async (tx) => {
    const user = await tx.user.findUnique({ where: { id } });
    if (!user) throw ApiError.notFound("User not found", "user.notFound");
    if (id === currentUserId) throw ApiError.badRequest("You cannot delete your own account", "user.cannotDeleteSelf");
    if (isPrivilegedRole(user.role)) await assertNotLastPrivileged(tx);

    await tx.user.delete({ where: { id } });
  });
}

async function assertNotLastPrivileged(tx) {
  const privileged = await tx.user.count({ where: { role: { in: PRIVILEGED_ROLES } } });
  if (privileged <= 1) {
    throw ApiError.badRequest("The last privileged user cannot be removed or changed", "user.lastPrivileged");
  }
}
