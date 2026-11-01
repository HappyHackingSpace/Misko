<script setup>
// One paradigm trial: the paradigm and the arena it is tried in, its video,
// the calibration it is analysed with, and every analysis of it, newest first.
// While an analysis is queued or running the page looks again every few
// seconds until it finishes.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { NAlert, NButton, NCard, NPopconfirm, NSpin, useMessage } from "naive-ui";
import { trials } from "../../api/trials.js";
import { useAuth } from "../../stores/auth.js";
import { useBreadcrumb } from "../../stores/breadcrumb.js";
import { usePolling } from "../../composables/usePolling.js";
import { useUserNames } from "../../composables/useUserNames.js";
import { useNumbers, useParadigmText } from "../../components/analysis/format.js";
import PageHead from "../../components/PageHead.vue";
import TrialVideoCard from "../../components/trials/TrialVideoCard.vue";
import TrialCalibrationCard from "../../components/trials/TrialCalibrationCard.vue";
import TrialAnalysisCard from "../../components/trials/TrialAnalysisCard.vue";
import AnalyseAgainDrawer from "../../components/trials/AnalyseAgainDrawer.vue";
import AnalysisCompare from "../../components/trials/AnalysisCompare.vue";

const { t } = useI18n();
const route = useRoute();
const router = useRouter();
const auth = useAuth();
const crumb = useBreadcrumb();
const message = useMessage();
const fmt = useNumbers();
const text = useParadigmText();
const { userName } = useUserNames();

const trial = ref(null);
const error = ref("");
const deleting = ref(false);
const again = ref(false);
const comparing = ref(false);

const canRun = computed(() => auth.can("trial:run"));
const manifest = computed(() => (trial.value ? text.manifestOf(trial.value.paradigmKey, trial.value.paradigmVersion) : null));
const analysisParameters = computed(() => (manifest.value?.parameters || []).filter((p) => p.scope === "ANALYSIS"));
const metricLabel = (key) => text.metricLabel(trial.value?.paradigmKey, key);
const eventLabels = computed(() => (trial.value ? text.eventLabels(trial.value.paradigmKey) : {}));
const analyses = computed(() => trial.value?.analyses || []);
const active = computed(() => analyses.value.some((a) => a.job.status === "QUEUED" || a.job.status === "RUNNING"));
// A verified, calibrated video gets its automatic analysis from the scheduler shortly.
const awaitingFirst = computed(() => trial.value?.video?.status === "VERIFIED" && !!trial.value.calibrationId && !analyses.value.length);
const succeeded = computed(() => analyses.value.filter((a) => a.job.status === "SUCCEEDED").length);
const canAnalyse = computed(() => canRun.value && trial.value?.video?.status === "VERIFIED" && !!trial.value.calibrationId);

const polling = usePolling(() => load(true), () => active.value || awaitingFirst.value);

async function load(quiet = false) {
  try {
    show(await trials.get(route.params.id));
    error.value = "";
  } catch (e) {
    if (!quiet || !trial.value) error.value = e.message;
  }
  polling.schedule();
}

function show(detail) {
  trial.value = detail;
  text.need(detail.paradigmKey, detail.paradigmVersion);
  crumb.set([{ label: t("trials.title"), to: "/trials" }, { label: detail.name }]);
}

async function remove() {
  deleting.value = true;
  try {
    await trials.remove(trial.value.id);
    message.success(t("trials.deleted", { name: trial.value.name }));
    router.push("/trials");
  } catch (e) {
    error.value = e.message;
  } finally {
    deleting.value = false;
  }
}

onMounted(() => load());
</script>

