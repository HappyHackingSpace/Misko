// Test sessions: planning them with their animals and arenas, running them
// (start, trials per animal, complete or cancel) and their comments.
import { api } from "./client.js";

export const tests = {
  ofExperiment: (experimentId, query) => api(`/experiments/${experimentId}/tests`, { query }),
  worklist: (query) => api("/tests", { query }),
  get: (id) => api(`/tests/${id}`),
  plan: (experimentId, body) => api(`/experiments/${experimentId}/tests`, { method: "POST", body }),
  addSubject: (id, body) => api(`/tests/${id}/subjects`, { method: "POST", body }),
  removeSubject: (id, testSubjectId) => api(`/tests/${id}/subjects/${testSubjectId}`, { method: "DELETE" }),
  // An empty body means "now"; the API fills the instant in.
  start: (id) => api(`/tests/${id}/start`, { method: "POST", body: {} }),
  complete: (id) => api(`/tests/${id}/complete`, { method: "POST", body: {} }),
  cancel: (id, reason) => api(`/tests/${id}/cancel`, { method: "POST", body: { reason } }),
  recordTrial: (id, testSubjectId, body) => api(`/tests/${id}/subjects/${testSubjectId}/trials`, { method: "POST", body }),
  comments: (id) => api(`/tests/${id}/comments`),
  createComment: (id, body) => api(`/tests/${id}/comments`, { method: "POST", body: { body } }),
  updateComment: (id, commentId, body) => api(`/tests/${id}/comments/${commentId}`, { method: "PATCH", body: { body } }),
  deleteComment: (id, commentId) => api(`/tests/${id}/comments/${commentId}`, { method: "DELETE" }),
};

// Lookups the session screens need: arenas, paradigm names and the lab's clock.
export const lookups = {
  environmentRevisions: () => api("/environment-revisions"),
  paradigms: () => api("/paradigms"),
  experiments: (query) => api("/experiments", { query }),
  lab: () => api("/lab"),
};
