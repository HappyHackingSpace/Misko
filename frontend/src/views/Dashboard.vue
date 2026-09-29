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

const groups = computed(() => [
  { key: "tests", title: t("dashboard.groupTests"), tiles: testTiles.value, link: "/tests", dataTest: "test-status-tiles" },
  { key: "analysis", title: t("dashboard.groupAnalysis"), tiles: pipelineTiles.value, dataTest: "analysis-tiles" },
]);

const dateTime = (iso) => new Date(iso).toLocaleString(locale.value);

// "5 min ago" / "in 2 days"; the exact timestamp stays available as a tooltip.
const rtf = computed(() => new Intl.RelativeTimeFormat(locale.value, { numeric: "auto" }));
const UNITS = [["day", 86400], ["hour", 3600], ["minute", 60]];
function relative(iso) {
  const diff = (new Date(iso).getTime() - Date.now()) / 1000;
  for (const [unit, secs] of UNITS) {
    if (Math.abs(diff) >= secs) return rtf.value.format(Math.round(diff / secs), unit);
  }
  return rtf.value.format(Math.round(diff), "second");
}

// Feather-style 24px stroke icons, inlined to avoid a new dependency.
const ICONS = {
  planned: "M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z",
  inProgress: "M22 12h-4l-3 9L9 3l-3 9H2",
  completed: "M22 11.08V12a10 10 0 1 1-5.93-9.14M22 4L12 14.01l-3-3",
  cancelled: "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM15 9l-6 6M9 9l6 6",
  queued: "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 6v6l4 2",
  running: "M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15",
  failed: "M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0zM12 9v4M12 17h.01",
  calibration: "M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z",
};
</script>

<template>
  <PageHead :title="$t('dashboard.title')" :subtitle="$t('dashboard.intro')">
    <template #actions>
      <RouterLink to="/tests" class="head-link">{{ $t("dashboard.viewTests") }} &rarr;</RouterLink>
    </template>
  </PageHead>
  <NAlert v-if="error" type="error" :title="error" style="margin-bottom: 16px" />

  <template v-if="summary">
    <section v-for="group in groups" :key="group.key" class="group">
      <h2 class="group-title">{{ group.title }}</h2>
      <div class="stats" :data-test="group.dataTest">
        <component
          :is="group.link ? RouterLink : 'div'"
          v-for="tile in group.tiles"
          :key="tile.key"
          :to="group.link"
          class="tile"
          :class="[`tone-${tile.tone}`, { attention: tile.attention && tile.value > 0, quiet: tile.attention && tile.value === 0, clickable: group.link }]"
          :data-test="`tile-${tile.key}`"
        >
          <span class="tile-icon">
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path :d="ICONS[tile.key]" />
            </svg>
          </span>
          <span class="tile-value">{{ tile.value }}</span>
          <span class="tile-label">{{ tile.label }}</span>
        </component>
      </div>
    </section>

    <div class="grid-2">
      <NCard :bordered="true" size="small" class="list-card">
        <template #header>{{ $t("dashboard.recentActivity") }}</template>
        <ul v-if="summary.recentActivity.length" class="timeline" data-test="recent-activity">
          <li v-for="run in summary.recentActivity" :key="run.testId + run.finishedAt">
            <span class="dot dot-success"></span>
            <RouterLink class="link" :to="`/tests/${run.testId}`">{{ run.subjectCode }}</RouterLink>
            <span class="muted">{{ run.experimentCode }}</span>
            <NTag size="small" round :bordered="false">{{ run.paradigmKey }}</NTag>
            <span class="muted time" :title="dateTime(run.finishedAt)">{{ relative(run.finishedAt) }}</span>
          </li>
        </ul>
        <div v-else class="empty-state">
          <span class="empty-icon">
            <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path :d="ICONS.inProgress" /></svg>
          </span>
          <p class="muted">{{ $t("dashboard.noActivity") }}</p>
        </div>
      </NCard>

      <NCard :bordered="true" size="small" class="list-card">
        <template #header>{{ $t("dashboard.upcomingTests") }}</template>
        <ul v-if="summary.upcomingTests.length" class="timeline" data-test="upcoming-tests">
          <li v-for="test in summary.upcomingTests" :key="test.id">
            <span class="dot dot-info"></span>
            <RouterLink class="link" :to="`/tests/${test.id}`">{{ test.subjectCode }}</RouterLink>
            <span class="muted">{{ test.experimentCode }}</span>
            <NTag size="small" round :bordered="false">{{ test.paradigmKey }}</NTag>
            <span class="muted time" :title="dateTime(test.scheduledAt)">{{ relative(test.scheduledAt) }}</span>
          </li>
        </ul>
        <div v-else class="empty-state">
          <span class="empty-icon">
            <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path :d="ICONS.planned" /></svg>
          </span>
          <p class="muted">{{ $t("dashboard.noUpcoming") }}</p>
          <RouterLink to="/tests" class="empty-action">{{ $t("dashboard.planTest") }}</RouterLink>
        </div>
      </NCard>
    </div>
  </template>
