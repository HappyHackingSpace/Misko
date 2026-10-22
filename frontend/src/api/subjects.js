// Laboratory animals and their body weights. Allocation to a group is read-only
// here; it is managed per experiment.
import { api } from "./client.js";

export const subjects = {
  list: (query) => api("/subjects", { query }),
  get: (id) => api(`/subjects/${id}`),
  create: (body) => api("/subjects", { method: "POST", body }),
  update: (id, body) => api(`/subjects/${id}`, { method: "PATCH", body }),
  remove: (id) => api(`/subjects/${id}`, { method: "DELETE" }),
  weights: (id) => api(`/subjects/${id}/weights`),
  recordWeight: (id, body) => api(`/subjects/${id}/weights`, { method: "POST", body }),
};
