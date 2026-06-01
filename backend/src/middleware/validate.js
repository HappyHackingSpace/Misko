/**
 * Zod şemasıyla istek gövdesini doğrulayan middleware fabrikası.
 * Doğrulanan (ve dönüştürülen) veriyi req.body'ye yazar.
 */
export const validateBody = (schema) => (req, _res, next) => {
  req.body = schema.parse(req.body ?? {});
  next();
};
