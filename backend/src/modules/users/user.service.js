import bcrypt from "bcryptjs";
import { config } from "../../config/index.js";
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
 * ADMIN tarafından içeriden kullanıcı oluşturur. Şifre verilmezse sistem üretir;
 * üretilen/atanan düz şifre çağrıya **bir kez** `generatedPassword` ile döner.
 */
export async function create({ name, email, role, password }) {
  const exists = await prisma.user.findUnique({ where: { email } });
  if (exists) throw ApiError.conflict("Bu e-posta zaten kayıtlı");

  const plain = password || generateStrongPassword();
  const user = await prisma.user.create({
    data: { name, email, role, password: await hash(plain) },
  });

  return { user: toPublicUser(user), generatedPassword: password ? undefined : plain };
}

export async function update(id, data) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("Kullanıcı bulunamadı");

  // Son ADMIN'i OPERATOR'a düşürmeyi engelle.
  if (user.role === "ADMIN" && data.role === "OPERATOR") {
    await assertNotLastAdmin(id);
  }

  const updated = await prisma.user.update({ where: { id }, data });
  return toPublicUser(updated);
}

export async function resetPassword(id, password) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("Kullanıcı bulunamadı");

  const plain = password || generateStrongPassword();
  await prisma.user.update({ where: { id }, data: { password: await hash(plain) } });
  return { generatedPassword: password ? undefined : plain };
}

export async function remove(id, currentUserId) {
  const user = await prisma.user.findUnique({ where: { id } });
  if (!user) throw ApiError.notFound("Kullanıcı bulunamadı");
  if (id === currentUserId) throw ApiError.badRequest("Kendi hesabınızı silemezsiniz");
  if (user.role === "ADMIN") await assertNotLastAdmin(id);

  await prisma.user.delete({ where: { id } });
}

async function assertNotLastAdmin(id) {
  const admins = await prisma.user.count({ where: { role: "ADMIN" } });
  if (admins <= 1) throw ApiError.badRequest("Son ADMIN kullanıcı kaldırılamaz/değiştirilemez");
  return id;
}
