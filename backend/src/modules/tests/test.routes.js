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
// Per-environment run: start / write metrics / finish one environment of the scenario.
testRouter.patch("/:id/environments/:envId", requirePermission(PERMISSIONS.TEST_RUN), testController.submitEnvironmentResult);
testRouter.delete("/:id", requirePermission(PERMISSIONS.TEST_WRITE), testController.remove);
