<script setup>
// Which calibration the trial's video is analysed with: the arena's default
// or the trial's own, read from this video. Calibrating here never changes
// the arena; it applies to this trial and this video only.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NTag } from "naive-ui";
import { trialCalibration, trials } from "../../api/trials.js";
import { tagType, useExecutionFormat } from "../../composables/useExecutionFormat.js";
import CalibrationModal from "../calibration/CalibrationModal.vue";

const props = defineProps({
  // The trial detail: id, video and arena (with its parameter values).
  trial: { type: Object, required: true },
  // The paradigm version's manifest, for the calibrator's zones and presets.
  paradigm: { type: Object, default: null },
  canRun: { type: Boolean, default: false },
});
const emit = defineEmits(["saved"]);
const { t } = useI18n();
const fmt = useExecutionFormat();

const state = ref(null);
const error = ref("");
const open = ref(false);

const status = computed(() => state.value?.status || null);
const source = computed(() => state.value?.source || null);
const current = computed(() => state.value?.calibration || null);
const canCalibrate = computed(() => props.canRun && props.trial.video?.status === "VERIFIED" && status.value && status.value !== "NOT_REQUIRED");
// What the trial is analysed with, in a sentence.
const explanation = computed(() => {
  if (status.value === "CALIBRATED" && source.value) return t(`trials.calibration.sources.${source.value}`);
  if (status.value === "WAITING_FOR_CALIBRATION" && source.value === "TRIAL") return t("trials.calibration.ownRejected");
  return status.value ? t(`trials.calibration.states.${status.value}`) : "";
});

async function load() {
  error.value = "";
  try {
    state.value = await trialCalibration.status(props.trial.id);
  } catch (e) {
    error.value = e.message;
  }
}
watch(() => `${props.trial.video?.id}:${props.trial.video?.status}`, load, { immediate: true });

async function context() {
  const link = await trials.videoUrl(props.trial.id);
  return { paradigm: props.paradigm, parameters: props.trial.arena.parameters || {}, videoUrl: link.url };
}
const chain = async () => (await trialCalibration.list(props.trial.id)).data;

function saved() {
  load();
  emit("saved");
}
</script>

<template>
  <NCard size="small" :title="$t('trials.calibration.title')" data-test="trial-calibration" :data-status="status || ''">
    <template #header-extra>
      <NTag v-if="status" size="small" round :bordered="false" :type="tagType(status)" data-test="trial-calibration-status">{{ fmt.status(status) }}</NTag>
    </template>
    <p class="muted" data-test="trial-calibration-source" :data-source="source || ''">{{ explanation }}</p>
    <p v-if="current" class="muted small">
      {{ $t("execution.calibration.checkError", { error: fmt.number(current.checkRmsErrorCm, 2), tolerance: fmt.number(current.toleranceCm, 2) }) }}
    </p>
    <NButton v-if="canCalibrate" size="small" secondary data-test="trial-calibrate" @click="open = true">
      {{ source === "TRIAL" ? $t("trials.calibration.correct") : $t("trials.calibration.calibrate") }}
    </NButton>
    <NAlert v-if="error" type="error" :title="error" />

    <CalibrationModal
      v-if="open"
      :title="$t('trials.calibration.dialogTitle', { name: trial.name })"
      :note="$t('trials.calibration.note')"
      :context="context"
      :chain="chain"
      :create="(body) => trialCalibration.create(trial.id, body)"
      :fallback="current"
      :on-current-video="(c) => c.videoAssetId === trial.videoId"
      :saved-message="$t('trials.calibration.saved')"
      @saved="saved"
      @close="open = false"
    />
  </NCard>
</template>

<style scoped>
p { margin: 0 0 8px; }
.small { font-size: 12px; }
</style>
