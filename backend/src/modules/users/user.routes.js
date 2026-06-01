import { Router } from "express";
import { requireRole } from "../../middleware/authenticate.js";
import { validateBody } from "../../middleware/validate.js";
import * as userController from "./user.controller.js";
import { createUserSchema, resetPasswordSchema, updateUserSchema } from "./user.validation.js";

// Tüm kullanıcı yönetimi yalnızca ADMIN'e açıktır (router üst seviyede authenticate edilir).
export const userRouter = Router();

userRouter.use(requireRole("ADMIN"));

userRouter.get("/", userController.list);
userRouter.post("/", validateBody(createUserSchema), userController.create);
userRouter.patch("/:id", validateBody(updateUserSchema), userController.update);
userRouter.post("/:id/reset-password", validateBody(resetPasswordSchema), userController.resetPassword);
userRouter.delete("/:id", userController.remove);
