<script setup>
// The tracking quality a job measured on its trajectory: how many samples
// were tracked, lost or inferred, the mean confidence and whether the
// paradigm's QC rules passed.
import { NTag } from "naive-ui";
import { useNumbers } from "./format.js";

defineProps({
  // { samples, tracked, inferred, lostRatio, inferredRatio, meanConfidence, passed }
  qc: { type: Object, required: true },
});
const fmt = useNumbers();
const percent = (ratio) => `${fmt.number(ratio * 100)}%`;
</script>

<template>
  <div class="qc" data-test="qc" :data-passed="qc.passed">
    <NTag size="small" round :bordered="false" :type="qc.passed ? 'success' : 'error'">{{ qc.passed ? $t("qc.passed") : $t("qc.failed") }}</NTag>
    <span>{{ $t("qc.tracked", { tracked: fmt.number(qc.tracked), samples: fmt.number(qc.samples) }) }}</span>
    <span>{{ $t("qc.lost", { value: percent(qc.lostRatio) }) }}</span>
    <span>{{ $t("qc.inferred", { value: percent(qc.inferredRatio) }) }}</span>
    <span>{{ $t("qc.confidence", { value: percent(qc.meanConfidence) }) }}</span>
  </div>
</template>

<style scoped>
.qc { display: flex; flex-wrap: wrap; align-items: center; gap: 6px 14px; font-size: 12.5px; color: var(--muted); }
</style>
