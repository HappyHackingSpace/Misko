import crypto from "node:crypto";

// Karışıklık yaratan karakterler (0/O, 1/l/I) çıkarıldı.
const LOWER = "abcdefghijkmnpqrstuvwxyz";
const UPPER = "ABCDEFGHJKLMNPQRSTUVWXYZ";
const DIGIT = "23456789";
const SYMBOL = "!@#$%^&*-_=+";
const ALL = LOWER + UPPER + DIGIT + SYMBOL;

const pick = (set) => set[crypto.randomInt(set.length)];

/**
 * Kriptografik olarak güçlü, okunabilir bir parola üretir.
 * En az bir küçük/büyük harf, rakam ve sembol içerir.
 */
export function generateStrongPassword(length = 20) {
  const required = [pick(LOWER), pick(UPPER), pick(DIGIT), pick(SYMBOL)];
  const rest = Array.from({ length: Math.max(length, 12) - required.length }, () => pick(ALL));
  const chars = [...required, ...rest];

  // Fisher–Yates karıştırma (zorunlu karakterlerin başta kümelenmemesi için)
  for (let i = chars.length - 1; i > 0; i--) {
    const j = crypto.randomInt(i + 1);
    [chars[i], chars[j]] = [chars[j], chars[i]];
  }
  return chars.join("");
}
