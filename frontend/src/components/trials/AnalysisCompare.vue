<script setup>
// Two analyses of a trial side by side: every metric of both, and how the
// second differs from the first. Only succeeded analyses have metrics.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NModal, NSelect } from "naive-ui";
import { selectOption } from "../../composables/selectOptions.js";
import { useNumbers } from "../analysis/format.js";

const props = defineProps({
  show: { type: Boolean, default: false },
  // TrialAnalysis items, newest first.
  analyses: { type: Array, default: () => [] },
  metricLabel: { type: Function, default: (key) => key },
});
const emit = defineEmits(["update:show"]);
const { t } = useI18n();
const fmt = useNumbers();

const succeeded = computed(() => props.analyses.filter((a) => a.job.status === "SUCCEEDED"));
const describe = (a) => `${t(`trials.analyses.triggers.${a.trigger}`)} · ${fmt.dateTime(a.createdAt)}`;
const options = (id) => succeeded.value.map((a) => selectOption(id, a.jobId, describe(a)));
// The earlier analysis on the left, the newer on the right.
const left = ref(null);
const right = ref(null);
watch(
  () => props.show,
  (shown) => {
    if (!shown) return;
    right.value = succeeded.value[0]?.jobId || null;
    left.value = succeeded.value[1]?.jobId || null;
  },
  { immediate: true },
);

const byJob = (id) => succeeded.value.find((a) => a.jobId === id) || null;
const rows = computed(() => {
  const a = byJob(left.value);
  const b = byJob(right.value);
  if (!a || !b) return [];
  const keys = [...new Set([...a.metrics, ...b.metrics].map((m) => m.key))];
  return keys.map((key) => {
    const x = a.metrics.find((m) => m.key === key);
    const y = b.metrics.find((m) => m.key === key);
    const unit = (x || y).unit;
    const both = x?.value != null && y?.value != null;
    return { key, x, y, unit, diff: both ? y.value - x.value : null };
  });
});
const cell = (m) => (!m ? "–" : m.value === null ? t("analysis.missing") : fmt.metric(m.value, m.unit, m.key));
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="$t('trials.compare.title')"
    style="width: 760px; max-width: calc(100vw - 16px)"
    data-test="trial-compare"
    @update:show="emit('update:show', $event)"
  >
    <div class="pick">
      <NSelect v-model:value="left" :options="options('compare-left')" data-test="compare-left" />
      <NSelect v-model:value="right" :options="options('compare-right')" data-test="compare-right" />
    </div>
    <div class="scroll">
      <table v-if="rows.length" class="table" data-test="compare-table">
        <thead>
          <tr>
            <th>{{ $t("trials.compare.metric") }}</th>
            <th>{{ $t("trials.compare.first") }}</th>
            <th>{{ $t("trials.compare.second") }}</th>
            <th>{{ $t("trials.compare.difference") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="row in rows" :key="row.key" data-test="compare-row" :data-metric="row.key">
            <td>{{ metricLabel(row.key) }}</td>
            <td>{{ cell(row.x) }}</td>
            <td>{{ cell(row.y) }}</td>
            <td :class="{ muted: !row.diff }">{{ row.diff == null ? "–" : fmt.signedMetric(row.diff, row.unit, row.key) }}</td>
          </tr>
        </tbody>
      </table>
      <p v-else class="muted">{{ $t("trials.compare.choose") }}</p>
    </div>
  </NModal>
</template>

<style scoped>
.pick { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 10px; margin-bottom: 14px; }
.scroll { overflow-x: auto; }
.table { width: 100%; border-collapse: collapse; font-size: 13px; }
.table th { text-align: left; font-weight: 600; font-size: 12px; color: var(--muted); padding: 6px; border-bottom: 1px solid var(--line); }
.table td { padding: 6px; border-bottom: 1px solid var(--line); font-variant-numeric: tabular-nums; }
@media (max-width: 520px) { .pick { grid-template-columns: minmax(0, 1fr); } }
</style>
