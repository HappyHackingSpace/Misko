import "dotenv/config";
import { defineConfig } from "prisma/config";

// Prisma 7: bağlantı URL'i artık schema.prisma'da değil burada (Migrate/CLI için).
// Çalışma zamanında PrismaClient bir driver adapter ile beslenir (src/lib/prisma.js).
export default defineConfig({
  schema: "prisma/schema.prisma",
  migrations: {
    path: "prisma/migrations",
  },
  datasource: {
    url: process.env.DATABASE_URL,
  },
});
