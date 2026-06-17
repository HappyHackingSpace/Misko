/**
 * Application error that carries an HTTP status code. The service/controller
 * layer throws it, and the central error middleware catches it and converts it to a response.
 */
export class ApiError extends Error {
  constructor(statusCode, message, code, details) {
    super(message);
    this.name = "ApiError";
    this.statusCode = statusCode;
    this.code = code;
    this.details = details;
  }

  static badRequest(message = "Bad request", code, details) {
    return new ApiError(400, message, code, details);
  }

  static unauthorized(message = "Unauthorized", code, details) {
    return new ApiError(401, message, code, details);
  }

  static forbidden(message = "Access denied", code, details) {
    return new ApiError(403, message, code, details);
  }

  static notFound(message = "Not found", code, details) {
    return new ApiError(404, message, code, details);
  }

  static conflict(message = "Conflict", code, details) {
    return new ApiError(409, message, code, details);
  }

  static tooManyRequests(message = "Too many requests", code, details) {
    return new ApiError(429, message, code, details);
  }
}
