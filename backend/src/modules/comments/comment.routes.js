import { Router } from "express";
import { validateBody } from "../../middleware/validate.js";
import { rateLimit } from "../../middleware/rateLimit.js";
import { createCommentSchema, updateCommentSchema } from "./comment.validation.js";
import * as commentController from "./comment.controller.js";

// Mounted under `/tests/:testId/comments` behind `authenticate` (see routes/index.js).
// `mergeParams` lets the controller read :testId from the parent path.
export const commentRouter = Router({ mergeParams: true });

// Limit how fast a single user can post/edit comments (anti-spam / light DoS guard).
const writeLimiter = rateLimit({ windowMs: 60_000, max: 20, code: "comment.rateLimited" });

// Read + create are open to any authenticated user ("everyone can comment").
commentRouter.get("/", commentController.list);
commentRouter.post("/", writeLimiter, validateBody(createCommentSchema), commentController.create);

// Edit (author only) and delete (author or moderator) are authorized in the service.
commentRouter.patch("/:commentId", writeLimiter, validateBody(updateCommentSchema), commentController.update);
commentRouter.delete("/:commentId", commentController.remove);
