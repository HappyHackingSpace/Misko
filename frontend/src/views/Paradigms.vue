<script setup>
// The published paradigm catalog: the templates environments are created
// from. The API returns the whole list at once in the chosen language.
import { computed, h, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NCard, NDataTable } from "naive-ui";
import { api } from "../api/client.js";
import { useTableExport } from "../composables/useTableExport.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t, locale } = useI18n();
const paradigms = ref([]);
const error = ref("");
const search = ref("");
const loading = ref(true);

async function load() {
  try {
    paradigms.value = (await api("/paradigms")).data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}
onMounted(load);
watch(locale, load);

const category = (key) => t(`paradigms.categories.${String(key).toLowerCase()}`);

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return paradigms.value;
  return paradigms.value.filter((p) =>
    [p.key, p.name, p.description, category(p.category)].some((text) => text.toLowerCase().includes(q)),
  );
});

const columns = computed(() => [
  {
    title: t("common.name"),
    key: "name",
    sorter: "default",
    render: (row) =>
      h("div", { class: "cell" }, [
        h(RouterLink, { class: "link", to: `/paradigms/${row.key}`, "data-test": `paradigm-${row.key}` }, () => row.name),
        h("small", { class: "muted" }, row.description),
      ]),
  },
  { title: t("paradigms.category"), key: "category", sorter: "default", width: 170, render: (row) => category(row.category) },
  { title: t("paradigms.versions"), key: "versions", render: (row) => row.versions.join(", ") },
]);

const exportColumns = [
  { key: "key", label: t("paradigms.key") },
  { key: "name", label: t("common.name") },
  { key: "category", label: t("paradigms.category"), value: (row) => category(row.category) },
  { key: "versions", label: t("paradigms.versions"), value: (row) => row.versions.join(", ") },
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
:deep(.cell) { display: flex; flex-direction: column; gap: 2px; max-width: 640px; }
</style>
