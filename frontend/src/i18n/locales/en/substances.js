export default {
  nav: { substances: "Substances" },
  substances: {
    title: "Substances",
    subtitle: "Test compounds, reference drugs, vehicles and disease inducers shared by every study of the lab.",
    new: "New substance",
    edit: "Edit substance",
    name: "Name",
    kind: "Kind",
    description: "Description",
    searchName: "Search by name",
    allKinds: "All kinds",
    namePlaceholder: "Donepezil",
    empty: "No substances yet.",
    noMatch: "Nothing matches these filters.",
    created: "{name} was created",
    updated: "{name} was saved",
    deleted: "{name} was deleted",
    confirmDelete: "Delete {name}? This is only possible while no regimen or dose uses it.",
    kinds: {
      COMPOUND: "Test compound",
      REFERENCE_DRUG: "Reference drug",
      VEHICLE: "Vehicle",
      INDUCER: "Disease inducer",
    },
    kindHints: {
      COMPOUND: "A substance under investigation.",
      REFERENCE_DRUG: "A known active drug, used by positive control groups.",
      VEHICLE: "The carrier given without the active substance, for example saline or 0.5% CMC.",
      INDUCER: "An agent that produces the disease model, for example MPTP, LPS or scopolamine.",
    },
  },
  errors: {
    common: {
      invalidJSON: "The request could not be read",
      bodyTooLarge: "The request is too large",
      serverError: "Server error, please try again",
      noChanges: "Nothing was changed",
      invalidQuery: "These filters are not valid",
      invalidName: "The name must contain 1 to 120 printable characters",
      invalidDescription: "The description can contain at most 5000 characters",
    },
    auth: {
      unauthenticated: "Please sign in again",
      forbidden: "You do not have permission for this action",
    },
    // The API code is catalog.nameTaken for substances and disease models alike.
    catalog: {
      nameTaken: "An entry with this name already exists",
    },
    substance: {
      notFound: "Substance not found",
      invalidKind: "Choose the kind of the substance",
      inUse: "This substance is used by a regimen or a recorded dose and cannot be deleted",
    },
  },
};
