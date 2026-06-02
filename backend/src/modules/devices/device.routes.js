import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { PERMISSIONS } from "../../config/permissions.js";

const deviceService = createCrudService({
  model: "device",
  allowed: ["name", "platform", "lastSeenAt"],
});

// Device (phone/camera) hardware is a lab-wide resource; managing it is lab configuration.
export const deviceRouter = createCrudRouter(deviceService, {
  writePermission: PERMISSIONS.LAB_CONFIGURE,
});
