/**
 * Async route handler wrapper: forwards rejected promises to Express's
 * error chain (next), so no try/catch is needed in each handler.
 */
export const asyncHandler = (fn) => (req, res, next) =>
  Promise.resolve(fn(req, res, next)).catch(next);
