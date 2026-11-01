<script setup>
// The landing page: where the lab's work stands right now. Everything is read
// from one summary call. Tiles link to the list that shows what they count
// (the review queue, the tests worklist, the experiments), filtered.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { RouterLink } from "vue-router";
import { NAlert, NCard, NSkeleton, NTag } from "naive-ui";
import { dashboard } from "../api/analysis.js";
import PageHead from "../components/PageHead.vue";
import { useNumbers, useParadigmText } from "../components/analysis/format.js";

const { t, locale } = useI18n();
const fmt = useNumbers();
const text = useParadigmText();

const summary = ref(null);
const error = ref("");

async function load() {
  error.value = "";
  try {
    summary.value = await dashboard.summary();
  } catch (e) {
    error.value = e.message;
  }
}
onMounted(load);

const groups = computed(() => {
  const s = summary.value;
  if (!s) return [];
  return [
    {
      key: "results",
      title: t("dashboard.groupResults"),
      tiles: [
        { key: "pending", value: s.results.pending, label: t("review.status.PENDING"), tone: "warning", to: "/review?status=PENDING", attention: true },
        { key: "accepted", value: s.results.accepted, label: t("review.status.ACCEPTED"), tone: "success", to: "/review?status=ACCEPTED" },
        { key: "rejected", value: s.results.rejected, label: t("review.status.REJECTED"), tone: "error", to: "/review?status=REJECTED" },
      ],
    },
    {
      key: "analysis",
      title: t("dashboard.groupAnalysis"),
      tiles: [
        { key: "queued", value: s.analysisQueued, label: t("dashboard.analysisQueued"), tone: "info" },
        { key: "running", value: s.analysisRunning, label: t("dashboard.analysisRunning"), tone: "info" },
        { key: "failed", value: s.analysisFailed, label: t("dashboard.analysisFailed"), tone: "error", attention: true },
        {
          key: "calibration",
          value: s.calibrationWaiting,
          label: t("dashboard.calibrationWaiting"),
          tone: "warning",
          attention: true,
          to: "/tests?status=IN_PROGRESS",
        },
      ],
    },
    {
      key: "tests",
      title: t("dashboard.groupTests"),
      tiles: [
        { key: "planned", value: s.tests.planned, label: t("dashboard.testStatus.PLANNED"), tone: "info", to: "/tests?status=PLANNED" },
        { key: "inProgress", value: s.tests.inProgress, label: t("dashboard.testStatus.IN_PROGRESS"), tone: "warning", to: "/tests?status=IN_PROGRESS" },
        { key: "completed", value: s.tests.completed, label: t("dashboard.testStatus.COMPLETED"), tone: "success", to: "/tests?status=COMPLETED" },
        { key: "cancelled", value: s.tests.cancelled, label: t("dashboard.testStatus.CANCELLED"), tone: "default", to: "/tests?status=CANCELLED" },
      ],
    },
    {
      key: "experiments",
      title: t("dashboard.groupExperiments"),
      tiles: [
        { key: "draft", value: s.experiments.draft, label: t("experiments.status.DRAFT"), tone: "default", to: "/experiments?status=DRAFT" },
        { key: "experimentsRunning", value: s.experiments.running, label: t("experiments.status.RUNNING"), tone: "info", to: "/experiments?status=RUNNING" },
        { key: "experimentsCompleted", value: s.experiments.completed, label: t("experiments.status.COMPLETED"), tone: "success", to: "/experiments?status=COMPLETED" },
      ],
    },
  ];
});

