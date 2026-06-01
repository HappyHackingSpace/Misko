import "dotenv/config";

/**
 * Merkezi yapılandırma. Ortam değişkenleri burada okunur, doğrulanır ve
 * uygulamanın geri kalanına tek bir nesne olarak sunulur.
 */
function required(name, fallback) {
  const value = process.env[name] ?? fallback;
  if (value === undefined || value === "") {
    throw new Error(`Eksik ortam değişkeni: ${name}`);
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
  // Docker açılışında superadmin bootstrap'i için e-posta (şifreyi sistem üretir)
  adminEmail: process.env.ADMIN_EMAIL || "admin@fare.lab",
};
