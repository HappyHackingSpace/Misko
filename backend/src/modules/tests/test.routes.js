import { Router } from "express";
import { requirePermission } from "../../middleware/authenticate.js";
import { PERMISSIONS } from "../../config/permissions.js";
import * as testController from "./test.controller.js";

export const testRouter = Router();

testRouter.get("/", testController.list);
testRouter.get("/:id", testController.getById);
// Creating/deleting a test is a design permission; updating (running/status) requires test:run.
testRouter.post("/", requirePermission(PERMISSIONS.TEST_WRITE), testController.create);
testRouter.patch("/:id", requirePermission(PERMISSIONS.TEST_RUN), testController.update);
// Per-environment run: start / finish one environment, and log/remove its events.
testRouter.patch("/:id/environments/:envId", requirePermission(PERMISSIONS.TEST_RUN), testController.submitEnvironmentResult);
testRouter.post("/:id/environments/:envId/events", requirePermission(PERMISSIONS.TEST_RUN), testController.addEnvironmentEvent);
testRouter.delete("/:id/environments/:envId/events/:index", requirePermission(PERMISSIONS.TEST_RUN), testController.removeEnvironmentEvent);
testRouter.delete("/:id", requirePermission(PERMISSIONS.TEST_WRITE), testController.remove);
