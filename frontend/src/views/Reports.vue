<script setup>
// Published results across an experiment. A row is one metric or one event,
// and it carries the run, the recording and the calibration it came from, so a
// number can always be traced back to the video it was measured on. Reading
// results needs no permission beyond being signed in.
import { computed, h, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDataTable, NRadioButton, NRadioGroup, NSelect } from "naive-ui";
import { useDataTable } from "../composables/useDataTable.js";
import { experiments, reports } from "../api/endpoints.js";
import { getToken } from "../api/client.js";
import PageHead from "../components/PageHead.vue";

const { t } = useI18n();

const experimentList = ref([]);
const experimentId = ref("");
const tab = ref("metrics");
const metricKey = ref("");
const eventType = ref("");
// The API reports the latest run of each test by default; "all" keeps every run,
// which is what a reanalysis is compared against.
const selection = ref("latest");
const summary = ref(null);
const error = ref("");
const exporting = ref(false);

const isMetrics = computed(() => tab.value === "metrics");

// The filters travel as query parameters on every request, including the export.
const filters = () => ({
  experimentId: experimentId.value,
  metricKey: isMetrics.value ? metricKey.value : "",
  eventType: isMetrics.value ? "" : eventType.value,
  selection: selection.value,
});

// The default sort of the shared table is createdAt, which these endpoints do
// not accept; they sort by the time the test was scheduled.
const sort = { field: "scheduledAt", order: "desc" };
const metricTable = useDataTable("/reports/metrics", { defaultSort: sort, pageSize: 20, extraParams: filters });
const eventTable = useDataTable("/reports/events", { defaultSort: sort, pageSize: 20, extraParams: filters });

// The keys on offer are the ones this experiment actually published, rather
// than every key the paradigm defines.
const metricKeys = computed(() => [...new Set(metricTable.state.rows.map((r) => r.metricKey))].sort());
const eventTypes = computed(() => [...new Set(eventTable.state.rows.map((r) => r.eventType))].sort());

// The label carries a stable test hook: Naive UI's dropdown options are not a
// native <select>, so the browser suite can no longer address them by value.
function testLabel(text, testId) {
  return () => h("span", { "data-test": testId }, text);
}

const experimentOptions = computed(() =>
  experimentList.value.map((e) => ({ value: e.id, label: testLabel(`${e.code} · ${e.title}`, `option-experiment-${e.id}`) })),
);
const metricKeyOptions = computed(() => [
  { value: "", label: testLabel(t("reports.allMetrics"), "option-metric-all") },
  ...metricKeys.value.map((k) => ({ value: k, label: testLabel(k, `option-metric-${k}`) })),
]);
const eventTypeOptions = computed(() => [
  { value: "", label: testLabel(t("reports.allEvents"), "option-event-type-all") },
  ...eventTypes.value.map((k) => ({ value: k, label: testLabel(k, `option-event-type-${k}`) })),
]);
const selectionOptions = computed(() => [
  { value: "latest", label: testLabel(t("reports.latestOnly"), "option-selection-latest") },
  { value: "all", label: testLabel(t("reports.allRuns"), "option-selection-all") },
]);

const seconds = (us) => `${(us / 1_000_000).toFixed(2)} s`;
const number = (value) => (value === null || value === undefined ? t("reports.missing") : String(value));

function sortOrderFor(table, key) {
  if (table.state.sort.field !== key) return false;
  return table.state.sort.order === "asc" ? "ascend" : "descend";
}
function onUpdateSorter(table, sorter) {
  if (!sorter || !sorter.order) return;
  table.setSort({ field: sorter.columnKey, order: sorter.order === "ascend" ? "asc" : "desc" });
}

const metricColumns = computed(() => [
  { title: t("reports.subject"), key: "subjectCode", sorter: true, sortOrder: sortOrderFor(metricTable, "subjectCode") },
  { title: t("reports.paradigm"), key: "paradigmKey" },
  {
    title: t("reports.scheduled"),
    key: "scheduledAt",
    sorter: true,
    sortOrder: sortOrderFor(metricTable, "scheduledAt"),
    render: (row) => new Date(row.scheduledAt).toLocaleString(),
  },
  { title: t("reports.metricKey"), key: "metricKey", sorter: true, sortOrder: sortOrderFor(metricTable, "metricKey") },
  {
    title: t("reports.value"),
    key: "value",
    sorter: true,
    sortOrder: sortOrderFor(metricTable, "value"),
    render: (row) =>
      h(
        "span",
        { "data-metric": row.metricKey, "data-missing": row.value === null, "data-test": "metric-value" },
        row.value === null ? t("reports.missing") : String(row.value),
      ),
  },
  { title: t("reports.unit"), key: "unit" },
]);

