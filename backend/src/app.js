import express from "express";
import cors from "cors";
import helmet from "helmet";
import morgan from "morgan";
import { config } from "./config/index.js";
import { apiRouter } from "./routes/index.js";
import { errorHandler } from "./middleware/errorHandler.js";
import { notFound } from "./middleware/notFound.js";

/**
 * Express uygulamasını kurar ve döner (listen burada çağrılmaz —
 * test edilebilirlik için server.js ayrı tutulur).
 */
export function createApp() {
  const app = express();

  app.disable("x-powered-by");
  app.use(helmet());
  app.use(cors({ origin: config.cors.origin }));
  app.use(express.json());
  if (!config.isTest) {
    app.use(morgan(config.isProd ? "combined" : "dev"));
  }

  app.use("/api", apiRouter);

  app.use(notFound);
  app.use(errorHandler);

  return app;
}
