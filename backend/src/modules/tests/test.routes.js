import { Router } from "express";
import * as testController from "./test.controller.js";

export const testRouter = Router();

testRouter.get("/", testController.list);
testRouter.get("/:id", testController.getById);
testRouter.post("/", testController.create);
testRouter.patch("/:id", testController.update);
testRouter.delete("/:id", testController.remove);
