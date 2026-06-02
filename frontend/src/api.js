// Simple fetch wrapper, adds the token from localStorage
import { i18n } from "./i18n/index.js";

const TOKEN_KEY = "fl_token";

export const getToken = () => localStorage.getItem(TOKEN_KEY);
export const setToken = (t) => (t ? localStorage.setItem(TOKEN_KEY, t) : localStorage.removeItem(TOKEN_KEY));

// Maps a backend error response to a localized message. The backend returns a
// stable `code` (e.g. "user.emailExists") plus an English `error` fallback; we
// prefer the `errors.<code>` translation and fall back to the server message.
function localizeError(data, status) {
  const { t, te } = i18n.global;
  const code = data?.code;
  if (code && te(`errors.${code}`)) return t(`errors.${code}`, data.details ?? {});
  return data?.error || t("errors.common.requestFailed", { status });
}

export async function api(path, { method = "GET", body } = {}) {
  const headers = { "Content-Type": "application/json" };
  const token = getToken();
  if (token) headers.Authorization = `Bearer ${token}`;
  const res = await fetch(`/api${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  });
  if (res.status === 204) return null;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    const error = new Error(localizeError(data, res.status));
    error.code = data?.code;
    error.status = res.status;
    throw error;
  }
  return data;
}
