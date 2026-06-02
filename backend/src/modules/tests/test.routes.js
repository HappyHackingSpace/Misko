import { Router } from "express";
import { requirePermission } from "../../middleware/authenticate.js";
import { PERMISSIONS } from "../../config/permissions.js";
import * as testController from "./test.controller.js";

export const testRouter = Router();

testRouter.get("/", testController.list);
testRouter.get("/:id", testController.getById);
// Test oluşturma/silme tasarım yetkisidir; güncelleme (koşma/durum) test:run ister.
testRouter.post("/", requirePermission(PERMISSIONS.TEST_WRITE), testController.create);
testRouter.patch("/:id", requirePermission(PERMISSIONS.TEST_RUN), testController.update);
testRouter.delete("/:id", requirePermission(PERMISSIONS.TEST_WRITE), testController.remove);
