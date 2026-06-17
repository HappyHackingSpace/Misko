import { Router } from "express";
import { config } from "../config/index.js";
import { authenticate } from "../middleware/authenticate.js";
import { authRouter } from "../modules/auth/auth.routes.js";
import { userRouter } from "../modules/users/user.routes.js";
import { subjectRouter } from "../modules/subjects/subject.routes.js";
import { scenarioRouter } from "../modules/scenarios/scenario.routes.js";
import { testRouter } from "../modules/tests/test.routes.js";
import { commentRouter } from "../modules/comments/comment.routes.js";
import { paradigmRouter } from "../modules/paradigms/paradigm.routes.js";
import { environmentRouter } from "../modules/environments/environment.routes.js";
import { labRouter } from "../modules/lab/lab.routes.js";
import { prisma } from "../lib/prisma.js";

export const apiRouter = Router();

apiRouter.get("/health", (_req, res) => res.json({ ok: true, ts: Date.now() }));

// Public branding info (also shown on the login screen). If the Laboratory singleton
// is set up, the name is read from there; if setup has not run yet, it falls back to config.labName.
apiRouter.get("/meta", async (_req, res, next) => {
  try {
    const lab = await prisma.laboratory.findFirst({ orderBy: { createdAt: "asc" } });
    res.json({ appName: "Mişko", labName: lab?.name ?? config.labName });
  } catch (err) {
    next(err);
  }
});

apiRouter.use("/auth", authRouter);

// Protected resources
apiRouter.use("/users", authenticate, userRouter);
apiRouter.use("/lab", authenticate, labRouter);
apiRouter.use("/subjects", authenticate, subjectRouter);
apiRouter.use("/scenarios", authenticate, scenarioRouter);
apiRouter.use("/tests", authenticate, testRouter);
apiRouter.use("/tests/:testId/comments", authenticate, commentRouter);
apiRouter.use("/paradigms", authenticate, paradigmRouter);
apiRouter.use("/environments", authenticate, environmentRouter);
