<script setup>
// Every test across experiments in one worklist, newest scheduled first. A test
// is still planned from its experiment; this page is the way back to a test to
// upload its video, calibrate it or read its result.
import { computed, h, reactive, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { useEnvironmentRevisions } from "../composables/useEnvironmentRevisions.js";
import { NAlert, NButton, NCard, NDataTable, NSelect, NTag } from "naive-ui";
import { useDataTable } from "../composables/useDataTable.js";
import { experiments, subjects } from "../api/endpoints.js";
import PageHead from "../components/PageHead.vue";

const { t } = useI18n();

const experimentId = ref(null);
const status = ref("");
const error = ref("");

// The endpoint has one fixed order (newest scheduled first) and ignores sort.
const table = useDataTable("/tests", {
  pageSize: 20,
  extraParams: () => ({ experimentId: experimentId.value, status: status.value }),
});
watch([experimentId, status], () => table.setPage(1));

// Tests carry ids only; codes are looked up once per id and kept.
const experimentById = reactive({});
const subjectById = reactive({});
const experimentList = ref([]);

async function loadExperiments() {
  try {
    const page = await experiments.list({ pageSize: 100 });
    experimentList.value = page.data || [];
    for (const e of experimentList.value) experimentById[e.id] = e;
  } catch (e) {
    error.value = e.message;
  }
}

async function resolveNames(rows) {
  const missingSubjects = [...new Set(rows.map((r) => r.subjectId))].filter((id) => !(id in subjectById));
  const missingExperiments = [...new Set(rows.map((r) => r.experimentId))].filter((id) => !(id in experimentById));
  for (const id of missingSubjects) subjectById[id] = null;
  for (const id of missingExperiments) experimentById[id] = null;
  await Promise.all([
    ...missingSubjects.map((id) => subjects.get(id).then((s) => (subjectById[id] = s)).catch(() => {})),
    ...missingExperiments.map((id) => experiments.get(id).then((e) => (experimentById[id] = e)).catch(() => {})),
  ]);
}
watch(() => table.state.rows, resolveNames);

const statusTag = { PLANNED: "info", IN_PROGRESS: "warning", COMPLETED: "success", CANCELLED: "error" };

// The label carries a stable test hook: Naive UI's dropdown options are not a
// native <select>, so the browser suite cannot address them by value.
function testLabel(text, testId) {
  return () => h("span", { "data-test": testId }, text);
}

const experimentOptions = computed(() =>
  experimentList.value.map((e) => ({ value: e.id, label: testLabel(`${e.code} · ${e.title}`, `option-experiment-${e.id}`) })),
);
const statusOptions = computed(() => [
  { value: "", label: testLabel(t("testsPage.allStatuses"), "option-status-all") },
  ...["PLANNED", "IN_PROGRESS", "COMPLETED", "CANCELLED"].map((s) => ({
    value: s,
    label: testLabel(t(`statuses.${s}`), `option-status-${s}`),
  })),
]);

// Which apparatus and revision each test was run with.
const revisions = useEnvironmentRevisions();
revisions.load().catch(() => {});

const columns = computed(() => [
  {
    title: t("tests.subject"),
    key: "subjectId",
    render: (row) =>
      h(RouterLink, { class: "link", to: `/tests/${row.id}`, "data-test": "all-tests-link" }, () =>
        subjectById[row.subjectId]?.code || t("tests.subjectUnknown"),
      ),
  },
  {
    title: t("testsPage.experiment"),
    key: "experimentId",
    render: (row) => {
      const experiment = experimentById[row.experimentId];
      return h(RouterLink, { class: "link-muted", to: `/experiments/${row.experimentId}` }, () =>
        experiment ? experiment.code : "…",
      );
    },
  },
  {
    title: t("tests.paradigm"),
    key: "paradigmKey",
    render: (row) => [
      h(NTag, { size: "small", round: true, bordered: false }, () => row.paradigmKey),
      h("span", { class: "muted version" }, `v${row.paradigmVersion}`),
    ],
  },
  {
    title: t("tests.environment"),
    key: "environmentRevisionId",
    render: (row) => {
      const apparatus = revisions.byId.value[row.environmentRevisionId];
      if (!apparatus) return "…";
      return h(RouterLink, { class: "link-muted", to: `/environments/${apparatus.environmentId}`, "data-test": "all-tests-environment" }, () => apparatus.environmentName);
    },
  },
  {
    title: t("tests.revision"),
    key: "environmentRevision",
    render: (row) => {
      const apparatus = revisions.byId.value[row.environmentRevisionId];
      if (!apparatus) return "…";
      return h("span", { class: "muted", "data-test": "all-tests-revision", "data-revision": apparatus.number }, `#${apparatus.number}`);
    },
  },
  { title: t("tests.scheduled"), key: "scheduledAt", render: (row) => new Date(row.scheduledAt).toLocaleString() },
  {
    title: t("tests.status"),
    key: "status",
    render: (row) =>
      h(NTag, { size: "small", round: true, type: statusTag[row.status] || "default", "data-status": row.status }, () =>
        t(`statuses.${row.status}`),
      ),
  },
  {
    title: "",
    key: "actions",
    align: "right",
    render: (row) =>
      h(RouterLink, { to: `/tests/${row.id}`, custom: true }, {
        default: ({ navigate }) =>
          h(NButton, { size: "small", secondary: true, onClick: navigate }, () =>
            row.status === "CANCELLED" ? t("testsPage.view") : t("testsPage.open"),
          ),
      }),
  },
]);

loadExperiments();
</script>

<template>
  <PageHead :title="$t('testsPage.title')" :subtitle="$t('testsPage.intro')" :count="table.state.total" />
  <NAlert v-if="error || table.state.error" type="error" :title="error || table.state.error" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small">
    <template #header>
      <div class="filters">
        <label class="fld">
          <span>{{ $t("testsPage.experiment") }}</span>
          <NSelect
            v-model:value="experimentId"
            data-test="all-tests-experiment"
            filterable
            clearable
            :placeholder="$t('testsPage.allExperiments')"
            :options="experimentOptions"
          />
        </label>
        <label class="fld">
          <span>{{ $t("tests.status") }}</span>
          <NSelect v-model:value="status" data-test="all-tests-status" :options="statusOptions" />
        </label>
      </div>
    </template>
    <NDataTable
      remote
      :columns="columns"
      :data="table.state.rows"
      :loading="table.state.loading"
      :row-key="(row) => row.id"
      data-test="all-tests"
      :pagination="{
        page: table.state.page,
        pageSize: table.state.pageSize,
        itemCount: table.state.total,
        pageSizes: [20, 50, 100],
        showSizePicker: true,
        onUpdatePage: table.setPage,
        onUpdatePageSize: table.setPageSize,
      }"
    >
      <template #empty>
        <p class="muted empty">{{ $t("testsPage.empty") }}</p>
      </template>
    </NDataTable>
  </NCard>
</template>

<style scoped>
.filters { display: flex; gap: 14px; flex-wrap: wrap; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 220px; margin: 0; }
.fld > span { font-size: 12px; color: var(--muted); }
.fld :deep(.n-select) { width: 100%; }
.version { margin-left: 6px; }
.empty { text-align: center; margin: 12px 0; max-width: 460px; }
:deep(.link-muted) { color: var(--muted); text-decoration: none; }
:deep(.link-muted:hover) { color: var(--txt); text-decoration: underline; }
</style>
