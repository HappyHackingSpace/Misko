import { asyncHandler } from "../../utils/asyncHandler.js";
import * as testService from "./test.service.js";

export const list = asyncHandler(async (_req, res) => {
  res.json(await testService.list());
});

export const getById = asyncHandler(async (req, res) => {
  res.json(await testService.getById(req.params.id));
});

export const create = asyncHandler(async (req, res) => {
  res.status(201).json(await testService.create(req.body, req.user.id));
});

export const update = asyncHandler(async (req, res) => {
  res.json(await testService.update(req.params.id, req.body));
});

export const remove = asyncHandler(async (req, res) => {
  await testService.remove(req.params.id);
  res.status(204).end();
});
