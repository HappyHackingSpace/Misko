// The one video of a test session and the calibration of each animal's arena
// on it. Video bytes go straight to storage (see upload.js); the API only
// declares and verifies them.
import { api } from "./client.js";
import { sendVideo } from "./upload.js";

export const recordings = {
  list: (testId) => api(`/tests/${testId}/recordings`),
  readUrl: (testId, recordingId) => api(`/tests/${testId}/recordings/${recordingId}/read-url`),
};

/**
 * Declares, uploads and verifies a video for a test; it becomes the test's
 * video once verified. onStage receives "checksum", "upload" and "verify";
 * onProgress 0 to 1 while uploading. Returns the finalized recording.
 */
export const uploadVideo = (testId, file, progress = {}) =>
  sendVideo(file, {
    ...progress,
    declare: (body) => api(`/tests/${testId}/recordings`, { method: "POST", body }),
    verify: (started) => api(`/tests/${testId}/recordings/${started.recording.id}/finalize`, { method: "POST", body: {} }),
  });

export const calibration = {
  // One row per animal of the test, on its current video.
  status: (testId) => api(`/tests/${testId}/calibration-status`),
  overrides: (testId, testSubjectId) => api(`/tests/${testId}/subjects/${testSubjectId}/calibrations`),
  createOverride: (testId, testSubjectId, body) =>
    api(`/tests/${testId}/subjects/${testSubjectId}/calibrations`, { method: "POST", body }),
};
