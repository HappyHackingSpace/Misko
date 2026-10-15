<script setup>
// Arenas (environments) and their measurement revisions. Anyone signed in may
// read them; defining one needs apparatus:write, so the control appears only
// for a role the API would accept.
import { computed, h, onMounted, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NButton, NCard, NDataTable, NSelect, NTag } from "naive-ui";
import { environments, paradigmCatalog } from "../api/catalog.js";
import { useAuth } from "../stores/auth.js";
import { useTableExport } from "../composables/useTableExport.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t, locale } = useI18n();
const auth = useAuth();
const canWrite = computed(() => auth.can("apparatus:write"));
const rows = ref([]);
const paradigms = ref([]);
const paradigmFilter = ref(null);
const error = ref("");
const search = ref("");
const loading = ref(true);

// Paradigm names come from the API in the panel's language.
const paradigmName = (key) => paradigms.value.find((p) => p.key === key)?.name || key;
const paradigmOptions = computed(() => paradigms.value.map((p) => ({ value: p.key, label: p.name })));

async function loadParadigms() {
  paradigms.value = (await paradigmCatalog.list()).data || [];
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    rows.value = (await environments.list({ paradigmKey: paradigmFilter.value })).data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}

onMounted(async () => {
  try {
    await Promise.all([loadParadigms(), load()]);
  } catch (e) {
    error.value = e.message;
  }
});
watch(paradigmFilter, load);
watch(locale, () => loadParadigms().catch((e) => (error.value = e.message)));

const filtered = computed(() => {
  const q = search.value.trim().toLocaleLowerCase(locale.value);
  if (!q) return rows.value;
  return rows.value.filter((e) => e.name.toLocaleLowerCase(locale.value).includes(q) || paradigmName(e.paradigmKey).toLocaleLowerCase(locale.value).includes(q));
});

const columns = computed(() => [
  {
    title: t("common.name"),
    key: "name",
    sorter: (a, b) => a.name.localeCompare(b.name, locale.value),
    render: (row) => h(RouterLink, { class: "link", to: `/environments/${row.id}`, "data-test": "environment-open" }, () => row.name),
  },
  {
    title: t("environments.paradigm"),
    key: "paradigmKey",
    sorter: (a, b) => paradigmName(a.paradigmKey).localeCompare(paradigmName(b.paradigmKey), locale.value),
    render: (row) => h(NTag, { size: "small", round: true, bordered: false, "data-test": "environment-paradigm", "data-key": row.paradigmKey }, () => paradigmName(row.paradigmKey)),
  },
  { title: t("environments.latestRevision"), key: "latestRevision", width: 130 },
  { title: t("common.notes"), key: "notes", ellipsis: { tooltip: true }, render: (row) => row.notes || "" },
]);

const { exportCsv, exportPdf } = useTableExport({
  state: {
    get rows() {
      return filtered.value;
    },
  },
  columns: [
    { key: "name", label: t("common.name") },
    { key: "paradigmKey", label: t("environments.paradigm"), value: (row) => paradigmName(row.paradigmKey) },
    { key: "latestRevision", label: t("environments.latestRevision") },
    { key: "notes", label: t("common.notes") },
  ],
  exportName: "environments",
  entityLabel: t("environments.title"),
});
</script>

<template>
  <PageHead :title="$t('environments.title')" :count="filtered.length">
    <template #actions>
      <NButton v-if="canWrite" type="primary" data-test="environment-new" @click="$router.push('/environments/new')">
        {{ $t("environments.new") }}
      </NButton>
    </template>
  </PageHead>
  <p class="muted intro">{{ $t("environments.intro") }}</p>
  <NAlert v-if="error" type="error" :title="error" class="alert" data-test="error" />

  <NCard :bordered="true" size="small">
    <template #header>
      <ListToolbar :search="search" :export-disabled="!filtered.length" @update:search="search = $event" @csv="exportCsv" @pdf="exportPdf" />
      <div class="filters">
        <NSelect v-model:value="paradigmFilter" class="filter" clearable filterable data-test="environment-paradigm-filter" :options="paradigmOptions" :placeholder="$t('environments.allParadigms')" />
      </div>
    </template>
    <NDataTable
      :columns="columns"
      :data="filtered"
      :loading="loading"
      :row-key="(row) => row.id"
      :row-props="() => ({ 'data-test': 'environment' })"
      :pagination="{ pageSize: 20 }"
      :scroll-x="640"
    >
      <template #empty>
        <span class="muted">{{ search || paradigmFilter ? $t("environments.noMatch") : $t("environments.empty") }}</span>
      </template>
    </NDataTable>
  </NCard>
</template>

<style scoped>
.intro { margin: -8px 0 18px; }
.alert { margin-bottom: 16px; }
.filters { margin-top: 10px; }
.filter { width: 260px; max-width: 100%; }
</style>
