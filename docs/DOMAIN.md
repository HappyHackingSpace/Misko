# Mişko - Domain Model

The earlier subject/scenario model (a free-text `groupName` on the subject, a
`Scenario` bundling environments, a `Test = Subject + Scenario` with a JSON
result) is gone. The Go backend was rebuilt on a fresh schema around the study
design of a preclinical efficacy study. The full model, its reasoning and the
decisions are in [plans/DOMAIN_REDESIGN.md](plans/DOMAIN_REDESIGN.md); the
implemented rules and routes are in [backend/README.md](../backend/README.md).

In short:

- **Catalog:** substances (compounds, reference drugs, vehicles, inducers) and
  disease models; paradigms are versioned code; environments are physical
  apparatus with immutable revisions (an arena is one revision).
- **Design:** an experiment (`DRAFT -> RUNNING -> COMPLETED`, day 0 set on
  start) has groups with one of five roles (`HEALTHY_CONTROL`, `SHAM`,
  `DISEASE_CONTROL`, `POSITIVE_CONTROL`, `TREATMENT`) and a disease model,
  regimen items per group, timepoints in study days, protocols and endpoints.
  Starting checks the design and freezes it.
- **Execution:** an animal is allocated to exactly one group, ever, and is
  excluded with a reason, never deleted. Administrations record every actual
  dose, including the induction; induction outcomes record whether the model
  took.
- **Tests:** a test is one session (timepoint and protocol step) with one video
  and one or more animals, each in its own arena. Calibration is per arena with
  per-animal overrides; analysis runs per animal; every result is reviewed by a
  person before it counts.
- **Report:** per endpoint, group statistics over accepted results and the
  comparisons that follow from the group roles.

Single-tenant tenancy (one laboratory per installation), the five roles and the
code-defined permission matrix, and test comments are unchanged; see
`backend/README.md`.
