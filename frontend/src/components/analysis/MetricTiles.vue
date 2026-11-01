<script setup>
// Metrics of one analysis as tiles: label, value with its unit in the panel's
// language, and why a value is missing. Shared by review and paradigm trials.
import { useNumbers } from "./format.js";

defineProps({
  // [{ key, unit, value, missingReason }]
  metrics: { type: Array, default: () => [] },
  // The metric's label in the panel's language, from the paradigm manifest.
  label: { type: Function, default: (key) => key },
  // data-test of the grid is `${testPrefix}s`, of each tile `${testPrefix}-${key}`.
  testPrefix: { type: String, default: "metric" },
});
const fmt = useNumbers();
</script>

<template>
  <div class="kpis" :data-test="`${testPrefix}s`">
    <div v-for="metric in metrics" :key="metric.key" class="kpi" :class="{ missing: metric.value === null }" :data-test="`${testPrefix}-${metric.key}`">
      <span class="kpi-label">{{ label(metric.key) }}</span>
      <span class="kpi-value">{{ metric.value === null ? $t("analysis.missing") : fmt.metric(metric.value, metric.unit, metric.key) }}</span>
      <span v-if="metric.value === null" class="kpi-note">{{ fmt.missingReason(metric.missingReason) }}</span>
    </div>
  </div>
</template>

<style scoped>
.kpis { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 8px; }
.kpi { background: var(--panel2); border: 1px solid var(--line); border-radius: 10px; padding: 8px 10px; display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.kpi.missing { border-color: color-mix(in srgb, var(--warn) 60%, var(--line)); }
.kpi-label { font-size: 11.5px; color: var(--muted); overflow-wrap: anywhere; }
.kpi-value { font-size: 17px; font-weight: 700; font-variant-numeric: tabular-nums; }
.kpi-note { font-size: 11px; color: var(--warn); }
</style>
