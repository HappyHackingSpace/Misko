// Observer scoring of one animal of a test on the test's current video. Times
// are microseconds from the clip start of that video. Permission test:run.
import { api } from "./client.js";

const base = (testId, testSubjectId) => `/tests/${testId}/subjects/${testSubjectId}/scoring`;

// The API answers { paradigm, inputEvents, video, sheet, events }; sheet is null
// until the first event. The player reads one flat view: an animal without a
// sheet is an empty draft, and the duration defaults to the clip length.
function view(body) {
  const sheet = body.sheet;
  const clip = body.video && body.video.clipEndUs ? body.video.clipEndUs - body.video.clipStartUs : null;
  return {
    ...body,
    status: sheet?.status || "DRAFT",
    version: sheet?.version || 1,
    durationUs: sheet?.durationUs ?? clip,
    scoredTypes: sheet?.scoredEventTypes || [],
    events: body.events || [],
    inputEvents: body.inputEvents || [],
  };
}

export const scoring = {
  get: (testId, testSubjectId) => api(base(testId, testSubjectId)).then(view),
  // body: { type, label?, startUs, endUs? }
  add: (testId, testSubjectId, body) => api(`${base(testId, testSubjectId)}/events`, { method: "POST", body }),
  change: (testId, testSubjectId, eventId, body) =>
    api(`${base(testId, testSubjectId)}/events/${eventId}`, { method: "PATCH", body }),
  remove: (testId, testSubjectId, eventId) => api(`${base(testId, testSubjectId)}/events/${eventId}`, { method: "DELETE" }),
  // { scoredTypes, durationUs? } in the player's words
  complete: (testId, testSubjectId, { scoredTypes, durationUs }) =>
    api(`${base(testId, testSubjectId)}/complete`, { method: "POST", body: { scoredEventTypes: scoredTypes, durationUs } }),
  reopen: (testId, testSubjectId) => api(`${base(testId, testSubjectId)}/reopen`, { method: "POST", body: {} }),
};
