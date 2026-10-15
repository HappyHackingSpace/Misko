<script setup>
// The steps of one protocol version: which paradigm version runs, which trial
// type, how many trials with what pause, and the session settings of that
// paradigm. Positions follow the order on screen.
import { computed, watch, watchEffect } from "vue";
import { useI18n } from "vue-i18n";
import { NButton, NInput, NInputNumber, NSelect } from "naive-ui";
import { filterByText, selectOption } from "../../../composables/selectOptions.js";
import { defaults } from "../../../composables/useParameters.js";
import ParameterFields from "../../../components/ParameterFields.vue";
import { useParadigms } from "./shared.js";

const props = defineProps({
  // [{ paradigmKey, paradigmVersion, trialType, trials, interTrialIntervalS, session, notes }]
  modelValue: { type: Array, required: true },
});
const emit = defineEmits(["update:modelValue"]);
const { t } = useI18n();
const paradigms = useParadigms();

watchEffect(() => {
  paradigms.loadList();
  for (const step of props.modelValue) {
    if (step.paradigmKey && step.paradigmVersion) paradigms.loadManifest(step.paradigmKey, step.paradigmVersion);
  }
});

const paradigmOptions = computed(() => paradigms.list().map((p) => selectOption("step-paradigm", p.key, p.name)));
const versionOptions = (key) =>
  (paradigms.list().find((p) => p.key === key)?.versions || []).map((v) =>
    selectOption("step-version", v, t("design.protocols.versionN", { n: v }), v === paradigms.list().find((p) => p.key === key)?.latestVersion ? t("design.protocols.latest") : ""),
  );
const manifestOf = (step) => (step.paradigmKey && step.paradigmVersion ? paradigms.manifest(step.paradigmKey, step.paradigmVersion) : null);
const trialTypeOptions = (step) => (manifestOf(step)?.trialTypes || []).map((tt) => selectOption("step-trial-type", tt.key, tt.label));
const sessionParameters = (step) => (manifestOf(step)?.parameters || []).filter((p) => p.scope === "SESSION");

function update(index, patch) {
  const next = props.modelValue.map((s, i) => (i === index ? { ...s, ...patch } : s));
  emit("update:modelValue", next);
}

function setParadigm(index, key) {
  const latest = paradigms.list().find((p) => p.key === key)?.latestVersion || 1;
  update(index, { paradigmKey: key, paradigmVersion: latest, trialType: null, session: {} });
}
function setVersion(index, version) {
  update(index, { paradigmVersion: version, trialType: null, session: {} });
}

// When a manifest arrives, pick its only trial type and fill in session
// defaults that are still missing, so the form starts valid.
watch(
  () => props.modelValue.map((s) => manifestOf(s)),
  (manifests) => {
    let changed = false;
    const next = props.modelValue.map((step, i) => {
      const m = manifests[i];
      if (!m) return step;
      const patch = {};
      if (!step.trialType && m.trialTypes.length === 1) patch.trialType = m.trialTypes[0].key;
      const params = m.parameters.filter((p) => p.scope === "SESSION");
      const missing = params.filter((p) => step.session?.[p.key] === undefined);
      if (missing.length) patch.session = { ...defaults(params), ...(step.session || {}) };
      if (Object.keys(patch).length) {
        changed = true;
        return { ...step, ...patch };
      }
      return step;
    });
    if (changed) emit("update:modelValue", next);
  },
  { immediate: true },
);

function add() {
  emit("update:modelValue", [
    ...props.modelValue,
    { paradigmKey: null, paradigmVersion: null, trialType: null, trials: 1, interTrialIntervalS: 0, session: {}, notes: "" },
  ]);
}
function remove(index) {
  emit("update:modelValue", props.modelValue.filter((_, i) => i !== index));
}
function move(index, delta) {
  const next = [...props.modelValue];
  const [step] = next.splice(index, 1);
  next.splice(index + delta, 0, step);
  emit("update:modelValue", next);
}
</script>

<template>
  <div class="steps">
    <div v-for="(step, index) in modelValue" :key="index" class="step" :data-test="`step-${index + 1}`">
      <div class="step-head">
        <strong>{{ $t("design.protocols.stepN", { n: index + 1 }) }}</strong>
        <div class="step-actions">
          <NButton size="tiny" quaternary :disabled="index === 0" :aria-label="$t('design.protocols.moveUp')" @click="move(index, -1)">↑</NButton>
          <NButton size="tiny" quaternary :disabled="index === modelValue.length - 1" :aria-label="$t('design.protocols.moveDown')" @click="move(index, 1)">↓</NButton>
          <NButton size="tiny" quaternary type="error" :disabled="modelValue.length === 1" @click="remove(index)">{{ $t("common.delete") }}</NButton>
        </div>
      </div>
      <div class="grid">
        <div class="fld">
          <span>{{ $t("design.protocols.paradigm") }}</span>
          <NSelect :value="step.paradigmKey" :options="paradigmOptions" filterable :filter="filterByText" data-test="step-paradigm" @update:value="setParadigm(index, $event)" />
        </div>
        <div class="fld">
          <span>{{ $t("design.protocols.paradigmVersion") }}</span>
          <NSelect :value="step.paradigmVersion" :options="versionOptions(step.paradigmKey)" :disabled="!step.paradigmKey" data-test="step-version" @update:value="setVersion(index, $event)" />
        </div>
        <div class="fld">
          <span>{{ $t("design.protocols.trialType") }}</span>
          <NSelect :value="step.trialType" :options="trialTypeOptions(step)" :disabled="!manifestOf(step)" data-test="step-trial-type" @update:value="update(index, { trialType: $event })" />
        </div>
        <label class="fld">
          <span>{{ $t("design.protocols.trials") }}</span>
          <NInputNumber :value="step.trials" :min="1" :max="1000" :precision="0" :input-props="{ 'data-test': 'step-trials' }" @update:value="update(index, { trials: $event })" />
        </label>
        <label class="fld">
          <span>{{ $t("design.protocols.interval") }}</span>
          <NInputNumber :value="step.interTrialIntervalS" :min="0" :max="86400" :precision="0" :input-props="{ 'data-test': 'step-interval' }" @update:value="update(index, { interTrialIntervalS: $event })">
            <template #suffix>{{ $t("design.protocols.seconds") }}</template>
          </NInputNumber>
        </label>
      </div>
      <div v-if="sessionParameters(step).length" class="session">
        <small class="muted">{{ $t("design.protocols.sessionSettings") }}</small>
        <ParameterFields
          :parameters="sessionParameters(step)"
          :model-value="step.session || {}"
          :test-prefix="`step-${index + 1}-session`"
          @update:model-value="update(index, { session: $event })"
        />
      </div>
      <label class="fld notes">
        <span>{{ $t("common.notes") }}</span>
        <NInput :value="step.notes" :maxlength="2000" @update:value="update(index, { notes: $event })" />
      </label>
    </div>
    <NButton dashed block data-test="step-add" @click="add">{{ $t("design.protocols.addStep") }}</NButton>
  </div>
</template>

<style scoped>
.steps { display: flex; flex-direction: column; gap: 12px; }
.step { border: 1px solid var(--line); border-radius: 8px; padding: 10px 12px; display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.step-head { display: flex; align-items: center; justify-content: space-between; }
.step-actions { display: flex; gap: 2px; }
.grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; }
.fld { display: flex; flex-direction: column; gap: 4px; min-width: 0; }
.fld > span { font-size: 12px; color: var(--muted); }
.session { display: flex; flex-direction: column; gap: 8px; border-top: 1px dashed var(--line); padding-top: 8px; }
</style>
