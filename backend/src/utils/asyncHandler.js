/**
 * Async route handler sarmalayıcısı: reddedilen promise'leri Express'in
 * hata zincirine (next) yönlendirir, böylece her handler'da try/catch gerekmez.
 */
export const asyncHandler = (fn) => (req, res, next) =>
  Promise.resolve(fn(req, res, next)).catch(next);
