<script setup>
import { ref, watch, computed } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";

const { locale, t } = useI18n();

// Map raw operator symbols to i18n keys for human-readable labels.
const OP_KEY = { "<": "lt", "<=": "lte", ">": "gt", ">=": "gte", "==": "eq", "!=": "ne", between: "between" };
const operatorLabel = (op) => t(`acceptance.op.${OP_KEY[op] || "eq"}`);

/**
 * Expected-results (acceptance criteria) builder for a Scenario.
 *
 * The metric source is the UNION of the scenario's environments' paradigms,
 * passed in as `paradigmKeys`. The user adds criterion rows (metric + operator
 * + threshold). Criteria are optional; an empty list means the test will not be
 * evaluated.
 */
const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  paradigmKeys: { type: Array, default: () => [] },
  operators: { type: Array, default: () => [] },
});
const emit = defineEmits(["update:modelValue"]);

function normalize(c) {
  if (c.operator === "between" && Array.isArray(c.value)) {
    return { metricKey: c.metricKey, operator: c.operator, value: "", min: c.value[0], max: c.value[1] };
  }
  return { metricKey: c.metricKey, operator: c.operator, value: c.value, min: "", max: "" };
}

const rows = ref(props.modelValue.map((c) => normalize(c)));
const metrics = ref([]);
const err = ref("");

watch(
  () => props.modelValue,
  (v) => { rows.value = (v || []).map((c) => normalize(c)); },
);

function emitChange() {
  const clean = rows.value
    .filter((r) => r.metricKey && r.operator)
    .map((r) => (r.operator === "between"
      ? { metricKey: r.metricKey, operator: r.operator, value: [Number(r.min), Number(r.max)] }
      : { metricKey: r.metricKey, operator: r.operator, value: Number(r.value) }));
  emit("update:modelValue", clean);
}

/** Loads and merges (dedupes by key) the metrics of all source paradigms. */
async function loadMetrics() {
  err.value = "";
  const keys = props.paradigmKeys || [];
  if (!keys.length) { metrics.value = []; return; }
  try {
    const lists = await Promise.all(
      keys.map((k) => api(`/paradigms/metrics?paradigm=${k}&lang=${locale.value}`)),
    );
    const byKey = new Map();
    for (const list of lists) for (const m of list) if (!byKey.has(m.key)) byKey.set(m.key, m);
    metrics.value = [...byKey.values()];
  } catch (e) { err.value = e.message; }
}

watch(() => props.paradigmKeys, loadMetrics, { immediate: true, deep: true });
watch(locale, loadMetrics);

const hasMetrics = computed(() => metrics.value.length > 0);
const metricLabel = (key) => {
  const m = metrics.value.find((x) => x.key === key);
  return m ? `${m.label} (${m.unit})` : key;
};
const unitFor = (key) => {
  const m = metrics.value.find((x) => x.key === key);
  return m ? m.unit : "";
};

function addRow() {
  rows.value.push({ metricKey: "", operator: "<=", value: "", min: "", max: "" });
}
function removeRow(i) {
  rows.value.splice(i, 1);
  emitChange();
}
</script>

<template>
  <div class="accept-editor">
    <p class="err" v-if="err">{{ err }}</p>

    <table v-if="rows.length">
      <thead>
        <tr>
          <th>{{ $t("acceptance.metric") }}</th>
          <th>{{ $t("acceptance.operator") }}</th>
          <th>{{ $t("acceptance.value") }}</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="(r, i) in rows" :key="i">
          <td>
            <select v-model="r.metricKey" @change="emitChange" :disabled="!hasMetrics">
              <option value="" disabled>{{ $t("common.select") }}</option>
              <option v-for="m in metrics" :key="m.key" :value="m.key">{{ metricLabel(m.key) }}</option>
            </select>
          </td>
          <td>
            <select v-model="r.operator" @change="emitChange">
              <option v-for="op in operators" :key="op" :value="op">{{ operatorLabel(op) }}</option>
            </select>
          </td>
          <td>
            <template v-if="r.operator === 'between'">
              <input type="number" class="num" v-model="r.min" @input="emitChange" :placeholder="$t('acceptance.min')" />
              <input type="number" class="num" v-model="r.max" @input="emitChange" :placeholder="$t('acceptance.max')" />
            </template>
            <input v-else type="number" class="num" v-model="r.value" @input="emitChange" />
            <span class="muted unit">{{ unitFor(r.metricKey) }}</span>
          </td>
          <td><button class="danger" type="button" @click="removeRow(i)">×</button></td>
        </tr>
      </tbody>
    </table>

    <button type="button" @click="addRow" :disabled="!hasMetrics">+ {{ $t("acceptance.addRow") }}</button>
    <p class="muted" v-if="!hasMetrics">{{ $t("acceptance.pickEnvironmentFirst") }}</p>
    <p class="muted" v-else-if="!rows.length">{{ $t("acceptance.optionalHint") }}</p>
  </div>
</template>

<style scoped>
.accept-editor { display: flex; flex-direction: column; gap: 10px; }
.num { width: 90px; }
.unit { margin-left: 6px; }
</style>
