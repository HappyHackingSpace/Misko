import "dotenv/config";
import { prisma } from "../src/lib/prisma.js";

// Sample domain data: one subject. Environments and scenarios are created through
// the app/API (an environment needs a validated apparatus snapshot, and a
// scenario references environments + acceptance), so they are not seeded here.
// The admin user is NOT created here; the superadmin is created at Docker startup
// via `prisma/bootstrap-admin.js`. For a local admin: `npm run db:bootstrap`.
async function main() {
  const subjects = [
    { code: "F-001", sex: "M", groupName: "kontrol" },
    { code: "F-002", sex: "M", groupName: "kontrol" },
    { code: "F-003", sex: "F", groupName: "deney" },
    { code: "F-004", sex: "F", groupName: "deney" },
  ];

  for (const s of subjects) {
    await prisma.subject.upsert({
      where: { code: s.code },
      update: {},
      create: s,
    });
  }

  console.log("Seed done (4 subjects). For an admin: npm run db:bootstrap");
}

main().then(() => process.exit(0)).catch((e) => { console.error(e); process.exit(1); });
