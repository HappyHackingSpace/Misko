import { asyncHandler } from "../../utils/asyncHandler.js";
import * as userService from "./user.service.js";

export const list = asyncHandler(async (req, res) => {
  res.json(await userService.list(req.query));
});

export const getById = asyncHandler(async (req, res) => {
  res.json(await userService.getById(req.params.id));
});

export const create = asyncHandler(async (req, res) => {
  res.status(201).json(await userService.create(req.body));
});

export const update = asyncHandler(async (req, res) => {
  res.json(await userService.update(req.params.id, req.body));
});

export const resetPassword = asyncHandler(async (req, res) => {
  res.json(await userService.resetPassword(req.params.id, req.body.password));
});

export const remove = asyncHandler(async (req, res) => {
  await userService.remove(req.params.id, req.user.id);
  res.status(204).end();
});
