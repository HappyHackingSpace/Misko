import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";

const deviceService = createCrudService({
  model: "device",
  allowed: ["name", "platform", "lastSeenAt"],
});

export const deviceRouter = createCrudRouter(deviceService);
