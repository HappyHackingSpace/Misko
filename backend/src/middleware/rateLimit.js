import { ApiError } from "../utils/ApiError.js";

/**
 * Minimal in-memory, per-user sliding-window rate limiter.
 *
 * This guards write endpoints (e.g. posting comments) against spam / abusive
 * automation. Mişko is a single-instance on-prem deployment, so an in-process
 * counter is sufficient; it intentionally avoids an external store. The map is
 * keyed by user id and each user's own bucket is pruned on every request, so
 * memory stays bounded by the number of recently-active users.
 *
 * @param {{ windowMs?: number, max?: number, code?: string, message?: string }} [options]
 */
export function rateLimit({ windowMs = 60_000, max = 20, code = "common.rateLimited", message = "Too many requests, slow down" } = {}) {
  /** @type {Map<string, number[]>} userId -> recent request timestamps */
  const buckets = new Map();

  return (req, _res, next) => {
    // Falls back to the IP if somehow unauthenticated; authed routes always have req.user.
    const key = req.user?.id || req.ip || "anonymous";
    const now = Date.now();
    const recent = (buckets.get(key) || []).filter((ts) => now - ts < windowMs);

    if (recent.length >= max) {
      const retryAfter = Math.ceil((windowMs - (now - recent[0])) / 1000);
      throw ApiError.tooManyRequests(message, code, { retryAfter });
    }

    recent.push(now);
    buckets.set(key, recent);
    next();
  };
}
