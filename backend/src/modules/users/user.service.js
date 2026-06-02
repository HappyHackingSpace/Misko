import bcrypt from "bcryptjs";
import { config } from "../../config/index.js";
import { PRIVILEGED_ROLES, isPrivilegedRole } from "../../config/permissions.js";
import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { generateStrongPassword } from "../../utils/password.js";
import { toPublicUser } from "../auth/auth.service.js";

const hash = (pw) => bcrypt.hash(pw, config.bcryptRounds);

export async function list() {
  const users = await prisma.user.findMany({ orderBy: { createdAt: "desc" } });
  return users.map(toPublicUser);
}

/**
 * Creates a user internally by an ADMIN. If no password is given the system generates one;
 * the generated/assigned plaintext password is returned to the caller **once** via `generatedPassword`.
 */
export async function create({ name, email, role, password }) {
  const exists = await prisma.user.findUnique({ where: { email } });
  if (exists) throw ApiError.conflict("This email is already registered");

  const plain = password || generateStrongPassword();
  const user = await prisma.user.create({
    data: { name, email, role, password: await hash(plain) },
  });

  return { user: toPublicUser(user), generatedPassword: password ? undefined : plain };
}

export async function update(id, data) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("User not found");

  // Prevent demoting the last privileged user (with user:manage) to an unprivileged role.
  if (isPrivilegedRole(user.role) && data.role && !isPrivilegedRole(data.role)) {
    await assertNotLastPrivileged(id);
  }

  const updated = await prisma.user.update({ where: { id }, data });
  return toPublicUser(updated);
}

export async function resetPassword(id, password) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("User not found");

  const plain = password || generateStrongPassword();
  await prisma.user.update({ where: { id }, data: { password: await hash(plain) } });
  return { generatedPassword: password ? undefined : plain };
}

export async function remove(id, currentUserId) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("User not found");
  if (id === currentUserId) throw ApiError.badRequest("You cannot delete your own account");
  if (isPrivilegedRole(user.role)) await assertNotLastPrivileged(id);

  await prisma.user.delete({ where: { id } });
}

async function assertNotLastPrivileged(id) {
  const privileged = await prisma.user.count({ where: { role: { in: PRIVILEGED_ROLES } } });
  if (privileged <= 1) {
    throw ApiError.badRequest("The last privileged user cannot be removed or changed");
  }
  return id;
}
