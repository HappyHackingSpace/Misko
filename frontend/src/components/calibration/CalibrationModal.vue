<script setup>
// Calibrating by hand on a video, in a dialog: the calibrations already made
// on it form one chain, so a correction names the latest; the dialog stays
// open after saving to show how the calibration came out. The owner (an
// animal of a test, a paradigm trial) supplies where the chain, the paradigm,
// the arena values and the video come from, and where a calibration goes.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NModal, NSpin, useMessage } from "naive-ui";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";
import Calibrator from "./Calibrator.vue";
import CalibrationSummary from "./CalibrationSummary.vue";

const props = defineProps({
  title: { type: String, required: true },
  note: { type: String, default: "" },
  // async () => ({ paradigm, parameters, videoUrl }): the paradigm manifest,
  // the arena's values and a signed read of the video to calibrate on.
  context: { type: Function, required: true },
  // async () => the owner's calibrations, oldest first.
  chain: { type: Function, required: true },
  // async (body) => the stored calibration, VALID or REJECTED.
  create: { type: Function, required: true },
  // The calibration in use now, to start from when the chain is empty.
  fallback: { type: Object, default: null },
  // Whether a calibration of the chain was read from the current video.
  onCurrentVideo: { type: Function, default: () => true },
  savedMessage: { type: String, required: true },
});
const emit = defineEmits(["close", "saved"]);
const { t } = useI18n();
const message = useMessage();
const fmt = useExecutionFormat();

const calibrations = ref([]);
const paradigm = ref(null);
const parameters = ref({});
const videoUrl = ref("");
const referenceCalibration = ref(null);
const loading = ref(true);
const saving = ref(false);
const error = ref("");
// Bumped after a save so the editor starts again from the new calibration.
const round = ref(0);

// The latest calibration is the one no other supersedes.
const latest = computed(() => {
  const superseded = new Set(calibrations.value.map((c) => c.supersedesId).filter(Boolean));
  return calibrations.value.findLast((c) => !superseded.has(c.id)) || null;
});
// Only a calibration read from the current video is shown as this video's result.
const latestHere = computed(() => (latest.value && props.onCurrentVideo(latest.value) ? latest.value : null));
// Camera, frame, crop, points and zones are carried over from what was tried last or is in use.
const initial = computed(() => latest.value || props.fallback || null);

async function loadChain() {
  calibrations.value = (await props.chain()) || [];
}

async function save(body) {
  saving.value = true;
  error.value = "";
  try {
    if (latest.value) body.supersedesId = latest.value.id;
    const stored = await props.create(body);
    if (stored.status === "VALID") message.success(props.savedMessage);
    else message.warning(t("execution.calibration.rejected", { error: fmt.number(stored.checkMaxErrorCm, 2), tolerance: fmt.number(stored.toleranceCm, 2) }));
    await loadChain();
    round.value++;
    emit("saved", stored);
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  try {
    const [loaded] = await Promise.all([props.context(), loadChain()]);
    paradigm.value = loaded.paradigm;
    parameters.value = loaded.parameters || {};
    videoUrl.value = loaded.videoUrl || "";
    referenceCalibration.value = loaded.referenceCalibration || (props.fallback?.scope === "ENVIRONMENT" ? props.fallback : null);
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
});
</script>

<template>
  <NModal
    :show="true"
    preset="card"
    :title="title"
    style="width: 1000px; max-width: calc(100vw - 16px)"
    data-test="calibration-dialog"
    @update:show="(v) => !v && emit('close')"
  >
    <NSpin :show="loading">
      <p v-if="note" class="muted">{{ note }}</p>
      <CalibrationSummary v-if="latestHere" :calibration="latestHere" :zones="paradigm?.zones || []" />
      <Calibrator
        v-if="!loading"
        :key="round"
        :paradigm="paradigm"
        :parameters="parameters"
        :video-url="videoUrl"
        :initial="initial"
        :reference-calibration="referenceCalibration"
        :correcting="!!latest"
        :with-reference-frame="true"
        :saving="saving"
        :disabled="saving"
        @save="save"
        @cancel="emit('close')"
      />
      <NAlert v-if="error" type="error" :title="error" data-test="calibration-error" style="margin-top: 12px" />
    </NSpin>
  </NModal>
</template>
