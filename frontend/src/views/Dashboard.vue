<script setup>
// The landing page: where the lab's work stands right now, without opening a
// single experiment. Everything here is read from one summary call — no
// per-row fetches — so the counts and the two short lists load together.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NCard, NTag } from "naive-ui";
import { dashboard } from "../api/endpoints.js";
import PageHead from "../components/PageHead.vue";

const { t, locale } = useI18n();

const summary = ref(null);
const error = ref("");
const loading = ref(true);

async function load() {
  error.value = "";
  try {
    summary.value = await dashboard.summary();
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
}
onMounted(load);

const testTiles = computed(() => {
  const t2 = summary.value?.tests || { planned: 0, inProgress: 0, completed: 0, cancelled: 0 };
  return [
    { key: "planned", value: t2.planned, label: t("statuses.PLANNED"), tone: "info" },
    { key: "inProgress", value: t2.inProgress, label: t("statuses.IN_PROGRESS"), tone: "warning" },
    { key: "completed", value: t2.completed, label: t("statuses.COMPLETED"), tone: "success" },
    { key: "cancelled", value: t2.cancelled, label: t("statuses.CANCELLED"), tone: "default" },
  ];
});

const pipelineTiles = computed(() => {
  const s = summary.value;
  return [
    { key: "queued", value: s?.analysisQueued ?? 0, label: t("dashboard.analysisQueued"), tone: "info" },
    { key: "running", value: s?.analysisRunning ?? 0, label: t("dashboard.analysisRunning"), tone: "warning" },
    { key: "failed", value: s?.analysisFailed ?? 0, label: t("dashboard.analysisFailed"), tone: "error", attention: true },
    { key: "calibration", value: s?.calibrationWaiting ?? 0, label: t("dashboard.calibrationWaiting"), tone: "warning", attention: true },
  ];
});

const dateTime = (iso) => new Date(iso).toLocaleString(locale.value);
</script>

<template>
  <PageHead :title="$t('dashboard.title')" :subtitle="$t('dashboard.intro')" />
  <NAlert v-if="error" type="error" :title="error" style="margin-bottom: 16px" />

  <template v-if="summary">
    <div class="stats" data-test="test-status-tiles">
      <NCard v-for="tile in testTiles" :key="tile.key" size="small" :bordered="true" class="tile" :data-test="`tile-${tile.key}`">
        <div class="tile-value">{{ tile.value }}</div>
        <NTag size="small" round :bordered="false" :type="tile.tone">{{ tile.label }}</NTag>
      </NCard>
    </div>

    <div class="stats">
      <NCard
        v-for="tile in pipelineTiles"
        :key="tile.key"
        size="small"
        :bordered="true"
        class="tile"
        :class="{ attention: tile.attention && tile.value > 0 }"
        :data-test="`tile-${tile.key}`"
      >
        <div class="tile-value">{{ tile.value }}</div>
        <NTag size="small" round :bordered="false" :type="tile.tone">{{ tile.label }}</NTag>
      </NCard>
    </div>

    <div class="grid-2">
      <NCard :bordered="true" size="small">
        <template #header>{{ $t("dashboard.recentActivity") }}</template>
        <ul class="rows-list" v-if="summary.recentActivity.length" data-test="recent-activity">
          <li v-for="run in summary.recentActivity" :key="run.testId + run.finishedAt">
            <RouterLink class="link" :to="`/tests/${run.testId}`">{{ run.subjectCode }}</RouterLink>
            <span class="muted">{{ run.experimentCode }}</span>
            <NTag size="small" round :bordered="false">{{ run.paradigmKey }}</NTag>
            <span class="muted time">{{ dateTime(run.finishedAt) }}</span>
          </li>
        </ul>
        <p v-else class="muted empty">{{ $t("dashboard.noActivity") }}</p>
      </NCard>

      <NCard :bordered="true" size="small">
        <template #header>{{ $t("dashboard.upcomingTests") }}</template>
        <ul class="rows-list" v-if="summary.upcomingTests.length" data-test="upcoming-tests">
          <li v-for="test in summary.upcomingTests" :key="test.id">
            <RouterLink class="link" :to="`/tests/${test.id}`">{{ test.subjectCode }}</RouterLink>
            <span class="muted">{{ test.experimentCode }}</span>
            <NTag size="small" round :bordered="false">{{ test.paradigmKey }}</NTag>
            <span class="muted time">{{ dateTime(test.scheduledAt) }}</span>
          </li>
        </ul>
        <p v-else class="muted empty">{{ $t("dashboard.noUpcoming") }}</p>
      </NCard>
    </div>
  </template>
</template>

<style scoped>
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 12px; margin-bottom: 16px; }
.tile { text-align: left; }
.tile-value { font-size: 26px; font-weight: 700; letter-spacing: -0.02em; margin-bottom: 6px; }
.tile.attention { border-color: var(--bad); }

.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; align-items: start; }
@media (max-width: 860px) { .grid-2 { grid-template-columns: 1fr; } }

.rows-list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.rows-list li { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 8px 10px; border-radius: 8px; }
.rows-list li:hover { background: var(--panel2); }
.time { margin-left: auto; font-size: 12.5px; }
.empty { margin: 0; }
.link { color: var(--accent); font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