const eventColumns = computed(() => [
  { title: t("reports.subject"), key: "subjectCode", sorter: true, sortOrder: sortOrderFor(eventTable, "subjectCode") },
  {
    title: t("reports.eventType"),
    key: "eventType",
    sorter: true,
    sortOrder: sortOrderFor(eventTable, "eventType"),
    render: (row) => h("span", { "data-event": row.eventType, "data-test": "event-type" }, row.eventType),
  },
  { title: t("reports.kind"), key: "kind" },
  { title: t("reports.start"), key: "startUs", render: (row) => seconds(row.startUs) },
  { title: t("reports.end"), key: "endUs", render: (row) => seconds(row.endUs) },
  { title: t("reports.confidence"), key: "confidence" },
]);

const summaryColumns = computed(() => [
  { title: t("reports.group"), key: "groupName", render: (row) => row.groupName || t("reports.ungrouped") },
  { title: t("reports.tests"), key: "tests" },
  { title: t("reports.subjects"), key: "subjects" },
  { title: t("reports.mean"), key: "mean", render: (row) => h("span", { "data-test": "summary-mean" }, number(row.mean)) },
  { title: t("reports.sd"), key: "sd", render: (row) => number(row.sd) },
  { title: t("reports.sem"), key: "sem", render: (row) => number(row.sem) },
  { title: t("reports.min"), key: "min", render: (row) => number(row.min) },
  { title: t("reports.max"), key: "max", render: (row) => number(row.max) },
  { title: t("reports.missingSubjects"), key: "missingSubjects" },
]);

async function loadExperiments() {
  try {
    experimentList.value = (await experiments.list({ pageSize: 100 })).data || [];
  } catch (e) {
    error.value = e.message;
  }
}

// The summary compares the groups of one experiment on one metric, so it is
// asked for only once both are chosen. It reports the latest run of each test.
async function loadSummary() {
  summary.value = null;
  if (!experimentId.value || !metricKey.value || selection.value !== "latest") return;
  try {
    summary.value = await reports.summary({ experimentId: experimentId.value, metricKey: metricKey.value });
  } catch (e) {
    error.value = e.message;
  }
}

function refresh() {
  error.value = "";
  metricTable.setPage(1);
  eventTable.setPage(1);
  loadSummary();
}

// The export returns every matching row, not the page on screen, and the
// session token travels in a header, so the file is fetched and then saved.
async function download() {
  error.value = "";
  exporting.value = true;
  try {
    const path = reports.exportPath(isMetrics.value ? "metrics" : "events", { ...filters(), excel: "1" });
    const response = await fetch(`/api${path}`, { headers: { Authorization: `Bearer ${getToken()}` } });
    if (!response.ok) throw new Error(t("errors.common.requestFailed", { status: response.status }));
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.href = url;
    link.download = `misko-${isMetrics.value ? "metrics" : "events"}-${new Date().toISOString().slice(0, 10)}.csv`;
    document.body.appendChild(link);
    link.click();
    link.remove();
    URL.revokeObjectURL(url);
  } catch (e) {
    error.value = e.message;
  } finally {
    exporting.value = false;
  }
}

watch([experimentId, selection], refresh);
watch(metricKey, () => {
  metricTable.setPage(1);
  loadSummary();
});
watch(eventType, () => eventTable.setPage(1));

loadExperiments();
</script>

