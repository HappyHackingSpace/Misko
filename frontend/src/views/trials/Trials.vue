<script setup>
// Paradigm trials: a paradigm version tried on one video in one arena,
// outside any experiment. Everyone reads them; starting one needs trial:run.
import { computed, h, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRouter } from "vue-router";
import { NAlert, NButton, NCard, NDataTable, NSelect } from "naive-ui";
import { lookups } from "../../api/tests.js";
import { useDataTable } from "../../composables/useDataTable.js";
import { useParadigmNames } from "../../composables/useParadigmNames.js";
import { useUserNames } from "../../composables/useUserNames.js";
import { selectOption } from "../../composables/selectOptions.js";
import { useNumbers } from "../../components/analysis/format.js";
import { useAuth } from "../../stores/auth.js";
import PageHead from "../../components/PageHead.vue";
import ListToolbar from "../../components/ListToolbar.vue";
import JobStatusTag from "../../components/trials/JobStatusTag.vue";
import TrialDrawer from "../../components/trials/TrialDrawer.vue";

const { t } = useI18n();
const router = useRouter();
const auth = useAuth();
const fmt = useNumbers();
const { paradigmName } = useParadigmNames();
const { userName } = useUserNames();
const canRun = computed(() => auth.can("trial:run"));

const drawer = ref(false);
const paradigmKey = ref(null);
const table = useDataTable("/trials", { pageSize: 20, defaultSort: { field: "", order: "" }, extraParams: () => ({ paradigmKey: paradigmKey.value }) });
const setParadigm = (value) => {
  paradigmKey.value = value;
  table.setPage(1);
};
const filtered = computed(() => !!(table.state.search || paradigmKey.value));

const paradigms = ref([]);
lookups.paradigms().then((r) => (paradigms.value = r.data || [])).catch(() => {});
const paradigmOptions = computed(() => paradigms.value.map((p) => selectOption("trial-filter-paradigm", p.key, p.name)));

const columns = computed(() => [
  {
    title: t("trials.name"),
    key: "name",
    render: (row) => h(RouterLink, { class: "link", to: `/trials/${row.id}`, "data-test": "trial-open" }, () => row.name),
  },
  { title: t("trials.paradigm"), key: "paradigmKey", render: (row) => `${paradigmName(row.paradigmKey)} · ${t("trials.version", { n: row.paradigmVersion })}` },
  { title: t("trials.arena"), key: "arena", render: (row) => `${row.arena.environmentName} · ${t("trials.revision", { n: row.arena.revision })}` },
  { title: t("trials.createdBy"), key: "createdBy", render: (row) => userName(row.createdBy) },
  { title: t("trials.createdAt"), key: "createdAt", render: (row) => fmt.dateTime(row.createdAt) },
  {
    title: t("trials.lastResult"),
    key: "latestJob",
    render: (row) => (row.latestJob ? h(JobStatusTag, { status: row.latestJob.status }) : h("span", { class: "muted" }, row.videoId ? t("trials.notAnalysed") : t("trials.noVideo"))),
  },
]);
</script>

<template>
  <PageHead :title="$t('trials.title')" :subtitle="$t('trials.subtitle')">
    <template #actions>
      <NButton v-if="canRun" type="primary" data-test="trial-new" @click="drawer = true">{{ $t("trials.new") }}</NButton>
    </template>
  </PageHead>

  <NCard :bordered="true" size="small">
    <template #header>
      <ListToolbar :search="table.state.search" :exportable="false" @update:search="table.setSearch" />
      <div class="filters">
        <NSelect
          :value="paradigmKey"
          class="filter"
          clearable
          data-test="trial-filter-paradigm"
          :options="paradigmOptions"
          :placeholder="$t('trials.allParadigms')"
          @update:value="setParadigm"
        />
      </div>
    </template>
    <NAlert v-if="table.state.error" type="error" :title="table.state.error" class="alert" data-test="trial-list-error" />
    <NDataTable
      remote
      :columns="columns"
      :data="table.state.rows"
      :loading="table.state.loading"
      :row-key="(row) => row.id"
      :row-props="(row) => ({ 'data-test': 'trial-row', 'data-name': row.name, style: 'cursor: pointer', onClick: (e) => !e.target.closest('a') && router.push(`/trials/${row.id}`) })"
      :scroll-x="860"
      :pagination="{
        page: table.state.page,
        pageSize: table.state.pageSize,
        itemCount: table.state.total,
        pageSizes: [10, 20, 50],
        showSizePicker: true,
        onUpdatePage: table.setPage,
        onUpdatePageSize: table.setPageSize,
      }"
    >
      <template #empty>
        <span class="muted">{{ filtered ? $t("trials.noMatch") : $t("trials.empty") }}</span>
      </template>
    </NDataTable>
  </NCard>

  <TrialDrawer v-model:show="drawer" @saved="(trial) => router.push(`/trials/${trial.id}`)" />
</template>

<style scoped>
.filters { display: flex; flex-wrap: wrap; align-items: center; gap: 10px; margin-top: 10px; }
.filter { width: 240px; }
.alert { margin-bottom: 12px; }
@media (max-width: 520px) {
  .filter { width: 100%; }
}
</style>
