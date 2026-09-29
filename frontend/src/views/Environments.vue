<script setup>
// Apparatus and their measurement revisions. Anyone signed in may read them;
// defining one needs apparatus:write, so the control appears only for a role
// the API would accept.
import { computed, h, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NDataTable, NTag } from "naive-ui";
import { api } from "../api/client.js";
import { useAuth } from "../stores/auth.js";
import { useTableExport } from "../composables/useTableExport.js";
import PageHead from "../components/PageHead.vue";
import ListToolbar from "../components/ListToolbar.vue";

const { t } = useI18n();
const auth = useAuth();
const canWrite = computed(() => auth.can("apparatus:write"));
const environments = ref([]);
const error = ref("");
const search = ref("");
const loading = ref(true);

onMounted(async () => {
  try {
    environments.value = (await api("/environments")).data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
});

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase();
  if (!q) return environments.value;
  return environments.value.filter((e) => e.name.toLowerCase().includes(q) || e.paradigmKey.toLowerCase().includes(q));
});

const columns = computed(() => [
  { title: t("common.name"), key: "name", sorter: "default" },
  {
    title: t("environments.paradigm"),
    key: "paradigmKey",
    sorter: "default",
    render: (row) => h(NTag, { size: "small", round: true, bordered: false }, () => row.paradigmKey),
  },
  { title: t("environments.latestRevision"), key: "latestRevision" },
  { title: t("common.notes"), key: "notes", ellipsis: { tooltip: true }, render: (row) => row.notes || "—" },
]);

const exportColumns = [
  { key: "name", label: t("common.name") },
  { key: "paradigmKey", label: t("environments.paradigm") },
  { key: "latestRevision", label: t("environments.latestRevision") },
  { key: "notes", label: t("common.notes") },
];
const { exportCsv, exportPdf } = useTableExport({
  state: {
    get rows() {
      return filtered.value;
    },
  },
  columns: exportColumns,
  exportName: "environments",
  entityLabel: t("environments.title"),
});
</script>

<template>
  <PageHead :title="$t('environments.title')" :count="environments.length">
    <template #actions>
      <NButton v-if="canWrite" type="primary" data-test="environment-new" @click="$router.push('/environments/new')">
        <template #icon>
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
        </template>
        {{ $t("environments.new") }}
      </NButton>
    </template>
  </PageHead>
  <p class="muted intro">{{ $t("environments.intro") }}</p>
  <NAlert v-if="error" type="error" :title="error" style="margin-bottom: 16px" />

  <NCard :bordered="true" size="small">
    <template #header>
      <ListToolbar :search="search" :export-disabled="!filtered.length" @update:search="search = $event" @csv="exportCsv" @pdf="exportPdf" />
    </template>
    <NDataTable
      :columns="columns"
      :data="filtered"
      :loading="loading"
      :row-key="(row) => row.id"
      :row-props="() => ({ 'data-test': 'environment' })"
    />
  </NCard>
</template>

<style scoped>
.intro { margin: -8px 0 18px; }
</style>
