<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t } = useI18n();
const table = useDataTable("/experiments", { defaultSort: { field: "createdAt", order: "desc" } });

const columns = computed(() => [
  { key: "code", label: t("experiments.code"), sortable: true },
  { key: "title", label: t("experiments.name"), sortable: true },
  { key: "description", label: t("common.description"), cellClass: "muted" },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("experiments.title") }}</h1>
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
      export-name="experiments"
      :entity-label="$t('experiments.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-code="{ row }">
        <RouterLink class="link" :to="`/experiments/${row.id}`" data-test="experiment-link">{{ row.code }}</RouterLink>
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
.link { color: var(--accent); font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