// "5 min ago" / "in 2 days"; the exact time stays available as a tooltip.
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
  pending: "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 6v6l4 2",
  accepted: "M22 11.08V12a10 10 0 1 1-5.93-9.14M22 4L12 14.01l-3-3",
  rejected: "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM15 9l-6 6M9 9l6 6",
  planned: "M8 2v4M16 2v4M3 10h18M5 4h14a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2z",
  inProgress: "M22 12h-4l-3 9L9 3l-3 9H2",
  completed: "M22 11.08V12a10 10 0 1 1-5.93-9.14M22 4L12 14.01l-3-3",
  cancelled: "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM15 9l-6 6M9 9l6 6",
  queued: "M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20zM12 6v6l4 2",
  running: "M23 4v6h-6M1 20v-6h6M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15",
  failed: "M10.29 3.86L1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0zM12 9v4M12 17h.01",
  calibration: "M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z",
  draft: "M12 20h9M16.5 3.5a2.12 2.12 0 0 1 3 3L7 19l-4 1 1-4L16.5 3.5z",
  experimentsRunning: "M9 2h6M10 2v6L4 20a1 1 0 0 0 1 2h14a1 1 0 0 0 1-2L14 8V2",
  experimentsCompleted: "M22 11.08V12a10 10 0 1 1-5.93-9.14M22 4L12 14.01l-3-3",
};
</script>

<template>
  <PageHead :title="$t('dashboard.title')" :subtitle="$t('dashboard.intro')">
    <template #actions>
      <RouterLink to="/review" class="head-link" data-test="dashboard-review-link">{{ $t("dashboard.openReview") }} &rarr;</RouterLink>
    </template>
  </PageHead>
  <NAlert v-if="error" type="error" :title="error" style="margin-bottom: 16px" />
  <NSkeleton v-if="!summary && !error" text :repeat="6" />

  <template v-if="summary">
    <div class="groups">
    <section v-for="group in groups" :key="group.key" class="group">
      <h2 class="group-title">{{ group.title }}</h2>
      <div class="stats" :style="{ '--n': group.tiles.length }" :data-test="`tiles-${group.key}`">
        <component
          :is="tile.to ? RouterLink : 'div'"
          v-for="tile in group.tiles"
          :key="tile.key"
          :to="tile.to"
          class="tile"
          :class="[`tone-${tile.tone}`, { attention: tile.attention && tile.value > 0, quiet: tile.attention && tile.value === 0, clickable: tile.to }]"
          :data-test="`tile-${tile.key}`"
        >
          <span class="tile-icon">
            <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
              <path :d="ICONS[tile.key]" />
            </svg>
          </span>
          <span class="tile-value">{{ fmt.number(tile.value) }}</span>
          <span class="tile-label">{{ tile.label }}</span>
        </component>
      </div>
    </section>
    </div>

    <div class="grid-2">
      <NCard size="small" class="list-card">
        <template #header>{{ $t("dashboard.latestResults") }}</template>
        <ul v-if="summary.recentActivity.length" class="timeline" data-test="recent-activity">
          <li v-for="run in summary.recentActivity" :key="run.testSubjectId + run.finishedAt">
            <span class="dot dot-success"></span>
            <RouterLink class="link" :to="{ path: '/review', query: { status: 'ALL', open: run.testSubjectId } }" data-test="recent-result">
              {{ run.subjectCode }}
            </RouterLink>
            <span class="muted">{{ run.experimentCode }}</span>
            <NTag size="small" round :bordered="false">{{ text.paradigmName(run.paradigmKey) }}</NTag>
            <span class="muted time" :title="fmt.dateTime(run.finishedAt)">{{ relative(run.finishedAt) }}</span>
          </li>
        </ul>
        <p v-else class="muted empty">{{ $t("dashboard.noActivity") }}</p>
      </NCard>

      <NCard size="small" class="list-card">
        <template #header>{{ $t("dashboard.upcomingSessions") }}</template>
        <ul v-if="summary.upcomingTests.length" class="timeline" data-test="upcoming-tests">
          <li v-for="test in summary.upcomingTests" :key="test.id">
            <span class="dot dot-info"></span>
            <RouterLink class="link" :to="`/tests/${test.id}`" data-test="upcoming-test">
              {{ test.experimentCode }} · {{ test.timepointName }}
            </RouterLink>
            <span class="muted">{{ test.subjectCodes.join(", ") }}</span>
            <NTag size="small" round :bordered="false">{{ text.paradigmName(test.paradigmKey) }}</NTag>
            <span class="muted time" :title="fmt.dateTime(test.scheduledAt)">{{ relative(test.scheduledAt) }}</span>
          </li>
        </ul>
        <div v-else class="empty" data-test="upcoming-empty">
          <p class="muted">{{ $t("dashboard.noUpcoming") }}</p>
          <p class="hint">{{ $t("dashboard.noUpcomingHint") }}</p>
          <p class="empty-links">
            <RouterLink class="link" to="/experiments?status=RUNNING" data-test="upcoming-running">{{ $t("dashboard.runningStudies") }}</RouterLink>
            <a class="link" :href="$t('dashboard.planningDocs')" target="_blank" rel="noopener">{{ $t("dashboard.howToPlan") }}</a>
          </p>
        </div>
      </NCard>
    </div>
  </template>
