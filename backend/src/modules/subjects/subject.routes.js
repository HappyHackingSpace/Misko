import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { requirePermission } from "../../middleware/authenticate.js";
import { validateBody } from "../../middleware/validate.js";
import { asyncHandler } from "../../utils/asyncHandler.js";
import { PERMISSIONS } from "../../config/permissions.js";
import {
  createSubjectSchema,
  updateSubjectSchema,
  weightLogSchema,
  diseaseModelLinkSchema,
  treatmentLinkSchema,
} from "./subject.validation.js";
import { listWeights, addWeight, deleteWeight } from "./weight.service.js";
import {
  listSubjectDiseaseModels, attachDiseaseModel, detachDiseaseModel,
  listSubjectTreatments, attachTreatment, detachTreatment,
} from "./subjectLinks.service.js";

const subjectService = createCrudService({
  model: "subject",
  allowed: [
    "code", "microchipId", "earTag",
    "species", "strain", "line", "genotype", "zygosity", "sex", "birthDate", "coatColor",
    "healthStatus", "notes",
    "cageId", "litter", "cohort",
    "status", "acquiredAt", "sacrificedAt",
    "groupName",
  ],
  searchFields: ["code", "strain", "line", "genotype", "cageId", "cohort", "notes"],
  filterFields: { code: "text", sex: "enum", strain: "text", status: "enum", zygosity: "enum", groupName: "text" },
  sortFields: ["code", "sex", "strain", "status", "createdAt"],
});

export const subjectRouter = createCrudRouter(subjectService, {
  writePermission: PERMISSIONS.SUBJECT_WRITE,
  createSchema: createSubjectSchema,
  updateSchema: updateSubjectSchema,
});

// Nested weight-log time series (docs/DOMAIN.md §2). Reads need only auth; writes
// need `weight:write` (TECHNICIAN and above). These paths are more specific than
// the generic `/:id` route, so Express matches them correctly.
subjectRouter.get(
  "/:id/weights",
  asyncHandler(async (req, res) => {
    res.json(await listWeights(req.params.id));
  }),
);

subjectRouter.post(
  "/:id/weights",
  requirePermission(PERMISSIONS.WEIGHT_WRITE),
  validateBody(weightLogSchema),
  asyncHandler(async (req, res) => {
    res.status(201).json(await addWeight(req.params.id, req.body));
  }),
);

subjectRouter.delete(
  "/:id/weights/:weightId",
  requirePermission(PERMISSIONS.WEIGHT_WRITE),
  asyncHandler(async (req, res) => {
    await deleteWeight(req.params.id, req.params.weightId);
    res.status(204).end();
  }),
);

// Nested disease-model assignments (docs/DOMAIN.md §3). Writes need `subject:write`.
subjectRouter.get(
  "/:id/disease-models",
  asyncHandler(async (req, res) => {
    res.json(await listSubjectDiseaseModels(req.params.id));
  }),
);

subjectRouter.post(
  "/:id/disease-models",
  requirePermission(PERMISSIONS.SUBJECT_WRITE),
  validateBody(diseaseModelLinkSchema),
  asyncHandler(async (req, res) => {
    res.status(201).json(await attachDiseaseModel(req.params.id, req.body));
  }),
);

subjectRouter.delete(
  "/:id/disease-models/:linkId",
  requirePermission(PERMISSIONS.SUBJECT_WRITE),
  asyncHandler(async (req, res) => {
    await detachDiseaseModel(req.params.id, req.params.linkId);
    res.status(204).end();
  }),
);

// Nested treatment assignments (docs/DOMAIN.md §3). Writes need `subject:write`.
subjectRouter.get(
  "/:id/treatments",
  asyncHandler(async (req, res) => {
    res.json(await listSubjectTreatments(req.params.id));
  }),
);

subjectRouter.post(
  "/:id/treatments",
  requirePermission(PERMISSIONS.SUBJECT_WRITE),
  validateBody(treatmentLinkSchema),
  asyncHandler(async (req, res) => {
    res.status(201).json(await attachTreatment(req.params.id, req.body));
  }),
);

subjectRouter.delete(
  "/:id/treatments/:linkId",
  requirePermission(PERMISSIONS.SUBJECT_WRITE),
  asyncHandler(async (req, res) => {
    await detachTreatment(req.params.id, req.params.linkId);
    res.status(204).end();
  }),
);
