<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t, locale } = useI18n();

const table = useDataTable("/devices", { defaultSort: { field: "createdAt", order: "desc" } });

const columns = computed(() => [
  { key: "name", label: t("common.name"), sortable: true },
  {
    key: "platform", label: t("devices.platform"), sortable: true, cellClass: "muted",
    exportValue: (row) => (row.platform === "ios" ? "iOS" : "Android"),
  },
  {
    key: "createdAt", label: t("common.addedAt"), sortable: true, cellClass: "muted",
    exportValue: (row) => new Date(row.createdAt).toLocaleDateString(locale.value),
  },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("devices.title") }}</h1>
    <button class="primary" @click="$router.push('/devices/new')">{{ $t("common.create") }}</button>
  </div>

  <div class="card">
    <DataTable
      :columns="columns"
      :rows="table.state.rows"
      :total="table.state.total"
      :page="table.state.page"
      :page-size="table.state.pageSize"
      :sort="table.state.sort"
      :search="table.state.search"
      :loading="table.state.loading"
      export-name="devices"
      :entity-label="$t('devices.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-name="{ row }"><RouterLink class="link" :to="`/devices/${row.id}`">{{ row.name }}</RouterLink></template>
      <template #cell-platform="{ row }">{{ row.platform === "ios" ? "iOS" : "Android" }}</template>
      <template #cell-createdAt="{ row }">{{ new Date(row.createdAt).toLocaleDateString(locale) }}</template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