<template>
  <NSpin :show="!trial && !error">
    <NAlert v-if="error" type="error" :title="error" data-test="trial-error" class="alert" />
    <template v-if="trial">
      <PageHead :title="trial.name" :subtitle="$t('trials.detailSubtitle')">
        <template #actions>
          <NPopconfirm v-if="canRun" :positive-text="$t('common.delete')" :negative-text="$t('common.cancel')" @positive-click="remove">
            <template #trigger>
              <NButton secondary type="error" :loading="deleting" data-test="trial-delete">{{ $t("common.delete") }}</NButton>
            </template>
            {{ $t("trials.confirmDelete", { name: trial.name }) }}
          </NPopconfirm>
        </template>
      </PageHead>

      <dl class="facts" data-test="trial-facts">
        <div>
          <dt>{{ $t("trials.paradigm") }}</dt>
          <dd><RouterLink class="link" :to="`/paradigms/${trial.paradigmKey}`">{{ text.paradigmName(trial.paradigmKey) }}</RouterLink> · {{ $t("trials.version", { n: trial.paradigmVersion }) }}</dd>
        </div>
        <div>
          <dt>{{ $t("trials.arena") }}</dt>
          <dd data-test="trial-arena">
            <RouterLink class="link" :to="`/environments/${trial.arena.environmentId}`">{{ trial.arena.environmentName }}</RouterLink> · {{ $t("trials.revision", { n: trial.arena.revision }) }}
          </dd>
        </div>
        <div>
          <dt>{{ $t("trials.createdAt") }}</dt>
          <dd>{{ fmt.dateTime(trial.createdAt) }} · {{ userName(trial.createdBy) }}</dd>
        </div>
        <div v-if="trial.notes" class="wide">
          <dt>{{ $t("trials.notes") }}</dt>
          <dd class="notes">{{ trial.notes }}</dd>
        </div>
      </dl>

      <div class="grid">
        <TrialVideoCard :trial="trial" :can-run="canRun" @changed="(detail) => (show(detail), polling.schedule())" />
        <TrialCalibrationCard :trial="trial" :paradigm="manifest" :can-run="canRun" @saved="load(true)" />
      </div>

      <NCard size="small" :title="$t('trials.analyses.title')" class="analyses" data-test="trial-analyses">
        <template #header-extra>
          <div class="actions">
            <NButton v-if="succeeded >= 2" size="small" secondary data-test="trial-compare-open" @click="comparing = true">{{ $t("trials.compare.open") }}</NButton>
            <NButton v-if="canAnalyse" size="small" type="primary" secondary data-test="trial-analyse-again" @click="again = true">
              {{ $t("trials.again.open") }}
            </NButton>
          </div>
        </template>
        <p v-if="!analyses.length" class="muted" data-test="trial-analyses-none">
          {{ awaitingFirst ? $t("trials.analyses.starting") : $t("trials.analyses.none") }}
        </p>
        <TrialAnalysisCard
          v-for="analysis in analyses"
          :key="analysis.jobId"
          :analysis="analysis"
          :parameters="manifest?.parameters || []"
          :metric-label="metricLabel"
          :event-labels="eventLabels"
        />
      </NCard>

      <AnalyseAgainDrawer v-model:show="again" :trial="trial" :parameters="analysisParameters" @submitted="load(true)" />
      <AnalysisCompare v-model:show="comparing" :analyses="analyses" :metric-label="metricLabel" />
    </template>
  </NSpin>
</template>

<style scoped>
.alert { margin-bottom: 12px; }
.facts { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 8px 16px; margin: 0 0 16px; }
.facts div { min-width: 0; }
.facts .wide { grid-column: 1 / -1; }
.facts dt { font-size: 11.5px; color: var(--muted); }
.facts dd { margin: 0; font-size: 13.5px; overflow-wrap: anywhere; }
.notes { white-space: pre-wrap; }
.grid { display: grid; grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); gap: 14px; align-items: start; }
.analyses { margin-top: 14px; }
.actions { display: flex; gap: 8px; flex-wrap: wrap; justify-content: flex-end; }
@media (max-width: 900px) { .grid { grid-template-columns: minmax(0, 1fr); } }
</style>
