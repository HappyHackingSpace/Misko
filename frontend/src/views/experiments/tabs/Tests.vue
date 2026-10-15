<script setup>
// The Tests tab of a study: its sessions by timepoint, planning a new one and
// changing the animals of a session that has not started. Running a session
// happens on its own page.
import { computed, h, inject, onMounted, ref } from "vue";
import { RouterLink } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDataTable, NDrawer, NDrawerContent, NEmpty, NSpin, NTag } from "naive-ui";
import { tests } from "../../../api/tests.js";
import { studyDesign } from "../../../api/dosing.js";
import { useAuth } from "../../../stores/auth.js";
import { addDays, tagType, useExecutionFormat } from "../../../composables/useExecutionFormat.js";
import { useParadigmNames } from "../../../composables/useParadigmNames.js";
import PlanSessionDrawer from "../../../components/test/PlanSessionDrawer.vue";
import SessionAnimalsEditor from "../../../components/test/SessionAnimalsEditor.vue";

const { t } = useI18n();
const { experiment, id } = inject("study");
const auth = useAuth();
const fmt = useExecutionFormat();
const { paradigmName } = useParadigmNames();

const running = computed(() => experiment.value?.status === "RUNNING");
const canPlan = computed(() => running.value && auth.canWriteTests);

const timepoints = ref([]);
const allocations = ref([]);
const sessions = ref([]);
const loading = ref(false);
const error = ref("");
const planning = ref(null); // { timepointId }
const editing = ref(null); // a planned session whose animals are edited

