import { createRouter, createWebHistory } from "vue-router";
import { useAuth } from "./stores/auth.js";

const Login = () => import("./views/Login.vue");
const Dashboard = () => import("./views/Dashboard.vue");
const Experiments = () => import("./views/Experiments.vue");
const ExperimentDetail = () => import("./views/experiments/ExperimentDetail.vue");
const Substances = () => import("./views/Substances.vue");
const DiseaseModels = () => import("./views/DiseaseModels.vue");
const Review = () => import("./views/review/Review.vue");
const ExperimentForm = () => import("./views/ExperimentForm.vue");
const TestDetail = () => import("./views/TestDetail.vue");
const Tests = () => import("./views/Tests.vue");
const Subjects = () => import("./views/subjects/Subjects.vue");
const SubjectDetail = () => import("./views/subjects/SubjectDetail.vue");
const Reports = () => import("./views/Reports.vue");
const Trials = () => import("./views/trials/Trials.vue");
const TrialDetail = () => import("./views/trials/TrialDetail.vue");
const Paradigms = () => import("./views/Paradigms.vue");
const ParadigmDetail = () => import("./views/ParadigmDetail.vue");
const Environments = () => import("./views/Environments.vue");
const EnvironmentForm = () => import("./views/EnvironmentForm.vue");
const EnvironmentDetail = () => import("./views/EnvironmentDetail.vue");
const Users = () => import("./views/Users.vue");
const UserForm = () => import("./views/UserForm.vue");

const routes = [
  { path: "/login", component: Login, meta: { public: true } },
  { path: "/", redirect: "/dashboard" },

  { path: "/dashboard", component: Dashboard, meta: { titleKey: "nav.dashboard" } },
  { path: "/experiments", component: Experiments, meta: { titleKey: "nav.experiments" } },
  // Writing an experiment needs study:write. The edit route is /:id/edit so it
  // cannot be confused with the detail screen at /:id.
  { path: "/experiments/new", component: ExperimentForm, meta: { permission: "study:write", titleKey: "nav.experiments" } },
  { path: "/experiments/:id", component: ExperimentDetail, meta: { titleKey: "nav.experiments" } },
  { path: "/experiments/:id/edit", component: ExperimentForm, meta: { permission: "study:write", titleKey: "nav.experiments" } },
  { path: "/tests", component: Tests, meta: { titleKey: "nav.tests" } },
  { path: "/tests/:id", component: TestDetail, meta: { titleKey: "nav.tests" } },
  // Paradigm trials are read by every role; starting and running one needs
  // trial:run, checked on the page and by the API.
  { path: "/trials", component: Trials, meta: { titleKey: "nav.trials" } },
  { path: "/trials/:id", component: TrialDetail, meta: { titleKey: "nav.trials" } },

  // Subjects are created and edited in a drawer; the detail page shows weights and allocation.
  { path: "/subjects", component: Subjects, meta: { titleKey: "nav.subjects" } },
  { path: "/subjects/:id", component: SubjectDetail, meta: { titleKey: "nav.subjects" } },
  // Reading results is open to every signed-in role, like the rest of the data.
  { path: "/reports", component: Reports, meta: { titleKey: "nav.reports" } },
  { path: "/review", component: Review, meta: { titleKey: "nav.review" } },
  // Substances and disease models are created and edited in a drawer.
  { path: "/substances", component: Substances, meta: { titleKey: "nav.substances" } },
  { path: "/disease-models", component: DiseaseModels, meta: { titleKey: "nav.diseaseModels" } },
  { path: "/paradigms", component: Paradigms, meta: { titleKey: "nav.paradigms" } },
  { path: "/paradigms/:key", component: ParadigmDetail, meta: { titleKey: "nav.paradigms" } },
  { path: "/environments", component: Environments, meta: { titleKey: "nav.environments" } },
  // Defining an apparatus needs apparatus:write.
  { path: "/environments/new", component: EnvironmentForm, meta: { permission: "apparatus:write", titleKey: "nav.environments" } },
  // Reading an apparatus and its calibration is open to every role; calibrating
  // is checked on the page and by the API.
  { path: "/environments/:id", component: EnvironmentDetail, meta: { titleKey: "nav.environments" } },

  { path: "/users", component: Users, meta: { permission: "user:manage", titleKey: "nav.users" } },
  { path: "/users/new", component: UserForm, meta: { permission: "user:manage", titleKey: "nav.users" } },
  { path: "/users/:id", component: UserForm, meta: { permission: "user:manage", titleKey: "nav.users" } },
  // The former catalog page, and any unknown path, lands somewhere useful.
  { path: "/catalog", redirect: "/substances" },
  { path: "/:pathMatch(.*)*", redirect: "/dashboard" },
];

export const router = createRouter({ history: createWebHistory(), routes });

router.beforeEach(async (to) => {
  const auth = useAuth();
  if (!auth.ready) await auth.init();
  if (!to.meta.public && !auth.isLoggedIn) return "/login";
  if (to.path === "/login" && auth.isLoggedIn) return "/";
  if (to.meta.permission && !auth.can(to.meta.permission)) return "/";
});
