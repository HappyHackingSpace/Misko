import { z } from "zod";
import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { nullableString, nullableNumber, nullableEnum } from "../../common/zodFields.js";
import { PERMISSIONS } from "../../config/permissions.js";

const ROUTES = ["IP", "ORAL", "SC", "IV", "IN"];

// Treatment catalog (docs/DOMAIN.md §3). A drug/compound or vehicle/control.
const treatmentService = createCrudService({
  model: "treatment",
  allowed: ["key", "name", "defaultDose", "unit", "route", "notes"],
  searchFields: ["key", "name", "notes"],
  filterFields: { key: "text", route: "enum" },
  sortFields: ["key", "name", "route", "createdAt"],
});

const createSchema = z.object({
  key: z.string().min(1, "Key is required"),
  name: z.string().min(1, "Name is required"),
  defaultDose: nullableNumber,
  unit: nullableString,
  route: nullableEnum(ROUTES),
  notes: nullableString,
});

const updateSchema = z.object({
  key: z.string().min(1).optional(),
  name: z.string().min(1).optional(),
  defaultDose: nullableNumber,
  unit: nullableString,
  route: nullableEnum(ROUTES),
  notes: nullableString,
});

export const treatmentRouter = createCrudRouter(treatmentService, {
  writePermission: PERMISSIONS.SUBJECT_WRITE,
  createSchema,
  updateSchema,
});
