import { asyncHandler } from "../../utils/asyncHandler.js";
import * as commentService from "./comment.service.js";

// `testId` comes from the parent route (`/tests/:testId/comments`), which is why
// the router is created with `{ mergeParams: true }`.

export const list = asyncHandler(async (req, res) => {
  res.json(await commentService.list(req.params.testId));
});

export const create = asyncHandler(async (req, res) => {
  res.status(201).json(await commentService.create(req.params.testId, req.body, req.user.id));
});

export const update = asyncHandler(async (req, res) => {
  res.json(await commentService.update(req.params.testId, req.params.commentId, req.body, req.user));
});

export const remove = asyncHandler(async (req, res) => {
  await commentService.remove(req.params.testId, req.params.commentId, req.user);
  res.status(204).end();
});
