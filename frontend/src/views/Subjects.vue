<script setup>
// Laboratory animals, read only: subjects are created where they are enrolled.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t } = useI18n();
const table = useDataTable("/subjects", { defaultSort: { field: "createdAt", order: "desc" } });

const sexLabel = (sex) => t(`subjects.sex${sex.charAt(0) + sex.slice(1).toLowerCase()}`);

const columns = computed(() => [
  { key: "code", label: t("subjects.code"), sortable: true },
  { key: "species", label: t("subjects.species"), sortable: true },
  { key: "sex", label: t("subjects.sex"), sortable: true, exportValue: (row) => sexLabel(row.sex) },
  { key: "strain", label: t("subjects.strain"), cellClass: "muted" },
  { key: "notes", label: t("common.notes"), cellClass: "muted" },
]);
</script>

<template>
  <div class="head">
    <h1>{{ $t("subjects.title") }}</h1>
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
      <template #cell-sex="{ row }">{{ sexLabel(row.sex) }}</template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }
</style>