</template>

<style scoped>
.head-link { font-size: 13.5px; font-weight: 600; color: var(--accent); text-decoration: none; white-space: nowrap; }
.head-link:hover { text-decoration: underline; }
/* Each group is one card; groups sit two per row, and the tiles of a group
   share its width equally, so rows never leave an empty gap. */
.groups { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 16px; margin-bottom: 16px; }
.group { padding: 14px; border-radius: 14px; background: var(--panel); border: 1px solid var(--line); box-shadow: var(--shadow-sm); min-width: 0; }
.group-title { margin: 0 0 12px; font-size: 12px; font-weight: 600; letter-spacing: 0.06em; text-transform: uppercase; color: var(--muted); }
.stats { display: grid; grid-template-columns: repeat(var(--n, 3), minmax(0, 1fr)); gap: 10px; }
.tile {
  --tone: var(--muted);
  display: flex; flex-direction: column; align-items: flex-start; gap: 6px; min-width: 0;
  padding: 12px; border-radius: 10px; background: var(--panel2); border: 1px solid var(--line);
  color: var(--txt); text-decoration: none;
  transition: transform 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
}
.tile.tone-info { --tone: var(--accent); }
.tile.tone-warning { --tone: var(--warn); }
.tile.tone-success { --tone: var(--good); }
.tile.tone-error { --tone: var(--bad); }
.tile-icon {
  display: inline-flex; align-items: center; justify-content: center; width: 38px; height: 38px;
  border-radius: 10px; color: var(--tone); background: color-mix(in srgb, var(--tone) 14%, transparent);
}
.tile-value { font-size: 28px; font-weight: 700; letter-spacing: -0.02em; line-height: 1.1; font-variant-numeric: tabular-nums; }
.tile-label { font-size: 12.5px; color: var(--muted); line-height: 1.3; }
.tile.clickable:hover { transform: translateY(-2px); box-shadow: var(--shadow-md); border-color: color-mix(in srgb, var(--tone) 45%, var(--line)); }
.tile.attention { border-color: color-mix(in srgb, var(--tone) 55%, var(--line)); background: color-mix(in srgb, var(--tone) 6%, var(--panel)); }
.tile.quiet { opacity: 0.6; }
.grid-2 { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 16px; align-items: stretch; }
.list-card { height: 100%; }
@media (max-width: 1100px) { .groups { grid-template-columns: minmax(0, 1fr); } }
@media (max-width: 860px) { .grid-2 { grid-template-columns: minmax(0, 1fr); } }
@media (max-width: 560px) { .stats { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
.timeline { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 2px; }
.timeline li { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 8px 10px; border-radius: 8px; }
.timeline li:hover { background: var(--panel2); }
.dot { width: 8px; height: 8px; border-radius: 50%; flex: none; }
.dot-success { background: var(--good); }
.dot-info { background: var(--accent); }
.time { margin-left: auto; font-size: 12.5px; white-space: nowrap; }
.empty { padding: 18px 8px; text-align: center; }
.empty p { margin: 0 0 6px; }
.empty .hint { font-size: 13px; }
.empty-links { display: flex; flex-wrap: wrap; justify-content: center; gap: 6px 16px; font-size: 13px; }
</style>
