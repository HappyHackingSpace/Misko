<script setup>
// A new protocol with its first version, a new version of an existing one
// (earlier versions never change), or a new name and description.
import { computed, ref, watch } from "vue";
import { NAlert, NButton, NInput, NModal } from "naive-ui";
import { protocols } from "../../../api/protocols.js";
import StepsEditor from "./StepsEditor.vue";

const props = defineProps({
  show: { type: Boolean, default: false },
  experimentId: { type: String, required: true },
  // "create" | "version" | "rename"
  mode: { type: String, default: "create" },
  protocol: { type: Object, default: null },
  // Steps of the latest version, the starting point of a new version.
  steps: { type: Array, default: () => [] },
});
const emit = defineEmits(["update:show", "saved"]);

const name = ref("");
const description = ref("");
const notes = ref("");
const stepRows = ref([]);
const error = ref("");
const saving = ref(false);

const blankStep = () => ({ paradigmKey: null, paradigmVersion: null, trialType: null, trials: 1, interTrialIntervalS: 0, session: {}, notes: "" });

watch(
  () => props.show,
  (open) => {
    if (!open) return;
    error.value = "";
    name.value = props.protocol?.name || "";
    description.value = props.protocol?.description || "";
    notes.value = "";
    stepRows.value = props.mode === "version" && props.steps.length
      ? props.steps.map((s) => ({ ...s, session: { ...(s.session || {}) }, notes: s.notes || "" }))
      : [blankStep()];
  },
);

const withSteps = computed(() => props.mode !== "rename");

function versionBody() {
  const body = {
    steps: stepRows.value.map((s, i) => {
      const step = {
        position: i + 1,
        paradigmKey: s.paradigmKey,
        paradigmVersion: s.paradigmVersion,
        trialType: s.trialType,
        trials: s.trials,
        interTrialIntervalS: s.interTrialIntervalS || 0,
        session: Object.fromEntries(Object.entries(s.session || {}).filter(([, v]) => v !== "" && v !== null && v !== undefined).map(([k, v]) => [k, Number(v)])),
      };
      if (s.notes?.trim()) step.notes = s.notes.trim();
      return step;
    }),
  };
  if (notes.value.trim()) body.notes = notes.value.trim();
  return body;
}

async function save() {
  error.value = "";
  saving.value = true;
  try {
    if (props.mode === "create") {
      const body = { name: name.value.trim(), version: versionBody() };
      if (description.value.trim()) body.description = description.value.trim();
      await protocols.create(props.experimentId, body);
    } else if (props.mode === "version") {
      await protocols.addVersion(props.experimentId, props.protocol.id, versionBody());
    } else {
      await protocols.update(props.experimentId, props.protocol.id, { name: name.value.trim(), description: description.value.trim() });
    }
    emit("saved");
    emit("update:show", false);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

const ready = computed(() => {
  if (props.mode !== "version" && !name.value.trim()) return false;
  if (!withSteps.value) return true;
  return stepRows.value.length && stepRows.value.every((s) => s.paradigmKey && s.paradigmVersion && s.trialType && s.trials);
});
</script>

<template>
  <NModal
    :show="show"
    preset="card"
    :title="mode === 'create' ? $t('design.protocols.add') : mode === 'version' ? $t('design.protocols.newVersionOf', { name: protocol?.name }) : $t('design.protocols.rename')"
    style="width: 820px; max-width: 96vw"
    @update:show="emit('update:show', $event)"
  >
    <div class="form" data-test="protocol-form">
      <template v-if="mode !== 'version'">
        <label class="fld">
          <span>{{ $t("common.name") }}</span>
          <NInput v-model:value="name" :maxlength="120" :input-props="{ 'data-test': 'protocol-name' }" />
        </label>
        <label class="fld">
          <span>{{ $t("common.description") }}</span>
          <NInput v-model:value="description" type="textarea" :autosize="{ minRows: 1, maxRows: 4 }" :maxlength="5000" />
        </label>
      </template>
      <template v-if="withSteps">
        <p v-if="mode === 'version'" class="muted">{{ $t("design.protocols.versionHint") }}</p>
        <label v-if="mode === 'version'" class="fld">
          <span>{{ $t("design.protocols.versionNotes") }}</span>
          <NInput v-model:value="notes" :maxlength="2000" :input-props="{ 'data-test': 'protocol-version-notes' }" />
        </label>
        <StepsEditor v-model="stepRows" />
      </template>
      <NAlert v-if="error" type="error" :title="error" data-test="protocol-error" />
    </div>
    <template #footer>
      <div class="dialog-actions">
        <NButton @click="emit('update:show', false)">{{ $t("common.cancel") }}</NButton>
        <NButton type="primary" :loading="saving" :disabled="!ready" data-test="protocol-save" @click="save">{{ $t("common.save") }}</NButton>
      </div>
    </template>
  </NModal>
</template>

<style scoped>
.form { display: flex; flex-direction: column; gap: 12px; }
.fld { display: flex; flex-direction: column; gap: 4px; }
.fld > span { font-size: 12px; color: var(--muted); }
p { margin: 0; }
.dialog-actions { display: flex; justify-content: flex-end; gap: 8px; }
</style>
