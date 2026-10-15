<script setup>
// The test worklist across studies: what is filmed today, what is overdue
// (planned or still running from an earlier day) and what comes next. The view
// and the filters live in the address, so a link opens the same list. Days are
// the laboratory's days, as the API counts them.
import { computed, h, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { useI18n } from "vue-i18n";
import { NAlert, NBadge, NCard, NDataTable, NSelect, NTabPane, NTabs, NTag } from "naive-ui";
import { lookups, tests } from "../api/tests.js";
import { addDays, dateIn, tagType, useExecutionFormat } from "../composables/useExecutionFormat.js";
import { useParadigmNames } from "../composables/useParadigmNames.js";
import { filterByText, selectOption } from "../composables/selectOptions.js";
import PageHead from "../components/PageHead.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const fmt = useExecutionFormat();
const { paradigmName, names: paradigmNames } = useParadigmNames();

const VIEWS = ["today", "overdue", "upcoming", "all"];
const STATUSES = ["PLANNED", "IN_PROGRESS", "COMPLETED", "CANCELLED"];
const OPEN = ["PLANNED", "IN_PROGRESS"];

// The address is the state: ?view=&experimentId=&status=&paradigmKey=&page=
const view = computed(() => (VIEWS.includes(route.query.view) ? route.query.view : "today"));
const filters = computed(() => ({
  experimentId: route.query.experimentId || null,
  status: STATUSES.includes(route.query.status) ? route.query.status : null,
  paradigmKey: route.query.paradigmKey || null,
}));
const page = computed(() => Math.max(1, Number(route.query.page) || 1));
const pageSize = ref(20);

function setQuery(changes) {
  const query = { ...route.query, ...changes };
  if (!("page" in changes)) delete query.page;
  for (const key of Object.keys(query)) if (query[key] === null || query[key] === "" || query[key] === undefined) delete query[key];
  router.replace({ query });
}

const today = ref(null);
const rows = ref([]);
const total = ref(0);
const counts = ref({ today: null, overdue: null, upcoming: null });
const loading = ref(false);
const error = ref("");
const experiments = ref([]);

function range(name) {
  if (name === "today") return { from: today.value, to: today.value };
  if (name === "overdue") return { to: addDays(today.value, -1) };
  if (name === "upcoming") return { from: addDays(today.value, 1) };
  return {};
}

// Overdue means not finished: planned or in progress, from an earlier day.
const overdueStatus = () => filters.value.status || OPEN.join(",");
// Today, overdue and upcoming work reads best earliest first; "all" newest first.
const order = (name) => (name === "all" ? "desc" : "asc");

let seq = 0;
async function load() {
  if (!today.value) return;
  const mine = ++seq;
  loading.value = true;
  error.value = "";
  try {
    const base = { experimentId: filters.value.experimentId, paradigmKey: filters.value.paradigmKey };
    const status = view.value === "overdue" ? overdueStatus() : filters.value.status;
    const result = await tests.worklist({
      ...base, ...range(view.value), status, order: order(view.value), page: page.value, pageSize: pageSize.value,
    });
    if (mine !== seq) return;
    rows.value = result.data || [];
    total.value = result.total || 0;
    loadCounts(base);
  } catch (e) {
    if (mine === seq) error.value = e.message;
  } finally {
    if (mine === seq) loading.value = false;
  }
}

async function loadCounts(base) {
  try {
    const status = filters.value.status;
    const [todayPage, overduePage, upcomingPage] = await Promise.all([
      tests.worklist({ ...base, ...range("today"), status, pageSize: 1 }),
      tests.worklist({ ...base, ...range("overdue"), status: overdueStatus(), pageSize: 1 }),
      tests.worklist({ ...base, ...range("upcoming"), status, pageSize: 1 }),
    ]);
    counts.value = { today: todayPage.total, overdue: overduePage.total, upcoming: upcomingPage.total };
  } catch {
    // The counts are a hint; the list itself reports errors.
  }
}

watch(() => route.query, load);

function changePageSize(size) {
  pageSize.value = size;
  if (page.value === 1) load();
  else setQuery({ page: 1 });
}

const experimentOptions = computed(() => experiments.value.map((e) => selectOption("worklist-experiment", e.id, `${e.code} · ${e.title}`)));
const statusOptions = computed(() => STATUSES.map((s) => selectOption("worklist-status", s, fmt.status(s))));
const paradigmOptions = computed(() =>
  Object.entries(paradigmNames.value)
    .map(([key, name]) => selectOption("worklist-paradigm", key, name))
    .sort((a, b) => a.text.localeCompare(b.text)),
);

const columns = computed(() => [
  {
    title: t("execution.tests.scheduledAt"),
    key: "scheduledAt",
    width: 170,
    render: (row) => h(RouterLink, { class: "link", to: `/tests/${row.id}`, "data-test": "worklist-link" }, () => fmt.dateTime(row.scheduledAt)),
  },
  {
    title: t("execution.experiment"),
    key: "experimentCode",
    render: (row) => h(RouterLink, { class: "link-muted", to: `/experiments/${row.experimentId}?tab=tests` }, () => row.experimentCode),
  },
  {
    title: t("execution.timepoint"),
    key: "timepointName",
    render: (row) => h("div", [h("div", row.timepointName), h("div", { class: "muted small" }, t("execution.studyDay", { day: row.studyDay }))]),
  },
  {
    title: t("execution.paradigm"),
    key: "paradigmKey",
    render: (row) => h("div", [h("div", paradigmName(row.paradigmKey)), h("div", { class: "muted small" }, row.protocolName)]),
  },
  {
    title: t("execution.tests.animals"),
    key: "subjects",
    render: (row) =>
      h("div", { class: "chips" }, row.subjects.map((s) => h(NTag, { size: "small", bordered: false }, () => `${s.subjectCode} · ${s.environmentName}`))),
  },
  {
    title: t("execution.status"),
    key: "status",
    width: 130,
    render: (row) => h(NTag, { size: "small", round: true, type: tagType(row.status), "data-test": "worklist-status", "data-status": row.status }, () => fmt.status(row.status)),
  },
]);

onMounted(async () => {
  try {
    const lab = await lookups.lab();
    today.value = dateIn(new Date(), lab.timezone);
  } catch {
    today.value = dateIn(new Date());
  }
  load();
  try {
    experiments.value = (await lookups.experiments({ pageSize: 100 })).data || [];
  } catch {
    // The filter falls back to an empty list.
  }
});
</script>

<template>
  <PageHead :title="$t('execution.worklist.title')" :subtitle="$t('execution.worklist.intro')" :count="total" />
  <NAlert v-if="error" type="error" :title="error" style="margin-bottom: 16px" data-test="worklist-error" />

  <NCard size="small" :bordered="true">
    <div class="filters">
      <NSelect
        :value="filters.experimentId"
        filterable
        clearable
        :filter="filterByText"
        :placeholder="$t('execution.worklist.allExperiments')"
        :options="experimentOptions"
        data-test="worklist-experiment"
        @update:value="(v) => setQuery({ experimentId: v })"
      />
      <NSelect
        :value="filters.status"
        clearable
        :placeholder="$t('execution.worklist.allStatuses')"
        :options="statusOptions"
        data-test="worklist-status-filter"
        @update:value="(v) => setQuery({ status: v })"
      />
      <NSelect
        :value="filters.paradigmKey"
        clearable
        filterable
        :filter="filterByText"
        :placeholder="$t('execution.worklist.allParadigms')"
        :options="paradigmOptions"
        data-test="worklist-paradigm"
        @update:value="(v) => setQuery({ paradigmKey: v })"
      />
    </div>
    <NTabs :value="view" type="line" @update:value="(v) => setQuery({ view: v })">
      <NTabPane v-for="name in VIEWS" :key="name" :name="name">
        <template #tab>
          <span :data-test="`worklist-view-${name}`">{{ $t(`execution.worklist.views.${name}`) }}</span>
          <NBadge
            v-if="name !== 'all' && counts[name]"
            :value="counts[name]"
            :type="name === 'overdue' ? 'error' : 'info'"
            class="badge"
            :data-test="`worklist-count-${name}`"
          />
        </template>
      </NTabPane>
    </NTabs>
    <NDataTable
      remote
      :columns="columns"
      :data="rows"
      :loading="loading"
      :row-key="(row) => row.id"
      :scroll-x="900"
      data-test="worklist"
      :pagination="{
        page,
        pageSize,
        itemCount: total,
        pageSizes: [20, 50, 100],
        showSizePicker: true,
        onUpdatePage: (p) => setQuery({ page: p }),
        onUpdatePageSize: changePageSize,
      }"
    >
      <template #empty>
        <p class="muted empty">{{ $t(`execution.worklist.empty.${view}`) }}</p>
      </template>
    </NDataTable>
  </NCard>
</template>

<style scoped>
.filters { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 10px; margin-bottom: 8px; }
.badge { margin-left: 6px; }
.empty { text-align: center; margin: 12px 0; }
:deep(.chips) { display: flex; gap: 4px; flex-wrap: wrap; }
:deep(.small) { font-size: 12px; }
:deep(.link-muted) { color: var(--muted); text-decoration: none; }
:deep(.link-muted:hover) { color: var(--txt); text-decoration: underline; }
</style>
