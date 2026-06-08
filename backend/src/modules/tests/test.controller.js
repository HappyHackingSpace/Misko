import { asyncHandler } from "../../utils/asyncHandler.js";
import * as testService from "./test.service.js";

export const list = asyncHandler(async (req, res) => {
  res.json(await testService.list(req.query));
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

export const submitEnvironmentResult = asyncHandler(async (req, res) => {
  res.json(await testService.submitEnvironmentResult(req.params.id, req.params.envId, req.body ?? {}));
});

export const addEnvironmentEvent = asyncHandler(async (req, res) => {
  res.status(201).json(await testService.addEnvironmentEvent(req.params.id, req.params.envId, req.body ?? {}));
});

export const removeEnvironmentEvent = asyncHandler(async (req, res) => {
  res.json(await testService.removeEnvironmentEvent(req.params.id, req.params.envId, Number(req.params.index)));
});

export const remove = asyncHandler(async (req, res) => {
  await testService.remove(req.params.id);
  res.status(204).end();
});
