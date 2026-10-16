<script setup>
// Calibrates one animal's arena by hand on the session's current video, for
// when the camera or the arena moved since the arena's default calibration.
// It overrides the default for this video only. The animal's overrides form
// one chain over the session's videos, so a correction names the latest.
// The dialog stays open after saving to show how the calibration came out.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NModal, NSpin, useMessage } from "naive-ui";
import { calibration, recordings } from "../../api/media.js";
import { environments, paradigmCatalog } from "../../api/catalog.js";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";
import Calibrator from "../calibration/Calibrator.vue";
import CalibrationSummary from "../calibration/CalibrationSummary.vue";

const props = defineProps({
  // The test: its id and paradigm version.
  test: { type: Object, required: true },
  subject: { type: Object, required: true },
  // The animal's calibration status, carrying the calibration in use and the current video.
  status: { type: Object, default: null },
});
const emit = defineEmits(["close", "saved"]);
const { t } = useI18n();
const message = useMessage();
const fmt = useExecutionFormat();

const overrides = ref([]);
const paradigm = ref(null);
const parameters = ref({});
const videoUrl = ref("");
const loading = ref(true);
const saving = ref(false);
const error = ref("");
// Bumped after a save so the editor starts again from the new calibration.
const round = ref(0);

// The latest override is the one no other supersedes.
const latest = computed(() => {
  const superseded = new Set(overrides.value.map((c) => c.supersedesId).filter(Boolean));
  return overrides.value.findLast((c) => !superseded.has(c.id)) || null;
});
// Only an override read from the current video is shown as this video's result.
const latestHere = computed(() => (latest.value && latest.value.recordingId === props.status?.recordingId ? latest.value : null));
// Camera, frame, crop, points and zones are carried over from what is in use or was tried last.
const initial = computed(() => latest.value || props.status?.calibration || null);

async function loadOverrides() {
  overrides.value = (await calibration.overrides(props.test.id, props.subject.id)).data || [];
}

async function save(body) {
  saving.value = true;
  error.value = "";
  try {
    if (latest.value) body.supersedesId = latest.value.id;
    const stored = await calibration.createOverride(props.test.id, props.subject.id, body);
    if (stored.status === "VALID") {
      message.success(t("execution.calibration.saved", { animal: props.subject.subjectCode }));
    } else {
      message.warning(t("execution.calibration.rejected", { error: fmt.number(stored.checkMaxErrorCm, 2), tolerance: fmt.number(stored.toleranceCm, 2) }));
    }
    await loadOverrides();
    round.value++;
    emit("saved");
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  try {
    const [, manifest, revisions, link] = await Promise.all([
      loadOverrides(),
      paradigmCatalog.version(props.test.paradigmKey, props.test.paradigmVersion),
      environments.revisions(props.subject.environmentId),
      props.status?.recordingId ? recordings.readUrl(props.test.id, props.status.recordingId) : null,
    ]);
    paradigm.value = manifest;
    parameters.value = (revisions.data || []).find((r) => r.number === props.subject.environmentRevision)?.parameters || {};
    videoUrl.value = link?.url || "";
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
    :title="$t('execution.calibration.dialogTitle', { animal: subject.subjectCode, arena: subject.environmentName })"
    style="width: 1000px; max-width: calc(100vw - 16px)"
    data-test="calibration-dialog"
    @update:show="(v) => !v && emit('close')"
  >
    <NSpin :show="loading">
      <p class="muted">{{ $t("execution.calibration.overrideNote") }}</p>
      <CalibrationSummary v-if="latestHere" :calibration="latestHere" :zones="paradigm?.zones || []" />
      <Calibrator
        v-if="!loading"
        :key="round"
        :paradigm="paradigm"
        :parameters="parameters"
        :video-url="videoUrl"
        :initial="initial"
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
