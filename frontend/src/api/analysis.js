// API calls of the analysis area: analysis jobs, the runs of an animal, human
// review of results, the study report and the dashboard summary.
import { api, ApiError, getToken } from "./client.js";
import { i18n } from "../i18n/index.js";

// The analyses of the animals of a test, as the experiment side links them.
// A run id is the id of the analysis job.
export const runs = {
  // Runs of a test, newest first; with testSubjectId only that animal's runs.
  ofTest: (testId, testSubjectId) => api(`/tests/${testId}/analysis-runs`, { query: { testSubjectId } }),
  // A manual run of one animal with the current video and calibration.
  reanalyze: (testSubjectId) => api(`/analysis/test-subjects/${testSubjectId}/runs`, { method: "POST", body: {} }),
};

// Jobs of the analysis service, read the same way by the experiment screens
// and by paradigm trials.
export const jobs = {
  // The job with what it published (metrics, events, artifacts, QC) once it succeeded.
  get: (jobId) => api(`/analysis/jobs/${jobId}`),
  // Signed reads of the original and the analysed video on one timeline.
  videoPair: (jobId) => api(`/analysis/jobs/${jobId}/video-pair`),
  // A signed read of one output: ANALYZED_VIDEO, TRAJECTORY or THUMBNAIL.
  outputUrl: (jobId, kind) => api(`/analysis/jobs/${jobId}/outputs/${kind}/read-url`),
};

export const results = {
  // { experimentId, status: PENDING|ACCEPTED|REJECTED|ALL, page, pageSize }
  list: (query) => api("/analysis/results", { query }),
  // One animal's result with every succeeded run a reviewer can choose.
  get: (testSubjectId) => api(`/analysis/results/${testSubjectId}`),
  accept: (testSubjectId, note) => api(`/analysis/results/${testSubjectId}/accept`, { method: "POST", body: note ? { note } : {} }),
  reject: (testSubjectId, reason, note) =>
    api(`/analysis/results/${testSubjectId}/reject`, { method: "POST", body: note ? { reason, note } : { reason } }),
  chooseRun: (testSubjectId, runId) => api(`/analysis/results/${testSubjectId}/run`, { method: "POST", body: { runId } }),
  bulkAccept: (testSubjectIds, note) =>
    api("/analysis/results/bulk-accept", { method: "POST", body: note ? { testSubjectIds, note } : { testSubjectIds } }),
};

export const report = {
  get: (experimentId, endpointId) => api(`/experiments/${experimentId}/report`, { query: { endpointId } }),
  // The CSV is fetched rather than linked because the session token travels in
  // a header. Resolves to { blob, fileName }.
  async export(experimentId, { level = "animals", endpointId, excel } = {}) {
    const search = new URLSearchParams({ level });
    if (endpointId) search.set("endpointId", endpointId);
    if (excel) search.set("excel", "1");
    const token = getToken();
    const response = await fetch(`/api/experiments/${experimentId}/report/export?${search}`, {
      headers: { "Accept-Language": i18n.global.locale.value, ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    });
    if (!response.ok) {
      const data = await response.json().catch(() => ({}));
      const { t, te } = i18n.global;
      const message = data?.code && te(`errors.${data.code}`) ? t(`errors.${data.code}`) : t("errors.common.requestFailed", { status: response.status });
      throw new ApiError(message, { code: data?.code, status: response.status });
    }
    const name = /filename="([^"]+)"/.exec(response.headers.get("Content-Disposition") || "")?.[1] || `misko-report-${level}.csv`;
    return { blob: await response.blob(), fileName: name };
  },
};

export const dashboard = {
  summary: () => api("/dashboard/summary"),
};

// Experiments for pickers: the first 100 by code are plenty for a lab.
export const experimentOptions = () => api("/experiments", { query: { pageSize: 100, sort: "code", order: "asc" } });

// Paradigm wording (names, metric and event labels) in the current language.
// Cached per language so a list of results asks once per paradigm version.
const cache = new Map();
function cached(key, load) {
  const full = `${i18n.global.locale.value}:${key}`;
  if (!cache.has(full)) cache.set(full, load().catch((error) => (cache.delete(full), Promise.reject(error))));
  return cache.get(full);
}
export const paradigms = {
  list: () => cached("list", () => api("/paradigms").then((r) => r.data || [])),
  version: (key, version) => cached(`${key}@${version}`, () => api(`/paradigms/${key}/versions/${version}`)),
};
