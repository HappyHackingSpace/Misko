<script setup>
import { computed, watch } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import LabTabs from "../components/LabTabs.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t, locale } = useI18n();

const table = useDataTable("/paradigms", {
  defaultSort: { field: "name", order: "asc" },
  extraParams: () => ({ lang: locale.value }),
});

const columns = computed(() => [
  { key: "name", label: t("common.name"), sortable: true },
  {
    key: "category", label: t("paradigms.category"), sortable: true,
    exportValue: (row) => t("paradigms.categories." + row.category),
  },
  { key: "metricCount", label: t("paradigms.metricsShort"), sortable: true },
]);

// Re-fetch with localized labels when the language changes.
watch(locale, () => table.reload());
</script>

<template>
  <div class="lab-head">
    <span class="lab-head-side"></span>
    <LabTabs />
    <span class="lab-head-side"></span>
  </div>
  <p class="muted" style="margin-top:0">{{ $t("paradigms.intro") }}</p>

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
      row-key="key"
      export-name="paradigms"
      :entity-label="$t('paradigms.title')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-name="{ row }">
        <RouterLink class="link" :to="`/paradigms/${row.key}`">{{ row.name }}</RouterLink>
      </template>
      <template #cell-category="{ row }">{{ $t("paradigms.categories." + row.category) }}</template>
      <template #cell-metricCount="{ row }">
        <span class="pill">{{ row.metricCount }} {{ $t("paradigms.metricsShort") }}</span>
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
.lab-head { display: flex; align-items: center; margin-bottom: 12px; }
.lab-head-side { flex: 1; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