<template>
  <PageHead :title="$t('reports.title')" :subtitle="$t('reports.intro')" />
  <NAlert v-if="error" type="error" :title="error" data-test="report-error" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small" class="filters-card">
    <div class="filters">
      <label class="fld">
        <span>{{ $t("reports.experiment") }}</span>
        <NSelect
          v-model:value="experimentId"
          data-test="report-experiment"
          filterable
          clearable
          :placeholder="$t('reports.pickExperiment')"
          :options="experimentOptions"
        />
      </label>

      <label class="fld">
        <span>{{ isMetrics ? $t("reports.metricKey") : $t("reports.eventType") }}</span>
        <NSelect v-if="isMetrics" v-model:value="metricKey" data-test="report-metric" :options="metricKeyOptions" />
        <NSelect v-else v-model:value="eventType" data-test="report-event-type" :options="eventTypeOptions" />
      </label>

      <label class="fld">
        <span>{{ $t("analysis.runs") }}</span>
        <NSelect v-model:value="selection" data-test="report-selection" :options="selectionOptions" />
      </label>

      <div class="fld fld-tabs">
        <span>{{ $t("reports.view") }}</span>
        <NRadioGroup v-model:value="tab" class="tabs">
          <NRadioButton value="metrics" data-test="tab-metrics">{{ $t("reports.metrics") }}</NRadioButton>
          <NRadioButton value="events" data-test="tab-events">{{ $t("reports.events") }}</NRadioButton>
        </NRadioGroup>
      </div>

      <NButton type="primary" class="export-btn" :loading="exporting" data-test="report-export" @click="download">
        <template #icon>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 3v12M7 10l5 5 5-5" /><path d="M5 21h14" /></svg>
        </template>
        {{ exporting ? $t("reports.exporting") : $t("reports.export") }}
      </NButton>
    </div>
  </NCard>

  <!-- The summary is the comparison the laboratory reads: one row per group of
       the experiment, on one metric. -->
  <NCard v-if="summary" :bordered="true" size="small" class="summary-card" data-test="report-summary">
    <template #header>
      {{ $t("reports.summary") }} · {{ summary.metricKey }}<span class="muted" v-if="summary.version"> ({{ summary.version.unit }})</span>
    </template>
    <NDataTable
      :columns="summaryColumns"
      :data="summary.groups"
      :row-key="(row, i) => row.groupId || i"
      :row-props="() => ({ 'data-test': 'summary-group' })"
    />
  </NCard>
  <p class="muted hint" v-else-if="experimentId && isMetrics" data-test="summary-hint">{{ $t("reports.summaryHint") }}</p>

  <NCard :bordered="true" size="small">
    <NDataTable
      v-if="isMetrics"
      remote
      :columns="metricColumns"
      :data="metricTable.state.rows"
      :loading="metricTable.state.loading"
      :row-key="(row) => row.id"
      :pagination="{
        page: metricTable.state.page,
        pageSize: metricTable.state.pageSize,
        itemCount: metricTable.state.total,
        pageSizes: [20, 50, 100],
        showSizePicker: true,
        onUpdatePage: metricTable.setPage,
        onUpdatePageSize: metricTable.setPageSize,
      }"
      @update:sorter="(s) => onUpdateSorter(metricTable, s)"
    />
    <NDataTable
      v-else
      remote
      :columns="eventColumns"
      :data="eventTable.state.rows"
      :loading="eventTable.state.loading"
      :row-key="(row) => row.id"
      :pagination="{
        page: eventTable.state.page,
        pageSize: eventTable.state.pageSize,
        itemCount: eventTable.state.total,
        pageSizes: [20, 50, 100],
        showSizePicker: true,
        onUpdatePage: eventTable.setPage,
        onUpdatePageSize: eventTable.setPageSize,
      }"
      @update:sorter="(s) => onUpdateSorter(eventTable, s)"
    />
  </NCard>
</template>

<style scoped>
.filters-card, .summary-card { margin-bottom: 16px; }
.filters { display: flex; gap: 14px; align-items: flex-end; flex-wrap: wrap; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 180px; }
.fld > span { font-size: 12px; color: var(--muted); }
.fld :deep(.n-select) { width: 100%; }
.fld-tabs { min-width: 0; }
/* The global label rule in style.css makes these block-level with a bottom
   margin, which breaks the segmented control; put them back inline. */
.tabs :deep(.n-radio-button) { display: inline-flex; align-items: center; margin-bottom: 0; font-size: 14px; }
.export-btn { margin-left: auto; font-weight: 600; }
@media (max-width: 720px) { .export-btn { margin-left: 0; width: 100%; } }
.hint { margin: 0 0 16px; }
</style>
