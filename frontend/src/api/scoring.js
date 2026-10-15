// Observer scoring of one animal of a test on the test's current video. Times
// are microseconds from the clip start of that video. Permission test:run.
import { api } from "./client.js";

const base = (testId, testSubjectId) => `/tests/${testId}/subjects/${testSubjectId}/scoring`;

export const scoring = {
  // { status: DRAFT|COMPLETED, version, durationUs, scoredTypes, events, inputEvents }
  get: (testId, testSubjectId) => api(base(testId, testSubjectId)),
  // body: { type, label?, startUs, endUs? }
  add: (testId, testSubjectId, body) => api(`${base(testId, testSubjectId)}/events`, { method: "POST", body }),
  change: (testId, testSubjectId, eventId, body) =>
    api(`${base(testId, testSubjectId)}/events/${eventId}`, { method: "PATCH", body }),
  remove: (testId, testSubjectId, eventId) => api(`${base(testId, testSubjectId)}/events/${eventId}`, { method: "DELETE" }),
  // body: { scoredTypes, durationUs? }
  complete: (testId, testSubjectId, body) => api(`${base(testId, testSubjectId)}/complete`, { method: "POST", body }),
  reopen: (testId, testSubjectId) => api(`${base(testId, testSubjectId)}/reopen`, { method: "POST", body: {} }),
};