</template>

<style scoped>
.head-link { font-size: 13.5px; font-weight: 600; color: var(--accent); text-decoration: none; white-space: nowrap; }
.head-link:hover { text-decoration: underline; }

.group { margin-bottom: 22px; }
.group-title { margin: 0 0 10px; font-size: 12px; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
.stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(170px, 1fr)); gap: 12px; }

.tile {
  --tone: var(--muted);
  display: grid; grid-template-columns: auto 1fr; align-items: center; column-gap: 12px; row-gap: 2px;
  padding: 14px 16px; border-radius: 14px; background: var(--panel); border: 1px solid var(--line);
  box-shadow: var(--shadow-sm); color: var(--txt); text-decoration: none;
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
}
.tile.tone-info { --tone: #3b82f6; }
.tile.tone-warning { --tone: var(--warn); }
.tile.tone-success { --tone: var(--good); }
.tile.tone-error { --tone: var(--bad); }
.tile-icon {
  grid-row: span 2; display: inline-flex; align-items: center; justify-content: center; width: 38px; height: 38px;
  border-radius: 10px; color: var(--tone); background: color-mix(in srgb, var(--tone) 14%, transparent);
}
.tile-value { font-size: 28px; font-weight: 700; letter-spacing: -0.02em; line-height: 1.1; }
.tile-label { font-size: 12.5px; color: var(--muted); }
.tile.clickable:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); border-color: color-mix(in srgb, var(--tone) 45%, var(--line)); }
.tile.attention { border-color: color-mix(in srgb, var(--bad) 55%, var(--line)); background: color-mix(in srgb, var(--bad) 5%, var(--panel)); }
.tile.quiet { opacity: 0.6; }
.tile.quiet .tile-icon { color: var(--muted); background: color-mix(in srgb, var(--muted) 12%, transparent); }

.grid-2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; align-items: stretch; }
.list-card { height: 100%; }
@media (max-width: 860px) { .grid-2 { grid-template-columns: 1fr; } }

.timeline { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
.timeline li { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 8px 10px; border-radius: 8px; }
.timeline li:hover { background: var(--panel2); }
.dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
.dot-success { background: var(--good); box-shadow: 0 0 0 3px color-mix(in srgb, var(--good) 20%, transparent); }
.dot-info { background: #3b82f6; box-shadow: 0 0 0 3px color-mix(in srgb, #3b82f6 20%, transparent); }
.time { margin-left: auto; font-size: 12.5px; white-space: nowrap; }

.empty-state { display: flex; flex-direction: column; align-items: center; gap: 8px; padding: 22px 8px; text-align: center; }
.empty-state p { margin: 0; }
.empty-icon { display: inline-flex; align-items: center; justify-content: center; width: 44px; height: 44px; border-radius: 12px; color: var(--muted); background: var(--active-bg); }
.empty-action { font-size: 13px; font-weight: 600; color: var(--accent); text-decoration: none; border: 1px solid var(--line); border-radius: 8px; padding: 6px 12px; }
.empty-action:hover { background: var(--active-bg); }
.link { color: var(--accent); font-weight: 600; text-decoration: none; }
.link:hover { text-decoration: underline; }
</style>
