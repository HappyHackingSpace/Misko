import { z } from "zod";
import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { nullableString } from "../../common/zodFields.js";
import { PERMISSIONS } from "../../config/permissions.js";

// DiseaseModel catalog (docs/DOMAIN.md §3). Reference data managed by researchers.
const diseaseModelService = createCrudService({
  model: "diseaseModel",
  allowed: ["key", "name", "category", "description"],
  searchFields: ["key", "name", "category", "description"],
  filterFields: { key: "text", category: "text" },
  sortFields: ["key", "name", "category", "createdAt"],
});

const createSchema = z.object({
  key: z.string().min(1, "Key is required"),
  name: z.string().min(1, "Name is required"),
  category: nullableString,
  description: nullableString,
});

const updateSchema = z.object({
  key: z.string().min(1).optional(),
  name: z.string().min(1).optional(),
  category: nullableString,
  description: nullableString,
});

export const diseaseModelRouter = createCrudRouter(diseaseModelService, {
  writePermission: PERMISSIONS.SUBJECT_WRITE,
  createSchema,
  updateSchema,
});
