import "dotenv/config";
import bcrypt from "bcryptjs";
import { config } from "../src/config/index.js";
import { ROLES } from "../src/config/permissions.js";
import { prisma } from "../src/lib/prisma.js";
import { generateStrongPassword } from "../src/utils/password.js";

/**
 * Setup wizard (CLI) - runs at Docker startup.
 *
 * Prepares two things together (single tenant / on-prem):
 *  1. Laboratory singleton (exactly ONE laboratory per install).
 *  2. SUPERADMIN user.
 *
 * Everything is idempotent: a second run skips what already exists and does not
 * create a second laboratory (singleton guarantee).
 */

/**
 * Ensures the Laboratory singleton. Returns it if it already exists; otherwise
 * creates it with LAB_NAME. A second laboratory is never created.
 */
async function ensureLaboratory() {
  const existing = await prisma.laboratory.findFirst({ orderBy: { createdAt: "asc" } });
  if (existing) {
    console.log(`→ Laboratuvar zaten mevcut ('${existing.name}'), oluşturma atlandı.`);
    return existing;
  }
  const lab = await prisma.laboratory.create({ data: { name: config.labName } });
  console.log(`→ Laboratuvar oluşturuldu: '${lab.name}'.`);
  return lab;
}

/**
 * Superadmin bootstrap.
 *
 * - If a SUPERADMIN already exists in the system: does nothing (idempotent).
 * - Otherwise: creates a SUPERADMIN with ADMIN_EMAIL, generates a STRONG
 *   password, and writes it to the startup log **once**. After first login the
 *   password should be changed internally (User management).
 *
 * Internal SaaS: there is no public signup; this is the first and only
 * automatically created user.
 */
async function ensureSuperadmin() {
  const superadminCount = await prisma.user.count({ where: { role: ROLES.SUPERADMIN } });
  if (superadminCount > 0) {
    console.log("→ Superadmin zaten mevcut, bootstrap atlandı.");
    return;
  }

  const email = config.adminEmail;
  const existing = await prisma.user.findUnique({ where: { email } });
  if (existing) {
    console.log(`→ '${email}' zaten kayıtlı ama SUPERADMIN değil. Bootstrap atlandı.`);
    return;
  }

  const password = generateStrongPassword();
  const admin = await prisma.user.create({
    data: {
      email,
      name: "Superadmin",
      role: ROLES.SUPERADMIN,
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

async function main() {
  await ensureLaboratory();
  await ensureSuperadmin();
}

main()
  .then(() => process.exit(0))
  .catch((e) => {
    console.error("Bootstrap hatası:", e);
    process.exit(1);
  });
