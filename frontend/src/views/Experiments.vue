<script setup>
import { computed, h, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NButton, NCard, NDataTable, NSelect, NTag } from "naive-ui";
import { useDataTable } from "../composables/useDataTable.js";
import { useTableExport } from "../composables/useTableExport.js";
import { useAuth } from "../stores/auth.js";
import { useDesignFormat } from "./experiments/design/shared.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t } = useI18n();
const auth = useAuth();
const fmt = useDesignFormat();
// Creating a study needs study:write, so the control is offered only to a role
// the API would actually accept.
const canWrite = computed(() => auth.can("study:write"));
const status = ref(null);
const table = useDataTable("/experiments", {
  defaultSort: { field: "createdAt", order: "desc" },
  extraParams: () => ({ status: status.value }),
});

const STATUS_TAG = { DRAFT: "default", RUNNING: "info", COMPLETED: "success" };
const statusOptions = computed(() => ["DRAFT", "RUNNING", "COMPLETED"].map((s) => ({ value: s, label: t(`experiments.status.${s}`) })));
function onStatus(value) {
  status.value = value;
  table.setPage(1);
}

function sortOrderFor(key) {
  if (table.state.sort.field !== key) return false;
  return table.state.sort.order === "asc" ? "ascend" : "descend";
}
function onUpdateSorter(sorter) {
  if (!sorter || !sorter.order) return;
  table.setSort({ field: sorter.columnKey, order: sorter.order === "ascend" ? "asc" : "desc" });
}

const columns = computed(() => [
  {
    title: t("design.form.code"),
    key: "code",
    sorter: true,
    sortOrder: sortOrderFor("code"),
    render: (row) => h(RouterLink, { class: "link", to: `/experiments/${row.id}`, "data-test": "experiment-link" }, () => row.code),
  },
  { title: t("design.form.title"), key: "title", sorter: true, sortOrder: sortOrderFor("title"), ellipsis: { tooltip: true } },
  {
    title: t("design.list.status"),
    key: "status",
    render: (row) => h(NTag, { size: "small", round: true, bordered: false, type: STATUS_TAG[row.status] }, () => t(`experiments.status.${row.status}`)),
  },
  { title: t("design.list.dayZero"), key: "dayZero", render: (row) => (row.dayZero ? fmt.date(row.dayZero) : "-") },
  {
    title: t("design.list.createdAt"),
    key: "createdAt",
    sorter: true,
    sortOrder: sortOrderFor("createdAt"),
    render: (row) => fmt.dateTime(row.createdAt),
  },
]);

const exportColumns = [
  { key: "code", label: t("design.form.code") },
  { key: "title", label: t("design.form.title") },
  { key: "status", label: t("design.list.status"), value: (row) => t(`experiments.status.${row.status}`) },
  { key: "dayZero", label: t("design.list.dayZero"), value: (row) => (row.dayZero ? fmt.date(row.dayZero) : "") },
  { key: "hypothesis", label: t("design.form.hypothesis") },
  { key: "description", label: t("common.description") },
];
const { exportCsv, exportPdf } = useTableExport({
  state: table.state,
  columns: exportColumns,
  exportName: "experiments",
  entityLabel: t("nav.experiments"),
});
</script>

<template>
  <PageHead :title="$t('nav.experiments')" :count="table.state.total">
    <template #actions>
      <NButton v-if="canWrite" type="primary" data-test="experiment-new" @click="$router.push('/experiments/new')">
        <template #icon>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
        </template>
        {{ $t("design.form.new") }}
      </NButton>
    </template>
  </PageHead>

  <NCard :bordered="true" size="small">
    <template #header>
      <div class="toolbar">
        <ListToolbar class="grow" :search="table.state.search" :export-disabled="!table.state.rows.length" @update:search="table.setSearch" @csv="exportCsv" @pdf="exportPdf" />
        <NSelect
          class="status"
          :value="status"
          :options="statusOptions"
          clearable
          :placeholder="$t('design.list.allStatuses')"
          data-test="experiment-status-filter"
          @update:value="onStatus"
        />
      </div>
    </template>
    <p v-if="table.state.error" class="error">{{ table.state.error }}</p>
    <NDataTable
      remote
      :columns="columns"
      :data="table.state.rows"
      :loading="table.state.loading"
      :row-key="(row) => row.id"
      :scroll-x="720"
      :pagination="{
        page: table.state.page,
        pageSize: table.state.pageSize,
        itemCount: table.state.total,
        pageSizes: [10, 20, 50],
        showSizePicker: true,
        onUpdatePage: table.setPage,
        onUpdatePageSize: table.setPageSize,
      }"
      @update:sorter="onUpdateSorter"
    >
      <template #empty>
        <span class="muted">{{ table.state.search || status ? $t("design.list.noMatch") : $t("design.list.empty") }}</span>
      </template>
    </NDataTable>
  </NCard>
</template>

<style scoped>
.toolbar { display: flex; gap: 12px; align-items: center; flex-wrap: wrap; }
.grow { flex: 1; min-width: 240px; }
.status { width: 180px; }
.error { color: var(--bad); margin: 0 0 10px; }
@media (max-width: 520px) {
  .status { width: 100%; }
}
</style>
