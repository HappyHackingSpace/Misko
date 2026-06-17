import { prisma } from "../../lib/prisma.js";
import { ApiError } from "../../utils/ApiError.js";
import { isPrivilegedRole } from "../../config/permissions.js";

/**
 * Comment service - a discussion thread attached to a Test.
 *
 * Authorization model (see docs/DOMAIN.md):
 *   - read / create: any authenticated user (enforced by the router's `authenticate`).
 *   - edit:          author only. Editing someone else's words is never allowed.
 *   - delete:        author, or a privileged role (SUPERADMIN / LAB_MANAGER) for moderation.
 *
 * All persistence goes through Prisma (parameterized queries - no string-built
 * SQL), so the comment body cannot be used for SQL injection. The body is stored
 * as raw text and returned verbatim; output encoding is the renderer's job.
 */

// Author fields exposed with a comment. We never leak the author's email here.
const authorSelect = { select: { id: true, name: true, role: true } };

/** Loads a test by id or throws 404. Keeps comment endpoints from leaking test existence. */
async function ensureTestExists(testId) {
  const test = await prisma.test.findUnique({ where: { id: testId }, select: { id: true } });
  if (!test) throw ApiError.notFound("Test not found", "common.notFound");
}

/** Loads a comment scoped to its test, or throws 404. Prevents cross-test id guessing (IDOR). */
async function getOwnedComment(testId, commentId) {
  const comment = await prisma.comment.findUnique({ where: { id: commentId } });
  if (!comment || comment.testId !== testId) throw ApiError.notFound("Comment not found", "common.notFound");
  return comment;
}

/** Lists all comments on a test, oldest first. */
export async function list(testId) {
  await ensureTestExists(testId);
  return prisma.comment.findMany({
    where: { testId },
    orderBy: { createdAt: "asc" },
    include: { author: authorSelect },
  });
}

/** Creates a comment authored by the logged-in user. */
export async function create(testId, { body }, authorId) {
  await ensureTestExists(testId);
  return prisma.comment.create({
    data: { testId, authorId, body },
    include: { author: authorSelect },
  });
}

/** Updates a comment. Only the original author may edit. */
export async function update(testId, commentId, { body }, user) {
  const comment = await getOwnedComment(testId, commentId);
  if (comment.authorId !== user.id) {
    throw ApiError.forbidden("You can only edit your own comments", "comment.cannotEdit");
  }
  return prisma.comment.update({
    where: { id: commentId },
    data: { body },
    include: { author: authorSelect },
  });
}

/** Deletes a comment. Author or a privileged (moderator) role may delete. */
export async function remove(testId, commentId, user) {
  const comment = await getOwnedComment(testId, commentId);
  const canDelete = comment.authorId === user.id || isPrivilegedRole(user.role);
  if (!canDelete) {
    throw ApiError.forbidden("You cannot delete this comment", "comment.cannotDelete");
  }
  await prisma.comment.delete({ where: { id: commentId } });
}
