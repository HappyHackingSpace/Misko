<script setup>
// Published results across an experiment. A row is one metric or one event,
// and it carries the run, the recording and the calibration it came from, so a
// number can always be traced back to the video it was measured on. Reading
// results needs no permission beyond being signed in.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";
import { experiments, reports } from "../api/endpoints.js";
import { getToken } from "../api/client.js";

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

const seconds = (us) => `${(us / 1_000_000).toFixed(2)} s`;
const number = (value) => (value === null || value === undefined ? t("reports.missing") : String(value));

const metricColumns = computed(() => [
  { key: "subjectCode", label: t("reports.subject"), sortable: true },
  { key: "paradigmKey", label: t("reports.paradigm") },
  { key: "scheduledAt", label: t("reports.scheduled"), sortable: true },
  { key: "metricKey", label: t("reports.metricKey"), sortable: true },
  { key: "value", label: t("reports.value"), sortable: true },
  { key: "unit", label: t("reports.unit"), cellClass: "muted" },
]);

const eventColumns = computed(() => [
  { key: "subjectCode", label: t("reports.subject"), sortable: true },
  { key: "eventType", label: t("reports.eventType"), sortable: true },
  { key: "kind", label: t("reports.kind"), cellClass: "muted" },
  { key: "startUs", label: t("reports.start") },
  { key: "endUs", label: t("reports.end") },
  { key: "confidence", label: t("reports.confidence"), cellClass: "muted" },
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
    const path = reports.exportPath(isMetrics.value ? "metrics" : "events", filters());
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
  <div class="head">
    <h1>{{ $t("reports.title") }}</h1>
  </div>
  <p class="muted intro">{{ $t("reports.intro") }}</p>
  <p class="err" v-if="error" data-test="report-error">{{ error }}</p>

  <div class="card filters">
    <label class="fld">
      <span>{{ $t("reports.experiment") }}</span>
      <select v-model="experimentId" data-test="report-experiment">
        <option value="">{{ $t("reports.pickExperiment") }}</option>
        <option v-for="experiment in experimentList" :key="experiment.id" :value="experiment.id">
          {{ experiment.code }} · {{ experiment.title }}
        </option>
      </select>
    </label>

    <label class="fld">
      <span>{{ isMetrics ? $t("reports.metricKey") : $t("reports.eventType") }}</span>
      <select v-if="isMetrics" v-model="metricKey" data-test="report-metric">
        <option value="">{{ $t("reports.allMetrics") }}</option>
        <option v-for="key in metricKeys" :key="key" :value="key">{{ key }}</option>
      </select>
      <select v-else v-model="eventType" data-test="report-event-type">
        <option value="">{{ $t("reports.allEvents") }}</option>
        <option v-for="type in eventTypes" :key="type" :value="type">{{ type }}</option>
      </select>
    </label>

    <label class="fld">
      <span>{{ $t("analysis.runs") }}</span>
      <select v-model="selection" data-test="report-selection">
        <option value="latest">{{ $t("reports.latestOnly") }}</option>
        <option value="all">{{ $t("reports.allRuns") }}</option>
      </select>
    </label>

    <div class="tabs">
      <button class="small" :class="{ active: isMetrics }" data-test="tab-metrics" @click="tab = 'metrics'">
        {{ $t("reports.metrics") }}
      </button>
      <button class="small" :class="{ active: !isMetrics }" data-test="tab-events" @click="tab = 'events'">
        {{ $t("reports.events") }}
      </button>
    </div>

    <button class="small" :disabled="exporting" data-test="report-export" @click="download">
      {{ exporting ? $t("reports.exporting") : $t("reports.export") }}
    </button>
  </div>

  <!-- The summary is the comparison the laboratory reads: one row per group of
       the experiment, on one metric. -->
  <div class="card" v-if="summary" data-test="report-summary">
    <h4>{{ $t("reports.summary") }} · {{ summary.metricKey }}<span class="muted" v-if="summary.version"> ({{ summary.version.unit }})</span></h4>
    <div class="table-scroll">
      <table class="rows">
        <thead>
          <tr>
            <th>{{ $t("reports.group") }}</th>
            <th>{{ $t("reports.tests") }}</th>
            <th>{{ $t("reports.subjects") }}</th>
            <th>{{ $t("reports.mean") }}</th>
            <th>{{ $t("reports.sd") }}</th>
            <th>{{ $t("reports.sem") }}</th>
            <th>{{ $t("reports.min") }}</th>
            <th>{{ $t("reports.max") }}</th>
            <th>{{ $t("reports.missingSubjects") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="(group, index) in summary.groups" :key="group.groupId || index" data-test="summary-group">
            <td>{{ group.groupName || $t("reports.ungrouped") }}</td>
            <td class="muted">{{ group.tests }}</td>
            <td class="muted">{{ group.subjects }}</td>
            <td data-test="summary-mean">{{ number(group.mean) }}</td>
            <td class="muted">{{ number(group.sd) }}</td>
            <td class="muted">{{ number(group.sem) }}</td>
            <td class="muted">{{ number(group.min) }}</td>
            <td class="muted">{{ number(group.max) }}</td>
            <td class="muted">{{ group.missingSubjects }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </div>
  <p class="muted hint" v-else-if="experimentId && isMetrics" data-test="summary-hint">{{ $t("reports.summaryHint") }}</p>

  <div class="card">
    <DataTable
      v-if="isMetrics"
      :columns="metricColumns"
      :rows="metricTable.state.rows"
      :total="metricTable.state.total"
      :page="metricTable.state.page"
      :page-size="metricTable.state.pageSize"
      :sort="metricTable.state.sort"
      :loading="metricTable.state.loading"
      :searchable="false"
      :exportable="false"
      :entity-label="$t('reports.metrics')"
      @page="metricTable.setPage"
      @page-size="metricTable.setPageSize"
      @sort="metricTable.setSort"
    >
      <template #cell-scheduledAt="{ row }">
        <span class="muted">{{ new Date(row.scheduledAt).toLocaleString() }}</span>
      </template>
      <!-- A measured value and a missing one are different answers, so the row
           says which it is rather than showing an empty cell. -->
      <template #cell-value="{ row }">
        <span :data-metric="row.metricKey" :data-missing="row.value === null" data-test="metric-value">
          {{ row.value === null ? $t("reports.missing") : row.value }}
        </span>
      </template>
    </DataTable>

    <DataTable
      v-else
      :columns="eventColumns"
      :rows="eventTable.state.rows"
      :total="eventTable.state.total"
      :page="eventTable.state.page"
      :page-size="eventTable.state.pageSize"
      :sort="eventTable.state.sort"
      :loading="eventTable.state.loading"
      :searchable="false"
      :exportable="false"
      :entity-label="$t('reports.events')"
      @page="eventTable.setPage"
      @page-size="eventTable.setPageSize"
      @sort="eventTable.setSort"
    >
      <template #cell-eventType="{ row }">
        <span :data-event="row.eventType" data-test="event-type">{{ row.eventType }}</span>
      </template>
      <template #cell-startUs="{ row }">{{ seconds(row.startUs) }}</template>
      <template #cell-endUs="{ row }">{{ seconds(row.endUs) }}</template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.intro { margin: 0 0 12px; max-width: 70ch; }
.filters { display: flex; gap: 14px; align-items: flex-end; flex-wrap: wrap; margin-bottom: 16px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld span { font-size: 12px; color: var(--muted); }
.tabs { display: flex; gap: 6px; }
.tabs .active { border-color: var(--accent); color: var(--accent); }
.small { padding: 4px 10px; font-size: .85rem; }
.hint { margin: 0 0 16px; }
.table-scroll { overflow-x: auto; }
.rows { width: 100%; border-collapse: collapse; }
.rows th { text-align: left; font-size: 12px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; padding: 6px 8px; }
.rows td { padding: 6px 8px; border-top: 1px solid var(--line); }
</style>
