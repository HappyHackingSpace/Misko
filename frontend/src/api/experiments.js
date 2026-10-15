// API calls of the experiment design screens: the experiment and its
// lifecycle, groups, regimen items, timepoints, endpoints and allocations, plus
// the read-only catalog lists their pickers need.
import { api } from "./client.js";

const base = (id) => `/experiments/${id}`;

export const experiments = {
  list: (query) => api("/experiments", { query }),
  get: (id) => api(base(id)),
  create: (body) => api("/experiments", { method: "POST", body }),
  update: (id, body) => api(base(id), { method: "PATCH", body }),
  readiness: (id) => api(`${base(id)}/readiness`),
  start: (id, dayZero) => api(`${base(id)}/start`, { method: "POST", body: { dayZero } }),
  complete: (id) => api(`${base(id)}/complete`, { method: "POST" }),
  metricOptions: (id) => api(`${base(id)}/metric-options`),
};

export const groups = {
  list: (id) => api(`${base(id)}/groups`),
  create: (id, body) => api(`${base(id)}/groups`, { method: "POST", body }),
  update: (id, groupId, body) => api(`${base(id)}/groups/${groupId}`, { method: "PATCH", body }),
  remove: (id, groupId) => api(`${base(id)}/groups/${groupId}`, { method: "DELETE" }),
};

export const regimenItems = {
  list: (id) => api(`${base(id)}/regimen-items`),
  create: (id, body) => api(`${base(id)}/regimen-items`, { method: "POST", body }),
  update: (id, itemId, body) => api(`${base(id)}/regimen-items/${itemId}`, { method: "PATCH", body }),
  remove: (id, itemId) => api(`${base(id)}/regimen-items/${itemId}`, { method: "DELETE" }),
};

export const timepoints = {
  list: (id) => api(`${base(id)}/timepoints`),
  create: (id, body) => api(`${base(id)}/timepoints`, { method: "POST", body }),
  update: (id, timepointId, body) => api(`${base(id)}/timepoints/${timepointId}`, { method: "PATCH", body }),
  remove: (id, timepointId) => api(`${base(id)}/timepoints/${timepointId}`, { method: "DELETE" }),
};

export const endpoints = {
  list: (id) => api(`${base(id)}/endpoints`),
  create: (id, body) => api(`${base(id)}/endpoints`, { method: "POST", body }),
  update: (id, endpointId, body) => api(`${base(id)}/endpoints/${endpointId}`, { method: "PATCH", body }),
  remove: (id, endpointId) => api(`${base(id)}/endpoints/${endpointId}`, { method: "DELETE" }),
};

export const allocations = {
  // { data: allocations, groups: fill of every group }
  list: (id, query) => api(`${base(id)}/allocations`, { query }),
  allocate: (id, body) => api(`${base(id)}/allocations`, { method: "POST", body }),
  randomise: (id, body) => api(`${base(id)}/allocations/randomise`, { method: "POST", body }),
  update: (id, allocationId, body) => api(`${base(id)}/allocations/${allocationId}`, { method: "PATCH", body }),
  remove: (id, allocationId) => api(`${base(id)}/allocations/${allocationId}`, { method: "DELETE" }),
  exclude: (id, allocationId, body) => api(`${base(id)}/allocations/${allocationId}/exclusion`, { method: "POST", body }),
};

// Read-only lists for pickers; the catalog screens own writing them.
export const pickers = {
  substances: () => api("/catalog/substances"),
  diseaseModels: () => api("/catalog/disease-models"),
  // Animals not allocated to any group yet, one page at a time.
  unallocatedSubjects: (query) => api("/subjects", { query: { ...query, unallocated: "true" } }),
};
