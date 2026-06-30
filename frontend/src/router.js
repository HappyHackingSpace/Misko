import { createRouter, createWebHistory } from "vue-router";
import { useAuth } from "./stores/auth.js";

const Login = () => import("./views/Login.vue");
const Dashboard = () => import("./views/Dashboard.vue");
const Scenarios = () => import("./views/Scenarios.vue");
const ScenarioForm = () => import("./views/ScenarioForm.vue");
const Paradigms = () => import("./views/Paradigms.vue");
const ParadigmDetail = () => import("./views/ParadigmDetail.vue");
const Environments = () => import("./views/Environments.vue");
const EnvironmentForm = () => import("./views/EnvironmentForm.vue");
const Subjects = () => import("./views/Subjects.vue");
const SubjectForm = () => import("./views/SubjectForm.vue");
const Tests = () => import("./views/Tests.vue");
const TestNew = () => import("./views/TestNew.vue");
const TestDetail = () => import("./views/TestDetail.vue");
const Users = () => import("./views/Users.vue");
const UserForm = () => import("./views/UserForm.vue");

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
