// API calls for the test protocols of an experiment and the paradigm catalog
// their steps are built from.
import { api } from "./client.js";

const base = (id) => `/experiments/${id}/protocols`;

export const protocols = {
  list: (id) => api(base(id)),
  create: (id, body) => api(base(id), { method: "POST", body }),
  update: (id, protocolId, body) => api(`${base(id)}/${protocolId}`, { method: "PATCH", body }),
  versions: (id, protocolId) => api(`${base(id)}/${protocolId}/versions`),
  addVersion: (id, protocolId, body) => api(`${base(id)}/${protocolId}/versions`, { method: "POST", body }),
};

export const paradigmCatalog = {
  list: (lang) => api("/paradigms", { query: { lang } }),
  version: (key, version, lang) => api(`/paradigms/${key}/versions/${version}`, { query: { lang } }),
};
