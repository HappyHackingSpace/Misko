<script setup>
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import DataTable from "../components/DataTable.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { t } = useI18n();

const table = useDataTable("/tests", { defaultSort: { field: "createdAt", order: "desc" } });

const columns = computed(() => [
  { key: "scenario", label: t("tests.scenario"), exportValue: (row) => row.scenario?.name },
  { key: "subject", label: t("tests.subject"), exportValue: (row) => row.subject?.code },
  { key: "operator", label: t("tests.operator"), cellClass: "muted", exportValue: (row) => row.operator?.name },
  { key: "status", label: t("tests.status"), sortable: true },
  { key: "result", label: t("tests.result"), exportValue: (row) => passedLabel(row) },
]);

function passedLabel(row) {
  if (row.passed === true) return t("tests.passYes");
  if (row.passed === false) return t("tests.passNo");
  return t("tests.passNa");
}
function passedClass(row) {
  if (row.passed === true) return "pass-yes";
  if (row.passed === false) return "pass-no";
  return "pass-na";
}
function criteriaCount(row) {
  if (!row.acceptanceCriteria) return 0;
  try { return JSON.parse(row.acceptanceCriteria).length; } catch { return 0; }
}
</script>

<template>
  <div class="head">
    <h1>{{ $t("tests.title") }}</h1>
    <button class="primary" @click="$router.push('/tests/new')">{{ $t("common.create") }}</button>
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
      export-name="tests"
      :entity-label="$t('tests.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-scenario="{ row }"><RouterLink class="link" :to="`/tests/${row.id}`">{{ row.scenario?.name }}</RouterLink></template>
      <template #cell-subject="{ row }">{{ row.subject?.code }}</template>
      <template #cell-operator="{ row }">{{ row.operator?.name }}</template>
      <template #cell-status="{ row }"><span :class="'status-' + row.status">{{ row.status }}</span></template>
      <template #cell-result="{ row }">
        <span class="pill" :class="passedClass(row)">{{ passedLabel(row) }}</span>
        <span class="muted" v-if="criteriaCount(row)"> · {{ criteriaCount(row) }} {{ $t("acceptance.criteriaShort") }}</span>
      </template>
    </DataTable>
  </div>
</template>

<style scoped>
.head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
.pill.pass-yes { background: #1f7a3d; color: #fff; }
.pill.pass-no { background: #a3271f; color: #fff; }
.pill.pass-na { background: var(--active-bg); color: var(--muted); }
</style>
