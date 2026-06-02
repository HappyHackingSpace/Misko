import { Router } from "express";
import { requirePermission } from "../../middleware/authenticate.js";
import { PERMISSIONS } from "../../config/permissions.js";
import { validateBody } from "../../middleware/validate.js";
import * as userController from "./user.controller.js";
import { createUserSchema, resetPasswordSchema, updateUserSchema } from "./user.validation.js";

// All user management depends on the `user:manage` permission (SUPERADMIN/LAB_MANAGER).
// The router is authenticated at the top level.
export const userRouter = Router();

userRouter.use(requirePermission(PERMISSIONS.USER_MANAGE));

userRouter.get("/", userController.list);
userRouter.post("/", validateBody(createUserSchema), userController.create);
userRouter.patch("/:id", validateBody(updateUserSchema), userController.update);
userRouter.post("/:id/reset-password", validateBody(resetPasswordSchema), userController.resetPassword);
userRouter.delete("/:id", userController.remove);
