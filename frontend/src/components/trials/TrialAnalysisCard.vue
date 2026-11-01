<script setup>
// One analysis of a trial: why it ran and with which changed thresholds, the
// job's state (with the reason when it failed), its QC and metrics, and on
// demand its overlay video and events.
import { computed } from "vue";
import { NAlert, NCollapse, NCollapseItem, NTag } from "naive-ui";
import { useParameters } from "../../composables/useParameters.js";
import { useNumbers } from "../analysis/format.js";
import JobOutputs from "../analysis/JobOutputs.vue";
import MetricTiles from "../analysis/MetricTiles.vue";
import QcSummary from "../analysis/QcSummary.vue";
import JobStatusTag from "./JobStatusTag.vue";

const props = defineProps({
  // A TrialAnalysis: { jobId, trigger, overrides, createdAt, job, metrics, qc }.
  analysis: { type: Object, required: true },
  // The paradigm's parameters, for the labels of changed thresholds.
  parameters: { type: Array, default: () => [] },
  metricLabel: { type: Function, default: (key) => key },
  eventLabels: { type: Object, default: () => ({}) },
});
const fmt = useNumbers();
const params = useParameters();

const job = computed(() => props.analysis.job);
const finished = computed(() => job.value.status === "SUCCEEDED" || job.value.status === "FAILED");
const changes = computed(() =>
  Object.entries(props.analysis.overrides || {}).map(([key, value]) => {
    const parameter = props.parameters.find((p) => p.key === key);
    return { key, text: parameter ? `${parameter.label}: ${params.format(parameter, value)}` : `${key}: ${value}` };
  }),
);
</script>

<template>
  <article class="analysis" data-test="trial-analysis" :data-job="analysis.jobId" :data-status="job.status" :data-trigger="analysis.trigger">
    <header class="head">
      <JobStatusTag :status="job.status" />
      <strong>{{ $t(`trials.analyses.triggers.${analysis.trigger}`) }}</strong>
      <span class="muted">{{ fmt.dateTime(analysis.createdAt) }}</span>
      <span v-if="finished && job.finishedAt" class="muted">· {{ $t("trials.analyses.finished", { at: fmt.dateTime(job.finishedAt) }) }}</span>
      <span v-if="job.modelVersion" class="muted">· {{ job.modelVersion }}</span>
    </header>
    <div v-if="changes.length" class="changes" data-test="trial-analysis-overrides">
      <span class="muted">{{ $t("trials.analyses.changed") }}</span>
      <NTag v-for="change in changes" :key="change.key" size="small" round :bordered="false">{{ change.text }}</NTag>
    </div>

    <p v-if="job.status === 'QUEUED' || job.status === 'RUNNING'" class="muted" data-test="trial-analysis-waiting">
      {{ $t(`trials.analyses.active.${job.status}`) }}
    </p>
    <NAlert v-if="job.status === 'FAILED'" type="error" :show-icon="false" data-test="trial-analysis-failure">
      {{ $t("trials.analyses.failed", { reason: fmt.failure(job.failureReason) || $t("trials.analyses.noReason") }) }}
    </NAlert>
    <QcSummary v-if="analysis.qc" :qc="analysis.qc" />
    <MetricTiles v-if="analysis.metrics.length" :metrics="analysis.metrics" :label="metricLabel" test-prefix="trial-metric" />

    <NCollapse v-if="job.status === 'SUCCEEDED'">
      <NCollapseItem :title="$t('trials.analyses.outputs')" name="outputs" data-test="trial-analysis-outputs">
        <JobOutputs :job-id="analysis.jobId" :event-labels="eventLabels" />
      </NCollapseItem>
    </NCollapse>
  </article>
</template>

<style scoped>
.analysis { display: flex; flex-direction: column; gap: 10px; padding: 12px 0; min-width: 0; }
.analysis + .analysis { border-top: 1px solid var(--line); }
.head { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 8px; font-size: 13px; }
.changes { display: flex; align-items: center; flex-wrap: wrap; gap: 6px; font-size: 12.5px; }
p { margin: 0; }
</style>
