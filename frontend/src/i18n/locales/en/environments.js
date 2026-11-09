export default {
  environments: {
    title: "Environments",
    intro:
      "Named arenas created from a paradigm template. A lab can hold several environments per paradigm (for example two Morris water tanks). Measurements are set here and every test keeps the revision it was run with.",
    allParadigms: "All paradigms",
    noMatch: "No environment matches these filters.",
    created: "Environment {name} was created",
    revisionSaved: "The new revision was created",
    calibration: {
      intro: "Calibrate once when the rig is set up. Every subject tested in this arena uses it, unless its test is calibrated by hand.",
      defaultActive: "Environment default active",
      defaultInactive: "Environment default inactive",
      automatic: "This calibration will be used automatically for other tests in the same arena. Keep the camera and arena position fixed; calibrate a test separately when its video uses another camera or resolution.",
      waiting: "Not calibrated yet. Subjects tested in this arena cannot be analysed into centimetres until it is.",
      rejected: "Rejected: the check points deviate by more than the tolerance. Analyses in this arena wait until it is corrected.",
      fixedCamera: "This assumes the camera and the arena stay where they were. A subject whose video does not match can be calibrated by hand from its test.",
      saved: "The calibration was saved",
    },
  },
  errors: {
    common: {
      invalidNotes: "Notes can contain at most 2000 characters",
    },
    environment: {
      notFound: "Environment not found",
      revisionNotFound: "This revision of the environment was not found",
      nameTaken: "Another environment already has this name",
      unknownParadigm: "Unknown paradigm",
      unknownParadigmVersion: "This paradigm version is not published",
      invalidParameters: "The values do not fit the paradigm: give every apparatus measurement, within its range and consistent with the others",
      invalidParadigmKey: "Choose a paradigm",
      invalidParadigmVersion: "The paradigm version is not valid",
    },
    calibration: {
      invalidScope: "This calibration does not belong here",
    },
  },
};
