<script setup>
// Inputs for a list of paradigm parameters. The model holds raw API values;
// each field shows its label, unit, allowed range and what the value means,
// and offers to go back to the paradigm's default.
import { NSwitch } from "naive-ui";
import { fromDisplay, isSwitch, toDisplay, useParameters } from "../composables/useParameters.js";

const props = defineProps({
  parameters: { type: Array, required: true },
  modelValue: { type: Object, required: true },
  // data-test of each input is `${testPrefix}-${parameter.key}`.
  testPrefix: { type: String, default: "parameter" },
});
const emit = defineEmits(["update:modelValue"]);
const { unit, format, range } = useParameters();

function set(parameter, raw) {
  emit("update:modelValue", { ...props.modelValue, [parameter.key]: raw });
}

function onInput(parameter, event) {
  const text = event.target.value;
  set(parameter, text === "" ? "" : fromDisplay(parameter, text));
}

const isDefault = (parameter) => Number(props.modelValue[parameter.key]) === parameter.default;
const step = (parameter) => (parameter.valueType === "integer" ? 1 : "any");
</script>

<template>
  <div class="params">
    <div class="fld" v-for="parameter in parameters" :key="parameter.key">
      <label :for="`${testPrefix}-${parameter.key}`" class="name">{{ parameter.label }}</label>
      <div v-if="isSwitch(parameter)" class="switch">
        <NSwitch
          :value="Number(modelValue[parameter.key]) === 1"
          :data-test="`${testPrefix}-${parameter.key}`"
          @update:value="(on) => set(parameter, on ? 1 : 0)"
        />
      </div>
      <div v-else class="input">
        <input
          :id="`${testPrefix}-${parameter.key}`"
          type="number"
          :step="step(parameter)"
          :min="toDisplay(parameter, parameter.min)"
          :max="toDisplay(parameter, parameter.max)"
          :value="toDisplay(parameter, modelValue[parameter.key])"
          :data-test="`${testPrefix}-${parameter.key}`"
          required
          @input="onInput(parameter, $event)"
        />
        <span class="unit" v-if="unit(parameter)">{{ unit(parameter) }}</span>
      </div>
      <small class="help" v-if="parameter.help">{{ parameter.help }}</small>
      <small class="muted">
        <span v-if="range(parameter)">{{ range(parameter) }} · </span>
        <span>{{ $t("paradigms.defaultValue", { value: format(parameter, parameter.default) }) }}</span>
        <a v-if="!isDefault(parameter)" href="#" class="link reset" @click.prevent="set(parameter, parameter.default)">
          {{ $t("paradigms.resetDefault") }}
        </a>
      </small>
    </div>
  </div>
</template>

<style scoped>
.params { display: grid; grid-template-columns: repeat(auto-fit, minmax(240px, 1fr)); gap: 16px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.name { font-size: 13px; font-weight: 600; }
.input { display: flex; align-items: center; gap: 6px; }
.input input { flex: 1; min-width: 0; }
.unit { font-size: 12px; color: var(--muted); white-space: nowrap; }
.help { font-size: 12px; line-height: 1.4; }
.muted { font-size: 11px; color: var(--muted); }
.reset { margin-left: 6px; }
.switch { min-height: 28px; display: flex; align-items: center; }
</style>
