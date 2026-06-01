/**
 * HTTP durum kodu taşıyan uygulama hatası. Servis/controller katmanı bunu
 * fırlatır, merkezi hata middleware'i yakalayıp yanıta çevirir.
 */
export class ApiError extends Error {
  constructor(statusCode, message, details) {
    super(message);
    this.name = "ApiError";
    this.statusCode = statusCode;
    this.details = details;
  }

  static badRequest(message = "Geçersiz istek", details) {
    return new ApiError(400, message, details);
  }

  static unauthorized(message = "Yetkisiz") {
    return new ApiError(401, message);
  }

  static forbidden(message = "Erişim reddedildi") {
    return new ApiError(403, message);
  }

  static notFound(message = "Bulunamadı") {
    return new ApiError(404, message);
  }

  static conflict(message = "Çakışma") {
    return new ApiError(409, message);
  }
}
