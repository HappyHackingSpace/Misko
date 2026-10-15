// Lab-wide catalog: substances, disease models, and the environments (arenas)
// built from paradigm templates.
import { api } from "./client.js";

export const substances = {
  list: (query) => api("/catalog/substances", { query }),
  create: (body) => api("/catalog/substances", { method: "POST", body }),
  update: (id, body) => api(`/catalog/substances/${id}`, { method: "PATCH", body }),
  remove: (id) => api(`/catalog/substances/${id}`, { method: "DELETE" }),
};

export const diseaseModels = {
  list: (query) => api("/catalog/disease-models", { query }),
  create: (body) => api("/catalog/disease-models", { method: "POST", body }),
  update: (id, body) => api(`/catalog/disease-models/${id}`, { method: "PATCH", body }),
  remove: (id) => api(`/catalog/disease-models/${id}`, { method: "DELETE" }),
};

// Paradigm templates are read-only; names and labels come back in the panel's language.
export const paradigmCatalog = {
  list: () => api("/paradigms"),
  version: (key, version) => api(`/paradigms/${key}/versions/${version}`),
};

export const environments = {
  list: (query) => api("/environments", { query }),
  get: (id) => api(`/environments/${id}`),
  create: (body) => api("/environments", { method: "POST", body }),
  update: (id, body) => api(`/environments/${id}`, { method: "PATCH", body }),
  revisions: (id) => api(`/environments/${id}/revisions`),
  addRevision: (id, body) => api(`/environments/${id}/revisions`, { method: "POST", body }),
  // The default calibration of one revision: every animal tested in this arena uses it.
  calibrations: (id, number) => api(`/environments/${id}/revisions/${number}/calibrations`),
  calibrationStatus: (id, number) => api(`/environments/${id}/revisions/${number}/calibration-status`),
  calibrate: (id, number, body) => api(`/environments/${id}/revisions/${number}/calibrations`, { method: "POST", body }),
};

// The `parameters` of a revision: every APPARATUS measurement, plus only the
// ANALYSIS settings that differ from the paradigm default (omitted ones take
// their default when used).
export function revisionParameters(apparatus, analysis, values) {
  const parameters = {};
  for (const parameter of apparatus) parameters[parameter.key] = Number(values[parameter.key]);
  for (const parameter of analysis) {
    const value = Number(values[parameter.key]);
    if (value !== parameter.default) parameters[parameter.key] = value;
  }
  return parameters;
}
