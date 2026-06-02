import { createCrudService } from "../../common/crudService.js";
import { createCrudRouter } from "../../common/crudController.js";
import { PERMISSIONS } from "../../config/permissions.js";

const deviceService = createCrudService({
  model: "device",
  allowed: ["name", "platform", "lastSeenAt"],
});

// Cihaz (telefon/kamera) donanımı lab geneli bir kaynaktır; yönetimi lab yapılandırmasıdır.
export const deviceRouter = createCrudRouter(deviceService, {
  writePermission: PERMISSIONS.LAB_CONFIGURE,
});
