<script setup>
// The published paradigm catalog. The API returns the whole list at once, and
// says which versions a worker can actually analyze.
import { computed, h, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NCard, NDataTable, NTag } from "naive-ui";
import { api } from "../api/client.js";
import { useTableExport } from "../composables/useTableExport.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t } = useI18n();
const paradigms = ref([]);
const error = ref("");
const search = ref("");
const loading = ref(true);

onMounted(async () => {
  try {
    paradigms.value = (await api("/paradigms")).data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
});

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return paradigms.value;
  return paradigms.value.filter((p) => p.key.toLowerCase().includes(q) || p.name.toLowerCase().includes(q));
});

const columns = computed(() => [
  {
    title: t("paradigms.key"),
    key: "key",
    sorter: "default",
    render: (row) => h(RouterLink, { class: "link", to: `/paradigms/${row.key}` }, () => row.key),
  },
  { title: t("common.name"), key: "name", sorter: "default" },
  { title: t("paradigms.versions"), key: "versions", render: (row) => row.versions.join(", ") },
  {
    title: t("paradigms.automated"),
    key: "automatedAnalysis",
    render: (row) =>
      h(NTag, { size: "small", round: true, bordered: false, type: row.automatedAnalysis ? "success" : "default" }, () =>
        row.automatedAnalysis ? t("common.yes") : t("common.no"),
      ),
  },
]);

const exportColumns = [
  { key: "key", label: t("paradigms.key") },
  { key: "name", label: t("common.name") },
  { key: "versions", label: t("paradigms.versions"), value: (row) => row.versions.join(", ") },
  { key: "automatedAnalysis", label: t("paradigms.automated"), value: (row) => (row.automatedAnalysis ? t("common.yes") : t("common.no")) },
];
const { exportCsv, exportPdf } = useTableExport({
  state: {
    get rows() {
      return filtered.value;
    },
  },
  columns: exportColumns,
  exportName: "paradigms",
  entityLabel: t("paradigms.title"),
});
</script>

<template>
  <PageHead :title="$t('paradigms.title')" :count="paradigms.length" />
  <p class="muted intro">{{ $t("paradigms.intro") }}</p>
  <NAlert v-if="error" type="error" :title="error" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small">
    <template #header>
      <ListToolbar :search="search" :export-disabled="!filtered.length" @update:search="search = $event" @csv="exportCsv" @pdf="exportPdf" />
    </template>
    <NDataTable :columns="columns" :data="filtered" :loading="loading" :row-key="(row) => row.key" />
  </NCard>
</template>

<style scoped>
.intro { margin: -8px 0 18px; }
</style>
