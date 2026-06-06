<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t } = useI18n();

const table = useDataTable("/subjects", { defaultSort: { field: "createdAt", order: "desc" } });

const statusLabel = (row) => (row.status ? t(`subjects.status${row.status}`) : "");

const columns = computed(() => [
  { key: "code", label: t("subjects.code"), sortable: true },
  {
    key: "sex", label: t("subjects.sex"), sortable: true,
    exportValue: (row) => (row.sex === "F" ? t("subjects.female") : t("subjects.male")),
  },
  { key: "strain", label: t("subjects.strain"), sortable: true, cellClass: "muted" },
  { key: "genotype", label: t("subjects.genotype"), cellClass: "muted" },
  {
    key: "status", label: t("subjects.status"), sortable: true,
    exportValue: statusLabel,
  },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("subjects.title") }}</h1>
    <button class="primary" @click="$router.push('/subjects/new')">{{ $t("common.create") }}</button>
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
      export-name="subjects"
      :entity-label="$t('subjects.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-code="{ row }"><RouterLink class="link" :to="`/subjects/${row.id}`">{{ row.code }}</RouterLink></template>
      <template #cell-sex="{ row }">{{ row.sex === "F" ? $t("subjects.female") : $t("subjects.male") }}</template>
      <template #cell-status="{ row }">{{ statusLabel(row) }}</template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
