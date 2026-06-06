<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t } = useI18n();

const table = useDataTable("/treatments", { defaultSort: { field: "createdAt", order: "desc" } });

const doseLabel = (row) => (row.defaultDose != null ? `${row.defaultDose}${row.unit ? " " + row.unit : ""}` : "");

const columns = computed(() => [
  { key: "key", label: t("treatments.key"), sortable: true },
  { key: "name", label: t("common.name"), sortable: true },
  { key: "route", label: t("treatments.route"), sortable: true, cellClass: "muted" },
  { key: "defaultDose", label: t("treatments.defaultDose"), cellClass: "muted", exportValue: doseLabel },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("treatments.title") }}</h1>
    <button class="primary" @click="$router.push('/treatments/new')">{{ $t("common.create") }}</button>
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
      export-name="treatments"
      :entity-label="$t('treatments.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-key="{ row }"><RouterLink class="link" :to="`/treatments/${row.id}`">{{ row.key }}</RouterLink></template>
      <template #cell-defaultDose="{ row }">{{ doseLabel(row) }}</template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
