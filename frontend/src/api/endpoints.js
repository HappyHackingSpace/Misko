// One place for the API paths the panel uses, so screens read as workflow steps.
import { api } from "./client.js";

export const auth = {
  login: (email, password) => api("/auth/login", { method: "POST", body: { email, password } }),
  me: () => api("/auth/me"),
};

export const meta = {
  load: () => api("/meta"),
};

export const subjects = {
  list: (query) => api("/subjects", { query }),
  get: (id) => api(`/subjects/${id}`),
  create: (body) => api("/subjects", { method: "POST", body }),
  update: (id, body) => api(`/subjects/${id}`, { method: "PATCH", body }),
};

export const experiments = {
  list: (query) => api("/experiments", { query }),
  get: (id) => api(`/experiments/${id}`),
  create: (body) => api("/experiments", { method: "POST", body }),
  update: (id, body) => api(`/experiments/${id}`, { method: "PATCH", body }),
  groups: (id) => api(`/experiments/${id}/groups`),
  addGroup: (id, body) => api(`/experiments/${id}/groups`, { method: "POST", body }),
  enrollments: (id, query) => api(`/experiments/${id}/enrollments`, { query }),
  enroll: (id, body) => api(`/experiments/${id}/enrollments`, { method: "POST", body }),
  tests: (id, query) => api(`/experiments/${id}/tests`, { query }),
  planTest: (id, body) => api(`/experiments/${id}/tests`, { method: "POST", body }),
};

export const tests = {
  get: (id) => api(`/tests/${id}`),
  // An empty body means "now"; the API fills the instant in.
  start: (id, body = {}) => api(`/tests/${id}/start`, { method: "POST", body }),
  complete: (id, body = {}) => api(`/tests/${id}/complete`, { method: "POST", body }),
  cancel: (id, reason) => api(`/tests/${id}/cancel`, { method: "POST", body: { reason } }),
  trials: (id) => api(`/tests/${id}/trials`),
  recordTrial: (id, body) => api(`/tests/${id}/trials`, { method: "POST", body }),
  comments: (id) => api(`/tests/${id}/comments`),
  createComment: (id, body) => api(`/tests/${id}/comments`, { method: "POST", body: { body } }),
  updateComment: (id, commentId, body) => api(`/tests/${id}/comments/${commentId}`, { method: "PATCH", body: { body } }),
  deleteComment: (id, commentId) => api(`/tests/${id}/comments/${commentId}`, { method: "DELETE" }),
};

export const recordings = {
  list: (testId) => api(`/tests/${testId}/recordings`),
  // Declares the upload and returns { recording, upload }.
  start: (testId, body) => api(`/tests/${testId}/recordings`, { method: "POST", body }),
  finalize: (testId, recordingId) => api(`/tests/${testId}/recordings/${recordingId}/finalize`, { method: "POST", body: {} }),
  readUrl: (testId, recordingId) => api(`/tests/${testId}/recordings/${recordingId}/read-url`),
};

export const calibration = {
  status: (testId, recordingId) => api(`/tests/${testId}/recordings/${recordingId}/calibration-status`),
  list: (testId, recordingId) => api(`/tests/${testId}/recordings/${recordingId}/calibrations`),
  create: (testId, recordingId, body) => api(`/tests/${testId}/recordings/${recordingId}/calibrations`, { method: "POST", body }),
};

export const analysis = {
  runsOfTest: (testId) => api(`/tests/${testId}/analysis-runs`),
  run: (runId) => api(`/analysis-runs/${runId}`),
  videoPair: (runId) => api(`/analysis-runs/${runId}/video-pair`),
  reanalyze: (testId, recordingId) => api(`/tests/${testId}/recordings/${recordingId}/analysis-runs`, { method: "POST", body: {} }),
  capabilities: () => api("/analysis/capabilities"),
};

export const paradigms = {
  list: () => api("/paradigms"),
  version: (key, version) => api(`/paradigms/${key}/versions/${version}`),
};

export const protocols = {
  list: (experimentId) => api(`/experiments/${experimentId}/protocols`),
  get: (experimentId, protocolId) => api(`/experiments/${experimentId}/protocols/${protocolId}`),
  create: (experimentId, body) => api(`/experiments/${experimentId}/protocols`, { method: "POST", body }),
  versions: (experimentId, protocolId) => api(`/experiments/${experimentId}/protocols/${protocolId}/versions`),
  addVersion: (experimentId, protocolId, body) =>
    api(`/experiments/${experimentId}/protocols/${protocolId}/versions`, { method: "POST", body }),
};

export const environments = {
  list: () => api("/environments"),
  get: (id) => api(`/environments/${id}`),
  create: (body) => api("/environments", { method: "POST", body }),
  revisions: (id) => api(`/environments/${id}/revisions`),
  addRevision: (id, body) => api(`/environments/${id}/revisions`, { method: "POST", body }),
};

export const reports = {
  metrics: (query) => api("/reports/metrics", { query }),
  events: (query) => api("/reports/events", { query }),
  summary: (query) => api("/reports/metric-summary", { query }),
  // The export returns every matching row rather than the page on screen, and
  // the session token travels in a header, so it is fetched rather than linked.
  exportPath: (kind, query) => {
    const search = new URLSearchParams();
    for (const [key, value] of Object.entries(query || {})) {
      if (value !== null && value !== undefined && value !== "") search.set(key, value);
    }
    return `/reports/${kind}/export${search.size ? `?${search}` : ""}`;
  },
};

export const dashboard = {
  summary: () => api("/dashboard/summary"),
};
