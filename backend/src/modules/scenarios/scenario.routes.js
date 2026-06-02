import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { PERMISSIONS } from "../../config/permissions.js";

const scenarioService = createCrudService({
  model: "scenario",
  allowed: ["name", "type", "description", "config"],
});

// A scenario is a physical setup definition that will later evolve into Apparatus.
export const scenarioRouter = createCrudRouter(scenarioService, {
  writePermission: PERMISSIONS.APPARATUS_WRITE,
});
