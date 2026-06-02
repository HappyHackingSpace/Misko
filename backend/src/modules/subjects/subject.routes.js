import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { PERMISSIONS } from "../../config/permissions.js";

const subjectService = createCrudService({
  model: "subject",
  allowed: ["code", "sex", "groupName", "birthDate", "notes"],
});

export const subjectRouter = createCrudRouter(subjectService, {
  writePermission: PERMISSIONS.SUBJECT_WRITE,
});