// Every session of the study; a study has tens of them, not thousands.
async function loadSessions() {
  const all = [];
  for (let page = 1; ; page++) {
    const result = await tests.ofExperiment(id.value, { page, pageSize: 100 });
    all.push(...(result.data || []));
    if (all.length >= result.total || !result.data?.length) break;
  }
  sessions.value = all;
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    const [timepointList, allocationList] = await Promise.all([studyDesign.timepoints(id.value), studyDesign.allocations(id.value)]);
    timepoints.value = timepointList.data || [];
    allocations.value = allocationList.data || [];
    await loadSessions();
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

const activeAnimals = computed(() => allocations.value.filter((a) => !a.excluded).length);
const sections = computed(() =>
  timepoints.value.map((tp) => {
    const list = sessions.value.filter((s) => s.timepointId === tp.id);
    const tested = new Set(list.filter((s) => s.status !== "CANCELLED").flatMap((s) => s.subjects.map((x) => x.allocationId)));
    return { timepoint: tp, sessions: list, scheduled: tested.size };
  }),
);
const studyDate = (day) => (experiment.value?.dayZero ? fmt.date(addDays(experiment.value.dayZero, day)) : "");

const columns = computed(() => [
  {
    title: t("execution.tests.scheduledAt"),
    key: "scheduledAt",
    width: 170,
    render: (row) => h(RouterLink, { to: `/tests/${row.id}`, class: "link", "data-test": "session-link" }, () => fmt.dateTime(row.scheduledAt)),
  },
  {
    title: t("execution.tests.step"),
    key: "step",
    render: (row) =>
      h("div", [
        h("div", paradigmName(row.paradigmKey)),
        h("div", { class: "muted small" }, `${row.protocolName} · ${t("execution.tests.versionN", { n: row.protocolVersion })} · ${t("execution.tests.stepN", { n: row.stepPosition })}`),
      ]),
  },
  {
    title: t("execution.tests.animals"),
    key: "subjects",
    render: (row) =>
      h(
        "div",
        { class: "chips" },
        row.subjects.map((s) =>
          h(NTag, { size: "small", bordered: false, type: s.excluded ? "error" : "default", "data-test": "session-animal-chip" }, () => `${s.subjectCode} · ${s.environmentName}`),
        ),
      ),
  },
  {
    title: t("execution.status"),
    key: "status",
    width: 130,
    render: (row) => h(NTag, { size: "small", round: true, type: tagType(row.status), "data-test": "session-status", "data-status": row.status }, () => fmt.status(row.status)),
  },
  {
    title: "",
    key: "actions",
    align: "right",
    width: 190,
    render: (row) =>
      h("div", { class: "actions" }, [
        row.status === "PLANNED" && canPlan.value
          ? h(NButton, { size: "small", quaternary: true, "data-test": "edit-animals", onClick: () => (editing.value = row) }, () => t("execution.tests.editAnimals"))
          : null,
        h(RouterLink, { to: `/tests/${row.id}`, custom: true }, { default: ({ navigate }) => h(NButton, { size: "small", secondary: true, onClick: navigate }, () => t("execution.open")) }),
      ]),
  },
]);

function planned() {
  planning.value = null;
  loadSessions().catch((e) => (error.value = e.message));
}
function updated(test) {
  editing.value = test;
  sessions.value = sessions.value.map((s) => (s.id === test.id ? test : s));
}

onMounted(load);
</script>

<template>
  <div class="tests-tab" data-test="tests-tab">
    <NAlert v-if="experiment?.status === 'DRAFT'" type="info" :title="$t('execution.tests.notStarted')" />
    <NAlert v-if="error" type="error" :title="error" />
    <div class="toolbar">
      <span class="muted">{{ $t("execution.tests.intro") }}</span>
      <NButton v-if="canPlan" type="primary" data-test="plan-session" @click="planning = { timepointId: null }">{{ $t("execution.tests.planTitle") }}</NButton>
    </div>
    <NSpin :show="loading">
      <NEmpty v-if="!loading && !timepoints.length" :description="$t('execution.tests.noTimepoints')" />
      <NCard v-for="section in sections" :key="section.timepoint.id" size="small" :bordered="true" class="section" data-test="timepoint-section" :data-timepoint="section.timepoint.name">
        <template #header>
          <span>{{ section.timepoint.name }}</span>
          <span class="muted head-meta">
            · {{ $t("execution.studyDay", { day: section.timepoint.studyDay }) }}
            <template v-if="experiment?.dayZero"> · {{ studyDate(section.timepoint.studyDay) }}</template>
          </span>
        </template>
        <template #header-extra>
          <span class="muted coverage" data-test="timepoint-coverage">
            {{ $t("execution.tests.coverage", { scheduled: section.scheduled, total: activeAnimals }) }}
          </span>
          <NButton v-if="canPlan" size="small" quaternary data-test="plan-at-timepoint" @click="planning = { timepointId: section.timepoint.id }">
            + {{ $t("execution.tests.plan") }}
          </NButton>
        </template>
        <NDataTable v-if="section.sessions.length" size="small" :columns="columns" :data="section.sessions" :row-key="(row) => row.id" :scroll-x="760" />
        <p v-else class="muted empty">{{ $t("execution.tests.noSessions") }}</p>
      </NCard>
    </NSpin>

    <PlanSessionDrawer
      v-if="planning && experiment"
      :experiment="experiment"
      :timepoints="timepoints"
      :allocations="allocations"
      :sessions="sessions"
      :timepoint-id="planning.timepointId"
      @close="planning = null"
      @planned="planned"
    />

    <NDrawer :show="!!editing" :width="480" style="max-width: 100vw" @update:show="(v) => !v && (editing = null)">
      <NDrawerContent v-if="editing" :title="$t('execution.tests.editAnimals')" closable>
        <p class="muted">{{ editing.timepointName }} · {{ paradigmName(editing.paradigmKey) }} · {{ fmt.dateTime(editing.scheduledAt) }}</p>
        <SessionAnimalsEditor :test="editing" @updated="updated" />
      </NDrawerContent>
    </NDrawer>
  </div>
</template>

<style scoped>
.tests-tab { display: flex; flex-direction: column; gap: 16px; min-width: 0; }
.toolbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; }
.section + .section { margin-top: 16px; }
.head-meta { font-weight: 400; font-size: 13px; }
.coverage { font-size: 12px; margin-right: 8px; }
.empty { margin: 0; }
:deep(.chips) { display: flex; gap: 4px; flex-wrap: wrap; }
:deep(.actions) { display: flex; gap: 6px; justify-content: flex-end; }
:deep(.small) { font-size: 12px; }
</style>
