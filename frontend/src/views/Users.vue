<script setup>
import { computed, h } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NButton, NCard, NDataTable } from "naive-ui";
import { useDataTable } from "../composables/useDataTable.js";
import { useTableExport } from "../composables/useTableExport.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t, locale } = useI18n();
const table = useDataTable("/users", { defaultSort: { field: "createdAt", order: "desc" } });

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
    title: t("common.name"),
    key: "name",
    sorter: true,
    sortOrder: sortOrderFor("name"),
    render: (row) => h(RouterLink, { class: "link", to: `/users/${row.id}` }, () => row.name),
  },
  { title: t("users.email"), key: "email", sorter: true, sortOrder: sortOrderFor("email") },
  { title: t("users.role"), key: "role", sorter: true, sortOrder: sortOrderFor("role"), render: (row) => t(`roles.${row.role}`) },
  {
    title: t("common.addedAt"),
    key: "createdAt",
    sorter: true,
    sortOrder: sortOrderFor("createdAt"),
    render: (row) => new Date(row.createdAt).toLocaleDateString(locale.value),
  },
]);

const exportColumns = [
  { key: "name", label: t("common.name") },
  { key: "email", label: t("users.email") },
  { key: "role", label: t("users.role"), value: (row) => t(`roles.${row.role}`) },
  { key: "createdAt", label: t("common.addedAt"), value: (row) => new Date(row.createdAt).toLocaleDateString(locale.value) },
];
const { exportCsv, exportPdf } = useTableExport({
  state: table.state,
  columns: exportColumns,
  exportName: "users",
  entityLabel: t("users.title"),
});
</script>

<template>
  <PageHead :title="$t('users.title')" :count="table.state.total">
    <template #actions>
      <NButton type="primary" @click="$router.push('/users/new')">
        <template #icon>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
        </template>
        {{ $t("common.create") }}
      </NButton>
    </template>
  </PageHead>
  <p class="muted intro">{{ $t("users.intro") }}</p>

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

<style scoped>
.intro { margin: -8px 0 18px; }
</style>
