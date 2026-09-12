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
  groups: (id) => api(`/experiments/${id}/groups`),
  enrollments: (id, query) => api(`/experiments/${id}/enrollments`, { query }),
  tests: (id, query) => api(`/experiments/${id}/tests`, { query }),
};

export const tests = {
  get: (id) => api(`/tests/${id}`),
  start: (id) => api(`/tests/${id}/start`, { method: "POST", body: {} }),
  complete: (id) => api(`/tests/${id}/complete`, { method: "POST", body: {} }),
  trials: (id) => api(`/tests/${id}/trials`),
  comments: (id) => api(`/tests/${id}/comments`),
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
  version: (key, version) => api(`/paradigms/${key}/versions/${version}`),
};

export const reports = {
  metrics: (query) => api("/reports/metrics", { query }),
  events: (query) => api("/reports/events", { query }),
  summary: (query) => api("/reports/metric-summary", { query }),
};
