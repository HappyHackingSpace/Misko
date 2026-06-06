import "dotenv/config";
import { prisma } from "../src/lib/prisma.js";

// Sample domain data: one subject. Environments and scenarios are created through
// the app/API (an environment needs a validated apparatus snapshot, and a
// scenario references environments + acceptance), so they are not seeded here.
// The admin user is NOT created here; the superadmin is created at Docker startup
// via `prisma/bootstrap-admin.js`. For a local admin: `npm run db:bootstrap`.
async function main() {
  await prisma.subject.upsert({
    where: { code: "F-001" },
    update: {},
    create: { code: "F-001", sex: "M", groupName: "kontrol" },
  });

  console.log("Seed done (subject F-001). For an admin: npm run db:bootstrap");
}

main().then(() => process.exit(0)).catch((e) => { console.error(e); process.exit(1); });
