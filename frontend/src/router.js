import { createRouter, createWebHistory } from "vue-router";
import { useAuth } from "./stores/auth.js";

import Login from "./views/Login.vue";
import Dashboard from "./views/Dashboard.vue";
import Scenarios from "./views/Scenarios.vue";
import ScenarioForm from "./views/ScenarioForm.vue";
import Paradigms from "./views/Paradigms.vue";
import ParadigmDetail from "./views/ParadigmDetail.vue";
import Environments from "./views/Environments.vue";
import EnvironmentForm from "./views/EnvironmentForm.vue";
import Subjects from "./views/Subjects.vue";
import SubjectForm from "./views/SubjectForm.vue";
import Tests from "./views/Tests.vue";
import TestNew from "./views/TestNew.vue";
import TestDetail from "./views/TestDetail.vue";
import Users from "./views/Users.vue";
import UserForm from "./views/UserForm.vue";

const routes = [
  { path: "/login", component: Login, meta: { public: true } },
  { path: "/", component: Dashboard, meta: { titleKey: "nav.dashboard" } },

  { path: "/scenarios", component: Scenarios, meta: { titleKey: "nav.scenarios" } },
  { path: "/scenarios/new", component: ScenarioForm, meta: { titleKey: "nav.scenarios" } },
  { path: "/scenarios/:id", component: ScenarioForm, meta: { titleKey: "nav.scenarios" } },

  { path: "/paradigms", component: Paradigms, meta: { titleKey: "nav.paradigms" } },
  { path: "/paradigms/:key", component: ParadigmDetail, meta: { titleKey: "nav.paradigms" } },

  { path: "/environments", component: Environments, meta: { titleKey: "nav.environments" } },
  { path: "/environments/new", component: EnvironmentForm, meta: { titleKey: "nav.environments" } },
  { path: "/environments/:id", component: EnvironmentForm, meta: { titleKey: "nav.environments" } },

  { path: "/subjects", component: Subjects, meta: { titleKey: "nav.subjects" } },
  { path: "/subjects/new", component: SubjectForm, meta: { titleKey: "nav.subjects" } },
  { path: "/subjects/:id", component: SubjectForm, meta: { titleKey: "nav.subjects" } },

  { path: "/tests", component: Tests, meta: { titleKey: "nav.tests" } },
  { path: "/tests/new", component: TestNew, meta: { titleKey: "nav.tests" } },
  { path: "/tests/:id", component: TestDetail, meta: { titleKey: "nav.tests" } },

  { path: "/users", component: Users, meta: { admin: true, titleKey: "nav.users" } },
  { path: "/users/new", component: UserForm, meta: { admin: true, titleKey: "nav.users" } },
  { path: "/users/:id", component: UserForm, meta: { admin: true, titleKey: "nav.users" } },
];

export const router = createRouter({ history: createWebHistory(), routes });

router.beforeEach(async (to) => {
  const auth = useAuth();
  if (!auth.ready) await auth.init();
  if (!to.meta.public && !auth.isLoggedIn) return "/login";
  if (to.path === "/login" && auth.isLoggedIn) return "/";
  if (to.meta.admin && !auth.isAdmin) return "/";
});
