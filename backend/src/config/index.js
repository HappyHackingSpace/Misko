import "dotenv/config";

/**
 * Central configuration. Environment variables are read, validated, and
 * exposed to the rest of the application as a single object here.
 */
function required(name, fallback) {
  const value = process.env[name] ?? fallback;
  if (value === undefined || value === "") {
    throw new Error(`Missing environment variable: ${name}`);
  }
  return value;
}

const nodeEnv = process.env.NODE_ENV || "development";
const isProd = nodeEnv === "production";

export const config = {
  env: nodeEnv,
  isProd,
  isTest: nodeEnv === "test",
  port: Number(process.env.PORT || 4000),
  databaseUrl: required("DATABASE_URL", isProd ? undefined : "postgresql://misko:misko@localhost:5432/misko?schema=public"),
  jwt: {
    secret: required("JWT_SECRET", isProd ? undefined : "dev-secret-change-me"),
    ttl: process.env.JWT_TTL || "7d",
  },
  cors: {
    origin: process.env.CORS_ORIGIN || "*",
  },
  bcryptRounds: Number(process.env.BCRYPT_ROUNDS || 10),
  // Email for the superadmin bootstrap at Docker startup (the system generates the password)
  adminEmail: process.env.ADMIN_EMAIL || "admin@fare.lab",
  // Laboratory name for the single-tenant (on-prem) install. The setup wizard
  // (bootstrap-admin.js) creates the Laboratory singleton with this; it is also
  // the fallback name for /api/meta branding when the singleton does not exist yet.
  labName: process.env.LAB_NAME || "Mişko Laboratuvarı",
};
