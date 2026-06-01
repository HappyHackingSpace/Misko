import { Router } from "express";
import { authenticate } from "../middleware/authenticate.js";
import { authRouter } from "../modules/auth/auth.routes.js";
import { userRouter } from "../modules/users/user.routes.js";
import { deviceRouter } from "../modules/devices/device.routes.js";
import { subjectRouter } from "../modules/subjects/subject.routes.js";
import { scenarioRouter } from "../modules/scenarios/scenario.routes.js";
import { testRouter } from "../modules/tests/test.routes.js";

export const apiRouter = Router();

apiRouter.get("/health", (_req, res) => res.json({ ok: true, ts: Date.now() }));

apiRouter.use("/auth", authRouter);

// Korumalı kaynaklar
apiRouter.use("/users", authenticate, userRouter);
apiRouter.use("/devices", authenticate, deviceRouter);
apiRouter.use("/subjects", authenticate, subjectRouter);
apiRouter.use("/scenarios", authenticate, scenarioRouter);
apiRouter.use("/tests", authenticate, testRouter);
