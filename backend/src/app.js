import express from "express";
import cors from "cors";
import helmet from "helmet";
import morgan from "morgan";
import { config } from "./config/index.js";
import { apiRouter } from "./routes/index.js";
import { errorHandler } from "./middleware/errorHandler.js";
import { notFound } from "./middleware/notFound.js";

/**
 * Builds and returns the Express app (listen is not called here;
 * server.js is kept separate for testability).
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
