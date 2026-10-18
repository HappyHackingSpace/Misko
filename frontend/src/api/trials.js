// Paradigm trials: one paradigm version tried on one video in one arena,
// outside any experiment. The trial's video goes straight to storage (see
// upload.js); its analyses are jobs of the analysis service (see analysis.js).
import { api } from "./client.js";
import { sendVideo } from "./upload.js";

export const trials = {
  // { search, paradigmKey, page, pageSize } -> { data, total, page, pageSize }
  list: (query) => api("/trials", { query }),
  get: (id) => api(`/trials/${id}`),
  // { name, paradigmKey, paradigmVersion, environmentRevisionId, notes }
  create: (body) => api("/trials", { method: "POST", body }),
  remove: (id) => api(`/trials/${id}`, { method: "DELETE" }),
  videoUrl: (id) => api(`/trials/${id}/video/read-url`),
  // A new analysis of the current video; overrides name ANALYSIS parameters only.
  analyse: (id, overrides) => api(`/trials/${id}/analyses`, { method: "POST", body: { overrides } }),
};

/**
 * Declares, uploads and verifies the trial's video. Once verified the trial is
 * analysed automatically when its calibration allows it. Resolves to the trial
 * detail.
 */
export const uploadTrialVideo = (id, file, progress = {}) =>
  sendVideo(file, {
    ...progress,
    declare: (body) => api(`/trials/${id}/video`, { method: "POST", body }),
    verify: () => api(`/trials/${id}/video/finalize`, { method: "POST", body: {} }),
  });

// The trial's own calibrations on its videos; they never change the arena's default.
export const trialCalibration = {
  status: (id) => api(`/trials/${id}/calibration-status`),
  list: (id) => api(`/trials/${id}/calibrations`),
  create: (id, body) => api(`/trials/${id}/calibrations`, { method: "POST", body }),
};
