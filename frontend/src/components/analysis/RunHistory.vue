<script setup>
// Every analysis run of one animal in a test, newest first. Runs never change;
// a reanalysis is a new run, so earlier ones stay as provenance.
import { computed, h } from "vue";
import { useI18n } from "vue-i18n";
import { NDataTable, NTag } from "naive-ui";
import { runTag, useNumbers } from "./format.js";

const props = defineProps({
  runs: { type: Array, default: () => [] },
  // The run the reviewed result points at, marked in the list.
  reviewedRunId: { type: String, default: "" },
  loading: { type: Boolean, default: false },
});
const { t } = useI18n();
const fmt = useNumbers();

const columns = computed(() => [
  {
    title: t("runs.status"),
    key: "status",
    render: (run) =>
      h("span", { class: "status-cell" }, [
        h(NTag, { size: "small", round: true, bordered: false, type: runTag(run.status) }, () => t(`runs.statuses.${run.status}`)),
        run.id === props.reviewedRunId ? h(NTag, { size: "small", round: true, bordered: false, type: "info" }, () => t("review.underReview")) : null,
      ]),
  },
  { title: t("runs.trigger"), key: "trigger", render: (run) => t(`analysis.${run.trigger}`) },
  { title: t("runs.created"), key: "createdAt", render: (run) => fmt.dateTime(run.createdAt) },
  { title: t("runs.finished"), key: "finishedAt", render: (run) => fmt.dateTime(run.finishedAt) || "–" },
  { title: t("runs.attempts"), key: "attempt", render: (run) => `${run.attempt}/${run.maxAttempts}` },
  { title: t("runs.model"), key: "modelVersion", render: (run) => run.modelVersion || "–" },
  { title: t("runs.engine"), key: "metricEngineVersion" },
  { title: t("runs.failure"), key: "failureReason", render: (run) => fmt.failure(run.failureReason) || "–" },
]);
</script>

<template>
  <NDataTable
    data-test="run-history"
    size="small"
    :columns="columns"
    :data="runs"
    :loading="loading"
    :row-key="(run) => run.id"
    :row-props="(run) => ({ 'data-test': 'run-row', 'data-run-id': run.id, 'data-status': run.status })"
  >
    <template #empty><span class="muted">{{ $t("runs.none") }}</span></template>
  </NDataTable>
</template>

<style scoped>
:deep(.status-cell) { display: inline-flex; gap: 6px; align-items: center; }
</style>
