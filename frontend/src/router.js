import { createRouter, createWebHistory } from "vue-router";
import { useAuth } from "./stores/auth.js";

const Login = () => import("./views/Login.vue");
const Experiments = () => import("./views/Experiments.vue");
const ExperimentDetail = () => import("./views/ExperimentDetail.vue");
const ExperimentForm = () => import("./views/ExperimentForm.vue");
const TestDetail = () => import("./views/TestDetail.vue");
const Subjects = () => import("./views/Subjects.vue");
const SubjectForm = () => import("./views/SubjectForm.vue");
const Paradigms = () => import("./views/Paradigms.vue");
const ParadigmDetail = () => import("./views/ParadigmDetail.vue");
const Environments = () => import("./views/Environments.vue");
const EnvironmentForm = () => import("./views/EnvironmentForm.vue");
const Users = () => import("./views/Users.vue");
const UserForm = () => import("./views/UserForm.vue");

const routes = [
  { path: "/login", component: Login, meta: { public: true } },
  { path: "/", redirect: "/experiments" },

  { path: "/experiments", component: Experiments, meta: { titleKey: "nav.experiments" } },
  // Writing an experiment needs study:write. The edit route is /:id/edit so it
  // cannot be confused with the detail screen at /:id.
  { path: "/experiments/new", component: ExperimentForm, meta: { permission: "study:write", titleKey: "nav.experiments" } },
  { path: "/experiments/:id", component: ExperimentDetail, meta: { titleKey: "nav.experiments" } },
  { path: "/experiments/:id/edit", component: ExperimentForm, meta: { permission: "study:write", titleKey: "nav.experiments" } },
  { path: "/tests/:id", component: TestDetail, meta: { titleKey: "nav.tests" } },

  { path: "/subjects", component: Subjects, meta: { titleKey: "nav.subjects" } },
  // Writing a subject needs subject:write, so a technician or a viewer cannot
  // reach the form by typing the address either.
  { path: "/subjects/new", component: SubjectForm, meta: { permission: "subject:write", titleKey: "nav.subjects" } },
  { path: "/subjects/:id", component: SubjectForm, meta: { permission: "subject:write", titleKey: "nav.subjects" } },
  { path: "/paradigms", component: Paradigms, meta: { titleKey: "nav.paradigms" } },
  { path: "/paradigms/:key", component: ParadigmDetail, meta: { titleKey: "nav.paradigms" } },
  { path: "/environments", component: Environments, meta: { titleKey: "nav.environments" } },
  // Defining an apparatus needs apparatus:write.
  { path: "/environments/new", component: EnvironmentForm, meta: { permission: "apparatus:write", titleKey: "nav.environments" } },

  { path: "/users", component: Users, meta: { permission: "user:manage", titleKey: "nav.users" } },
  { path: "/users/new", component: UserForm, meta: { permission: "user:manage", titleKey: "nav.users" } },
  { path: "/users/:id", component: UserForm, meta: { permission: "user:manage", titleKey: "nav.users" } },
];

export const router = createRouter({ history: createWebHistory(), routes });

router.beforeEach(async (to) => {
  const auth = useAuth();
  if (!auth.ready) await auth.init();
  if (!to.meta.public && !auth.isLoggedIn) return "/login";
  if (to.path === "/login" && auth.isLoggedIn) return "/";
  if (to.meta.permission && !auth.can(to.meta.permission)) return "/";
});
