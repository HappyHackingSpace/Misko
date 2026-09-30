<script setup>
// The history of analysis runs of one recording. Earlier runs and what they
// published never change; a reanalysis is a new run. Opening a row shows that
// run's result.
import { computed, h } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NDataTable, NTag } from "naive-ui";

const props = defineProps({
  // Newest first.
  runs: { type: Array, default: () => [] },
  selectedId: { type: String, default: "" },
  tagType: { type: Function, required: true },
});
const emit = defineEmits(["open"]);
const { t } = useI18n();

const when = (value) => (value ? new Date(value).toLocaleString() : "");

const columns = computed(() => [
  { title: t("analysis.run"), key: "trigger", render: (run) => t(`analysis.${run.trigger}`) },
  {
    title: t("tests.status"),
    key: "status",
    render: (run) => h(NTag, { size: "small", round: true, bordered: false, type: props.tagType(run.status) }, () => t(`statuses.${run.status}`)),
  },
  { title: t("analysis.started"), key: "createdAt", render: (run) => when(run.createdAt) },
  { title: t("analysis.finished"), key: "finishedAt", render: (run) => when(run.finishedAt) || "…" },
  { title: t("analysis.model"), key: "modelVersion", render: (run) => run.modelVersion || "…" },
  {
    title: "",
    key: "open",
    align: "right",
    render: (run) =>
      h(
        NButton,
        { size: "small", secondary: run.id !== props.selectedId, type: run.id === props.selectedId ? "primary" : "default", "data-test": "run-open", onClick: () => emit("open", run.id) },
        () => t("analysis.openResult"),
      ),
  },
]);
</script>

<template>
  <div class="runs">
    <p class="muted" v-if="!runs.length" data-test="no-runs">{{ $t("analysis.noRuns") }}</p>
    <NDataTable
      v-if="runs.length"
      data-test="runs"
      :columns="columns"
      :data="runs"
      :row-key="(run) => run.id"
      :row-props="(run) => ({ 'data-test': 'run', 'data-run-id': run.id, 'data-selected': String(run.id === selectedId) })"
      size="small"
    />
  </div>
</template>

<style scoped>
.runs { display: flex; flex-direction: column; gap: 10px; }
</style>
