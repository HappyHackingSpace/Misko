<script setup>
import { computed, h } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NButton, NCard, NDataTable } from "naive-ui";
import { useDataTable } from "../composables/useDataTable.js";
import { useTableExport } from "../composables/useTableExport.js";
import { useAuth } from "../stores/auth.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t } = useI18n();
const auth = useAuth();
// Creating a study needs study:write, so the control is offered only to a role
// the API would actually accept.
const canWrite = computed(() => auth.can("study:write"));
const table = useDataTable("/experiments", { defaultSort: { field: "createdAt", order: "desc" } });

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
    title: t("experiments.code"),
    key: "code",
    sorter: true,
    sortOrder: sortOrderFor("code"),
    render: (row) => h(RouterLink, { class: "link", to: `/experiments/${row.id}`, "data-test": "experiment-link" }, () => row.code),
  },
  { title: t("experiments.name"), key: "title", sorter: true, sortOrder: sortOrderFor("title") },
  { title: t("common.description"), key: "description", ellipsis: { tooltip: true }, render: (row) => row.description || "—" },
]);

const exportColumns = [
  { key: "code", label: t("experiments.code") },
  { key: "title", label: t("experiments.name") },
  { key: "description", label: t("common.description") },
];
const { exportCsv, exportPdf } = useTableExport({
  state: table.state,
  columns: exportColumns,
  exportName: "experiments",
  entityLabel: t("experiments.title"),
});
</script>

<template>
  <PageHead :title="$t('experiments.title')" :count="table.state.total">
    <template #actions>
      <NButton v-if="canWrite" type="primary" data-test="experiment-new" @click="$router.push('/experiments/new')">
        <template #icon>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
        </template>
        {{ $t("experiments.new") }}
      </NButton>
    </template>
  </PageHead>

  <NCard :bordered="true" size="small">
    <template #header>
      <ListToolbar :search="table.state.search" :export-disabled="!table.state.rows.length" @update:search="table.setSearch" @csv="exportCsv" @pdf="exportPdf" />
    </template>
    <NDataTable
      remote
      :columns="columns"
      :data="table.state.rows"
      :loading="table.state.loading"
      :row-key="(row) => row.id"
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
    />
  </NCard>
</template>
