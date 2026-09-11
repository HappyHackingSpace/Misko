// Fetch wrapper for the Go API: adds the session token, unwraps JSON and turns
// the API's stable error `code` into a localized message.
import { i18n } from "../i18n/index.js";

const TOKEN_KEY = "misko_token";

export const getToken = () => localStorage.getItem(TOKEN_KEY);
export const setToken = (token) => (token ? localStorage.setItem(TOKEN_KEY, token) : localStorage.removeItem(TOKEN_KEY));

export class ApiError extends Error {
  constructor(message, { code, status }) {
    super(message);
    this.code = code;
    this.status = status;
  }
}

function localize(data, status) {
  const { t, te } = i18n.global;
  const code = data?.code;
  if (code && te(`errors.${code}`)) return t(`errors.${code}`);
  return data?.error || t("errors.common.requestFailed", { status });
}

/**
 * Calls the API. `path` starts with "/" and is appended to /api.
 * Query values that are null, undefined or "" are dropped.
 */
export async function api(path, { method = "GET", body, query, signal } = {}) {
  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(query || {})) {
    if (value !== null && value !== undefined && value !== "") search.set(key, value);
  }
  const token = getToken();
  const response = await fetch(`/api${path}${search.size ? `?${search}` : ""}`, {
    method,
    headers: { "Content-Type": "application/json", ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body),
    signal,
  });
  if (response.status === 204) return null;
  const isJson = response.headers.get("content-type")?.includes("application/json");
  const data = isJson ? await response.json().catch(() => ({})) : {};
  if (!response.ok) {
    // An expired or revoked session sends the caller back to the login screen.
    if (response.status === 401 && !path.startsWith("/auth/login")) {
      setToken(null);
      if (!window.location.pathname.startsWith("/login")) window.location.assign("/login");
    }
    throw new ApiError(localize(data, response.status), { code: data?.code, status: response.status });
  }
  return data;
}
