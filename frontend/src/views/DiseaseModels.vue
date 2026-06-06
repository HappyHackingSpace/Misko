<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t } = useI18n();

const table = useDataTable("/disease-models", { defaultSort: { field: "createdAt", order: "desc" } });

const columns = computed(() => [
  { key: "key", label: t("diseaseModels.key"), sortable: true },
  { key: "name", label: t("common.name"), sortable: true },
  { key: "category", label: t("diseaseModels.category"), sortable: true, cellClass: "muted" },
  { key: "description", label: t("common.notes"), cellClass: "muted" },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("diseaseModels.title") }}</h1>
    <button class="primary" @click="$router.push('/disease-models/new')">{{ $t("common.create") }}</button>
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
      export-name="disease-models"
      :entity-label="$t('diseaseModels.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-key="{ row }"><RouterLink class="link" :to="`/disease-models/${row.id}`">{{ row.key }}</RouterLink></template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
