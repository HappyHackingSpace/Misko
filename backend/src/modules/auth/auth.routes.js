import { Router } from "express";
import { authenticate } from "../../middleware/authenticate.js";
import { validateBody } from "../../middleware/validate.js";
import * as authController from "./auth.controller.js";
import { loginSchema } from "./auth.validation.js";

export const authRouter = Router();

// Internal SaaS: no public signup. Login + session only.
authRouter.post("/login", validateBody(loginSchema), authController.login);
authRouter.get("/me", authenticate, authController.me);
