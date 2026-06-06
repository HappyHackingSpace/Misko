<script setup>
import { ref, watch } from "vue";

/**
 * Auto-generated metric entry form for one environment, built from the paradigm's
 * metric dictionary. Field type comes from `valueType`; `validRange` sets min/max;
 * `templated` metrics (zone-keyed) render one numeric input per environment zone.
 * The same shape the CV service will produce later. Emits a metrics object:
 *   { metricKey: number | boolean, templatedKey: { zoneKey: number } }
 */
const props = defineProps({
  metrics: { type: Array, default: () => [] }, // metric definitions
  zones: { type: Array, default: () => [] }, // environment zones [{ key, label }]
  modelValue: { type: Object, default: () => ({}) },
});
const emit = defineEmits(["update:modelValue"]);

function buildBuffer() {
  const o = {};
  for (const m of props.metrics) {
    if (m.templated) {
      o[m.key] = {};
      for (const z of props.zones) o[m.key][z.key] = props.modelValue[m.key]?.[z.key] ?? "";
    } else if (m.valueType === "boolean") {
      o[m.key] = props.modelValue[m.key] ?? false;
    } else {
      o[m.key] = props.modelValue[m.key] ?? "";
    }
  }
  return o;
}

const buf = ref(buildBuffer());
watch(() => [props.metrics, props.zones], () => { buf.value = buildBuffer(); }, { deep: true });

function emitChange() {
  const out = {};
  for (const m of props.metrics) {
    const v = buf.value[m.key];
    if (m.templated) {
      const zoneObj = {};
      for (const z of props.zones) {
        const zv = v?.[z.key];
        if (zv !== "" && zv != null && Number.isFinite(Number(zv))) zoneObj[z.key] = Number(zv);
      }
      if (Object.keys(zoneObj).length) out[m.key] = zoneObj;
    } else if (m.valueType === "boolean") {
      if (typeof v === "boolean") out[m.key] = v;
    } else if (v !== "" && v != null && Number.isFinite(Number(v))) {
      out[m.key] = Number(v);
    }
  }
  emit("update:modelValue", out);
}

const rangeHint = (m) => (m.validRange ? `${m.validRange[0]}–${m.validRange[1]}` : "");
</script>

<template>
  <div class="metrics-form">
    <div v-for="m in metrics" :key="m.key" class="metric">
      <label class="metric-label">
        {{ m.label }} <span class="muted">({{ m.unit }}<template v-if="m.required">, *</template>)</span>
      </label>

      <!-- boolean -->
      <label v-if="m.valueType === 'boolean' && !m.templated" class="bool">
        <input type="checkbox" v-model="buf[m.key]" @change="emitChange" />
      </label>

      <!-- templated: one input per zone -->
      <div v-else-if="m.templated" class="zones">
        <p v-if="!zones.length" class="muted">{{ $t("results.noZones") }}</p>
        <div v-for="z in zones" :key="z.key" class="zone-row">
          <span class="zone-name">{{ z.label || z.key }}</span>
          <input type="number" v-model="buf[m.key][z.key]" @input="emitChange" :placeholder="rangeHint(m)" />
        </div>
      </div>

      <!-- number / integer -->
      <input v-else type="number" :step="m.valueType === 'integer' ? 1 : 'any'"
             v-model="buf[m.key]" @input="emitChange" :placeholder="rangeHint(m)" />
    </div>
  </div>
</template>

<style scoped>
.metrics-form { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; }
.metric-label { display: block; font-size: 12px; color: var(--muted); margin-bottom: 4px; }
.zones { display: flex; flex-direction: column; gap: 4px; }
.zone-row { display: flex; align-items: center; gap: 8px; }
.zone-name { flex: 1; font-size: 13px; }
.zone-row input { width: 110px; }
.bool input { width: auto; }
</style>
