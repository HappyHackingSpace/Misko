import bcrypt from "bcryptjs";
import jwt from "jsonwebtoken";
import { config } from "../../config/index.js";
import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";

// Not: Bu internal bir SaaS uygulamasıdır — public kayıt (signup) yoktur.
// Kullanıcılar yalnızca ADMIN tarafından içeriden açılır (bkz. modules/users).

export const toPublicUser = (u) => ({
  id: u.id,
  email: u.email,
  name: u.name,
  role: u.role,
  createdAt: u.createdAt,
});

export function signToken(user) {
  return jwt.sign({ sub: user.id, role: user.role }, config.jwt.secret, {
    expiresIn: config.jwt.ttl,
  });
}

export async function login({ email, password }) {
  const user = await prisma.user.findUnique({ where: { email } });
  if (!user || !(await bcrypt.compare(password, user.password))) {
    throw ApiError.unauthorized("E-posta veya şifre hatalı");
  }
  return { token: signToken(user), user: toPublicUser(user) };
}
