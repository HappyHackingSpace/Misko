<script setup>
// Study reports: pick an experiment and read its report, the same one the
// experiment's Results tab shows. The choice is kept in the address so a
// report can be linked.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { NAlert, NEmpty, NSelect } from "naive-ui";
import { experimentOptions } from "../api/analysis.js";
import { filterByText, selectOption } from "../composables/selectOptions.js";
import PageHead from "../components/PageHead.vue";
import ReportView from "../components/analysis/ReportView.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();

const experiments = ref([]);
const error = ref("");
const loading = ref(true);
const experimentId = computed(() => route.query.experimentId || null);
const chosen = computed(() => experiments.value.find((e) => e.id === experimentId.value) || null);

onMounted(async () => {
  try {
    experiments.value = (await experimentOptions()).data || [];
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
});

// Drafts have nothing to report; they are listed last and say so.
const options = computed(() =>
  [...experiments.value]
    .sort((a, b) => (a.status === "DRAFT") - (b.status === "DRAFT"))
    .map((e) => ({
      ...selectOption("report-experiment", e.id, `${e.code} · ${e.title}`, t(`experiments.status.${e.status}`)),
      disabled: e.status === "DRAFT",
    })),
);
</script>

<template>
  <PageHead :title="$t('studyReport.pageTitle')" :subtitle="$t('studyReport.pageIntro')" />
  <NAlert v-if="error" type="error" :title="error" style="margin-bottom: 12px" />
  <div class="pick-row">
  <NSelect
    class="pick"
    :value="experimentId"
    filterable
    :filter="filterByText"
    :loading="loading"
    :placeholder="$t('studyReport.pickExperiment')"
    :options="options"
    data-test="report-experiment"
    @update:value="router.replace({ query: { experimentId: $event } })"
  />
  <RouterLink v-if="chosen" class="link" :to="`/experiments/${chosen.id}?tab=results`" data-test="report-open-experiment">{{ $t("studyReport.openExperiment") }}</RouterLink>
  </div>
  <ReportView v-if="experimentId" :key="experimentId" :experiment-id="experimentId" />
  <NEmpty v-else-if="!loading" :description="$t('studyReport.pickHint')" style="margin-top: 40px" />
</template>

<style scoped>
.pick-row { display: flex; gap: 14px; align-items: center; flex-wrap: wrap; margin-bottom: 16px; }
.pick { width: 420px; max-width: 100%; }
</style>
