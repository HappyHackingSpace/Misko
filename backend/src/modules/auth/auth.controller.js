import { asyncHandler } from "../../utils/asyncHandler.js";
import * as authService from "./auth.service.js";

export const login = asyncHandler(async (req, res) => {
  res.json(await authService.login(req.body));
});

export const me = (req, res) => {
  res.json({ user: req.user });
};
