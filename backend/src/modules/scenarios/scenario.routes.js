import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { PERMISSIONS } from "../../config/permissions.js";

const scenarioService = createCrudService({
  model: "scenario",
  allowed: ["name", "type", "description", "config"],
});

// Senaryo, ileride Apparatus'a evrilecek fiziksel kurulum tanımıdır.
export const scenarioRouter = createCrudRouter(scenarioService, {
  writePermission: PERMISSIONS.APPARATUS_WRITE,
});
