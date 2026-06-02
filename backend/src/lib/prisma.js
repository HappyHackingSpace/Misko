import { PrismaClient } from "@prisma/client";
import { PrismaPg } from "@prisma/adapter-pg";
import { config } from "../config/index.js";

/**
 * Singleton Prisma client. Stored on global to avoid opening multiple
 * connections during hot-reload in development.
 *
 * Prisma 7: the connection is now established through a driver adapter (pg);
 * the URL is passed to the adapter at runtime instead of in schema.prisma.
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
