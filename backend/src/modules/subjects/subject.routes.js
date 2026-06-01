import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";

const subjectService = createCrudService({
  model: "subject",
  allowed: ["code", "sex", "groupName", "birthDate", "notes"],
});

export const subjectRouter = createCrudRouter(subjectService);
