import "dotenv/config";
import bcrypt from "bcryptjs";
import { config } from "../src/config/index.js";
import { prisma } from "../src/lib/prisma.js";
import { generateStrongPassword } from "../src/utils/password.js";

/**
 * Superadmin bootstrap — Docker açılışında çalışır.
 *
 * - Sistemde zaten bir ADMIN varsa: hiçbir şey yapmaz (idempotent).
 * - Yoksa: ADMIN_EMAIL ile bir superadmin oluşturur, GÜÇLÜ bir şifre üretir ve
 *   bu şifreyi açılış log'una **bir kez** yazar. İlk girişten sonra şifre
 *   içeriden (User management) değiştirilmelidir.
 *
 * Internal SaaS: public signup yoktur; ilk ve tek otomatik kullanıcı budur.
 */
async function main() {
  const adminCount = await prisma.user.count({ where: { role: "ADMIN" } });
  if (adminCount > 0) {
    console.log("→ Superadmin zaten mevcut, bootstrap atlandı.");
    return;
  }

  const email = config.adminEmail;
  const existing = await prisma.user.findUnique({ where: { email } });
  if (existing) {
    console.log(`→ '${email}' zaten kayıtlı ama ADMIN değil. Bootstrap atlandı.`);
    return;
  }

  const password = generateStrongPassword();
  const admin = await prisma.user.create({
    data: {
      email,
      name: "Superadmin",
      role: "ADMIN",
      password: await bcrypt.hash(password, config.bcryptRounds),
    },
  });

  const line = "═".repeat(64);
  console.log(`\n${line}`);
  console.log("  SUPERADMIN OLUŞTURULDU — bu şifre yalnızca BİR KEZ gösterilir");
  console.log(line);
  console.log(`  E-posta : ${admin.email}`);
  console.log(`  Şifre   : ${password}`);
  console.log(line);
  console.log("  İlk girişten sonra şifreyi değiştirin (User management).");
  console.log(`${line}\n`);
}

main()
  .then(() => process.exit(0))
  .catch((e) => {
    console.error("Bootstrap hatası:", e);
    process.exit(1);
  });
