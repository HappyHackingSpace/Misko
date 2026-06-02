<script setup>
import { ref, watch, computed, onMounted } from "vue";
import { useI18n } from "vue-i18n";
import { api } from "../api.js";

const { locale } = useI18n();

/**
 * User-defined acceptance criteria builder.
 *
 * The user first picks a paradigm (to source the metric list), then adds
 * criterion rows with metric + operator + threshold value. Criteria are
 * optional; an empty list means the test will not be evaluated.
 */
const props = defineProps({
  modelValue: { type: Array, default: () => [] },
  paradigms: { type: Array, default: () => [] },
  operators: { type: Array, default: () => [] },
});
const emit = defineEmits(["update:modelValue"]);

const rows = ref(props.modelValue.map((c) => ({ ...c })));
const paradigmKey = ref("");
const metrics = ref([]);
const suggested = ref([]);
const err = ref("");

watch(
  () => props.modelValue,
  (v) => { rows.value = (v || []).map((c) => ({ ...c })); },
);

function emitChange() {
  // Only emit fully filled-in rows upward.
  const clean = rows.value
    .filter((r) => r.metricKey && r.operator)
    .map((r) => {
      if (r.operator === "between") {
        return { metricKey: r.metricKey, operator: r.operator, value: [Number(r.min), Number(r.max)] };
      }
      return { metricKey: r.metricKey, operator: r.operator, value: Number(r.value) };
    });
  emit("update:modelValue", clean);
}

async function loadMetrics() {
  err.value = "";
  metrics.value = [];
  suggested.value = [];
  if (!paradigmKey.value) return;
  try {
    metrics.value = await api(`/paradigms/metrics?paradigm=${paradigmKey.value}&lang=${locale.value}`);
    const spec = await api(`/paradigms/${paradigmKey.value}?lang=${locale.value}`);
    suggested.value = spec.suggestedAcceptance || [];
  } catch (e) { err.value = e.message; }
}

// On language change, refresh loaded metric labels (if a paradigm is selected).
watch(locale, () => { if (paradigmKey.value) loadMetrics(); });

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
function adoptSuggested(s) {
  // Suggestions with symbolic values (e.g. "max_trial_duration_s") are not
  // numeric; the user enters the value manually. Numeric ones are adopted directly.
  const numeric = typeof s.value === "number";
  rows.value.push({
    metricKey: s.metricKey,
    operator: s.operator === "between" ? "between" : s.operator,
    value: numeric ? s.value : "",
    min: "", max: "",
  });
  emitChange();
}

const hasParadigm = computed(() => Boolean(paradigmKey.value));

onMounted(() => {
  // If criteria are already saved, the paradigm choice is left to the user;
  // metric labels become richer once a paradigm is selected.
});
</script>

<template>
  <div class="accept-editor">
    <div class="field">
      <label>{{ $t("acceptance.sourceParadigm") }}</label>
      <select v-model="paradigmKey" @change="loadMetrics">
        <option value="" disabled>{{ $t("common.select") }}</option>
        <option v-for="p in paradigms" :key="p.key" :value="p.key">{{ p.name }}</option>
      </select>
    </div>

    <p class="err" v-if="err">{{ err }}</p>

    <div v-if="suggested.length" class="suggested">
      <span class="muted">{{ $t("acceptance.suggestions") }}:</span>
      <button
        v-for="s in suggested"
        :key="s.key"
        class="chip-btn"
        type="button"
        @click="adoptSuggested(s)"
      >+ {{ s.metricKey }} {{ s.operator }} {{ s.value }}</button>
    </div>

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
            <select v-model="r.metricKey" @change="emitChange" :disabled="!hasParadigm">
              <option value="" disabled>{{ $t("common.select") }}</option>
              <option v-for="m in metrics" :key="m.key" :value="m.key">{{ metricLabel(m.key) }}</option>
            </select>
          </td>
          <td>
            <select v-model="r.operator" @change="emitChange">
              <option v-for="op in operators" :key="op" :value="op">{{ op }}</option>
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

    <button type="button" @click="addRow" :disabled="!hasParadigm">+ {{ $t("acceptance.addRow") }}</button>
    <p class="muted" v-if="!hasParadigm">{{ $t("acceptance.pickParadigmFirst") }}</p>
    <p class="muted" v-else-if="!rows.length">{{ $t("acceptance.optionalHint") }}</p>
  </div>
</template>

<style scoped>
.accept-editor { display: flex; flex-direction: column; gap: 10px; }
.suggested { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
.chip-btn {
  background: var(--active-bg); border: 1px solid var(--line);
  border-radius: 999px; padding: 2px 10px; cursor: pointer; font-size: 12px;
}
.num { width: 90px; }
.unit { margin-left: 6px; }
</style>
