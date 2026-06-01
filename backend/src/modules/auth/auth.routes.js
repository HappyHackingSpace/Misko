import { Router } from "express";
import { authenticate } from "../../middleware/authenticate.js";
import { validateBody } from "../../middleware/validate.js";
import * as authController from "./auth.controller.js";
import { loginSchema } from "./auth.validation.js";

export const authRouter = Router();

// Internal SaaS: public kayıt (signup) yok. Sadece giriş + oturum.
authRouter.post("/login", validateBody(loginSchema), authController.login);
authRouter.get("/me", authenticate, authController.me);
