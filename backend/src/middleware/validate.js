/**
 * Middleware factory that validates the request body against a Zod schema.
 * Writes the validated (and transformed) data back to req.body.
 */
export const validateBody = (schema) => (req, _res, next) => {
  req.body = schema.parse(req.body ?? {});
  next();
};
