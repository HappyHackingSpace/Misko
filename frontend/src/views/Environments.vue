<script setup>
import { ref, computed, onMounted, watch } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";
import DataTable from "../components/DataTable.vue";
import LabTabs from "../components/LabTabs.vue";
import { useDataTable } from "../composables/useDataTable.js";

const { locale, t } = useI18n();

const paradigms = ref([]); // paradigm summaries for label lookup
const err = ref("");

const table = useDataTable("/environments", { defaultSort: { field: "createdAt", order: "desc" } });

const columns = computed(() => [
  { key: "name", label: t("common.name"), sortable: true },
  {
    key: "paradigmKey", label: t("environments.paradigm"), sortable: true,
    exportValue: (row) => paradigmName(row.paradigmKey),
  },
  { key: "notes", label: t("common.notes"), cellClass: "muted" },
]);

function paradigmName(key) {
  const p = paradigms.value.find((x) => x.key === key);
  return p ? p.name : key;
}

async function loadParadigms() {
  err.value = "";
  try {
    const res = await api(`/paradigms?all=true&lang=${locale.value}`);
    paradigms.value = res.data;
  } catch (e) {
    err.value = e.message;
  }
}

// Re-fetch localized paradigm labels when the language changes.
watch(locale, loadParadigms);
onMounted(loadParadigms);
</script>

<template>
  <div class="lab-head">
    <span class="lab-head-side"></span>
    <LabTabs />
    <span class="lab-head-side right">
      <button class="primary" @click="$router.push('/environments/new')">{{ $t("common.create") }}</button>
    </span>
  </div>
  <p class="muted" style="margin-top:0">{{ $t("environments.intro") }}</p>
  <p class="err" v-if="err">{{ err }}</p>

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
      export-name="environments"
      :entity-label="$t('environments.title')"
      :empty-hint="$t('datatable.emptyHint')"
      @page="table.setPage"
      @page-size="table.setPageSize"
      @sort="table.setSort"
      @search="table.setSearch"
    >
      <template #cell-name="{ row }"><RouterLink class="link" :to="`/environments/${row.id}`">{{ row.name }}</RouterLink></template>
      <template #cell-paradigmKey="{ row }"><span class="pill">{{ paradigmName(row.paradigmKey) }}</span></template>
    </DataTable>
  </div>
</template>

<style scoped>
.lab-head { display: flex; align-items: center; margin-bottom: 12px; gap: 12px; }
.lab-head-side { flex: 1; }
.lab-head-side.right { display: flex; justify-content: flex-end; }
.link { color: var(--accent); cursor: pointer; font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
