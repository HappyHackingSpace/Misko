import crypto from "node:crypto";

// Confusable characters (0/O, 1/l/I) are removed.
const LOWER = "abcdefghijkmnpqrstuvwxyz";
const UPPER = "ABCDEFGHJKLMNPQRSTUVWXYZ";
const DIGIT = "23456789";
const SYMBOL = "!@#$%^&*-_=+";
const ALL = LOWER + UPPER + DIGIT + SYMBOL;

const pick = (set) => set[crypto.randomInt(set.length)];

/**
 * Generates a cryptographically strong, readable password.
 * Contains at least one lowercase/uppercase letter, digit, and symbol.
 */
export function generateStrongPassword(length = 20) {
  const required = [pick(LOWER), pick(UPPER), pick(DIGIT), pick(SYMBOL)];
  const rest = Array.from({ length: Math.max(length, 12) - required.length }, () => pick(ALL));
  const chars = [...required, ...rest];

  // Fisher-Yates shuffle (so the required characters do not cluster at the start)
  for (let i = chars.length - 1; i > 0; i--) {
    const j = crypto.randomInt(i + 1);
    [chars[i], chars[j]] = [chars[j], chars[i]];
  }
  return chars.join("");
}
