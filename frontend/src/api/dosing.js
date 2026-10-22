// Dosing of a running study: the doses due per day, recording what was given
// (with the body weight per-kilogram doses need), the dose history and the
// confirmation of each animal's disease model.
import { api } from "./client.js";

const study = (id) => `/experiments/${id}`;

export const dosing = {
  // date is "YYYY-MM-DD"; without it the API uses the laboratory's current date.
  schedule: (experimentId, date) => api(`${study(experimentId)}/dosing`, { query: { date } }),
  history: (experimentId, query) => api(`${study(experimentId)}/administrations`, { query }),
  record: (experimentId, allocationId, body) =>
    api(`${study(experimentId)}/allocations/${allocationId}/administrations`, { method: "POST", body }),
  inductionOutcomes: (experimentId) => api(`${study(experimentId)}/induction-outcomes`),
  recordInductionOutcome: (experimentId, allocationId, body) =>
    api(`${study(experimentId)}/allocations/${allocationId}/induction-outcome`, { method: "POST", body }),
  recordWeight: (subjectId, body) => api(`/subjects/${subjectId}/weights`, { method: "POST", body }),
};

// Read-only design data the dosing screens show next to the doses.
export const studyDesign = {
  groups: (experimentId) => api(`${study(experimentId)}/groups`),
  regimenItems: (experimentId) => api(`${study(experimentId)}/regimen-items`),
  allocations: (experimentId, query) => api(`${study(experimentId)}/allocations`, { query }),
  timepoints: (experimentId) => api(`${study(experimentId)}/timepoints`),
  protocols: (experimentId) => api(`${study(experimentId)}/protocols`),
  protocolVersions: (experimentId, protocolId) => api(`${study(experimentId)}/protocols/${protocolId}/versions`),
  substances: (query) => api("/catalog/substances", { query }),
};
