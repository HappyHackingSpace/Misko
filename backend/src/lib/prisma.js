import { PrismaClient } from "@prisma/client";
import { PrismaPg } from "@prisma/adapter-pg";
import { config } from "../config/index.js";

/**
 * Tekil (singleton) Prisma istemcisi. Geliştirme sırasında hot-reload'da
 * birden fazla bağlantı açılmasını önlemek için global'de saklanır.
 *
 * Prisma 7: bağlantı artık bir driver adapter (pg) üzerinden kurulur; URL
 * schema.prisma yerine çalışma zamanında adapter'a verilir.
 */
const globalForPrisma = globalThis;

const adapter = new PrismaPg({ connectionString: config.databaseUrl });

export const prisma =
  globalForPrisma.__misko_prisma ??
  new PrismaClient({
    adapter,
    log: config.isProd ? ["warn", "error"] : ["query", "warn", "error"],
  });

if (!config.isProd) {
  globalForPrisma.__misko_prisma = prisma;
}

export async function disconnectPrisma() {
  await prisma.$disconnect();
}
