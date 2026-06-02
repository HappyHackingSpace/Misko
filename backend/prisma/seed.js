import "dotenv/config";
import { prisma } from "../src/lib/prisma.js";

// Sample domain data: 4 scenarios + sample subject.
// Note: the admin user is NOT created here; the superadmin is created at Docker
// startup via `prisma/bootstrap-admin.js` (with a strong, generated password).
// For a local admin: `npm run db:bootstrap`.
async function main() {
  const scenarios = [
    { name: "Havuz", type: "POOL", description: "Su labirenti — platform bulma" },
    { name: "Labirent", type: "MAZE", description: "Labirent geçişi" },
    { name: "Sopa", type: "STICK", description: "Denge/koordinasyon" },
    { name: "Yol", type: "PATH", description: "Yol/koşu testi" },
  ];
  for (const s of scenarios) {
    const exists = await prisma.scenario.findFirst({ where: { name: s.name } });
    if (!exists) await prisma.scenario.create({ data: s });
  }

  await prisma.subject.upsert({
    where: { code: "F-001" },
    update: {},
    create: { code: "F-001", sex: "M", groupName: "kontrol" },
  });

  console.log("Seed tamam (4 senaryo + denek F-001). Admin için: npm run db:bootstrap");
}

main().then(() => process.exit(0)).catch((e) => { console.error(e); process.exit(1); });
