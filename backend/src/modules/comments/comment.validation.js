import { z } from "zod";

/** Maximum stored comment length (characters). Guards against oversized payloads. */
export const MAX_COMMENT_LENGTH = 2000;

// C0/C1 control characters that have no place in a text comment. We deliberately
// keep tab (\x09), line feed (\x0A) and carriage return (\x0D) so multi-line
// messages survive. Stripping the rest neutralizes NUL bytes (which PostgreSQL
// rejects in `text`) and other control-character injection before the value is
// ever stored.
// eslint-disable-next-line no-control-regex
const CONTROL_CHARS = /[\x00-\x08\x0B\x0C\x0E-\x1F\x7F-\x9F]/g;

function stripControlChars(value) {
  return value.replace(CONTROL_CHARS, "");
}

// A comment body: a non-empty, length-bounded string. We strip control chars and
// trim first, THEN enforce the length bounds, so the stored value is exactly what
// is validated. Output encoding (XSS defense) is the renderer's job - the value is
// stored as raw text and never interpreted as HTML.
const body = z
  .string({ required_error: "Comment is required", invalid_type_error: "Comment must be text" })
  .transform((s) => stripControlChars(s).trim())
  .pipe(
    z
      .string()
      .min(1, "Comment cannot be empty")
      .max(MAX_COMMENT_LENGTH, `Comment is too long (max ${MAX_COMMENT_LENGTH} characters)`),
  );

// `.strict()` rejects unknown keys, so a client cannot smuggle extra fields
// (e.g. authorId, createdAt) into the create/update payload (mass-assignment guard).
export const createCommentSchema = z.object({ body }).strict();

export const updateCommentSchema = z.object({ body }).strict();
