import { createRouter, createWebHistory } from "vue-router";
import { useAuth } from "./stores/auth.js";

import Login from "./views/Login.vue";
import Dashboard from "./views/Dashboard.vue";
import Scenarios from "./views/Scenarios.vue";
import Paradigms from "./views/Paradigms.vue";
import Subjects from "./views/Subjects.vue";
import Devices from "./views/Devices.vue";
import Tests from "./views/Tests.vue";
import Users from "./views/Users.vue";

const routes = [
  { path: "/login", component: Login, meta: { public: true } },
  { path: "/", component: Dashboard },
  { path: "/scenarios", component: Scenarios },
  { path: "/paradigms", component: Paradigms },
  { path: "/subjects", component: Subjects },
  { path: "/devices", component: Devices },
  { path: "/tests", component: Tests },
  { path: "/users", component: Users, meta: { admin: true } },
];

export const router = createRouter({ history: createWebHistory(), routes });

router.beforeEach(async (to) => {
  const auth = useAuth();
  if (!auth.ready) await auth.init();
  if (!to.meta.public && !auth.isLoggedIn) return "/login";
  if (to.path === "/login" && auth.isLoggedIn) return "/";
  if (to.meta.admin && !auth.isAdmin) return "/";
});
