<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t } = useI18n();

const table = useDataTable("/scenarios", { defaultSort: { field: "createdAt", order: "desc" } });

const paradigmsOf = (row) => [...new Set((row.environments || []).map((e) => e.paradigmKey))].join(", ");
const envCountOf = (row) => (row.environments || []).length;

const columns = computed(() => [
  { key: "name", label: t("common.name"), sortable: true },
  { key: "paradigms", label: t("scenarios.paradigms"), cellClass: "muted", exportValue: paradigmsOf },
  { key: "envCount", label: t("scenarios.environments"), cellClass: "muted", exportValue: envCountOf },
  { key: "description", label: t("common.description"), cellClass: "muted" },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("scenarios.title") }}</h1>
    <button class="primary" @click="$router.push('/scenarios/new')">{{ $t("common.create") }}</button>
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
      export-name="scenarios"
      :entity-label="$t('scenarios.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-name="{ row }"><RouterLink class="link" :to="`/scenarios/${row.id}`">{{ row.name }}</RouterLink></template>
      <template #cell-paradigms="{ row }">{{ paradigmsOf(row) || "-" }}</template>
      <template #cell-envCount="{ row }">{{ envCountOf(row) }}</template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
