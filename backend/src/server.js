import { createApp } from "./app.js";
import { config } from "./config/index.js";
import { disconnectPrisma } from "./lib/prisma.js";

const app = createApp();

const server = app.listen(config.port, () => {
  console.log(`Mişko API → http://localhost:${config.port} [${config.env}]`);
});

async function shutdown(signal) {
  console.log(`\n${signal} alındı, kapanılıyor...`);
  server.close(async () => {
    await disconnectPrisma();
    process.exit(0);
  });
}

process.on("SIGTERM", () => shutdown("SIGTERM"));
process.on("SIGINT", () => shutdown("SIGINT"));
