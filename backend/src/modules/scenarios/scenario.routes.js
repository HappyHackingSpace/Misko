import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";

const scenarioService = createCrudService({
  model: "scenario",
  allowed: ["name", "type", "description", "config"],
});

export const scenarioRouter = createCrudRouter(scenarioService);
