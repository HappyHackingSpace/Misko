<script setup>
// Calibrates one animal's arena by hand on the session's current video, for
// when the camera or the arena moved since the arena's default calibration.
// It overrides the default for this video only.
import { useI18n } from "vue-i18n";
import { calibration, recordings } from "../../api/media.js";
import { environments, paradigmCatalog } from "../../api/catalog.js";
import CalibrationModal from "../calibration/CalibrationModal.vue";

const props = defineProps({
  // The test: its id and paradigm version.
  test: { type: Object, required: true },
  subject: { type: Object, required: true },
  // The animal's calibration status, carrying the calibration in use and the current video.
  status: { type: Object, default: null },
});
const emit = defineEmits(["close", "saved"]);
const { t } = useI18n();

async function context() {
  const [paradigm, revisions, link] = await Promise.all([
    paradigmCatalog.version(props.test.paradigmKey, props.test.paradigmVersion),
    environments.revisions(props.subject.environmentId),
    props.status?.recordingId ? recordings.readUrl(props.test.id, props.status.recordingId) : null,
  ]);
  return {
    paradigm,
    parameters: (revisions.data || []).find((r) => r.number === props.subject.environmentRevision)?.parameters || {},
    videoUrl: link?.url || "",
  };
}
const chain = async () => (await calibration.overrides(props.test.id, props.subject.id)).data;
</script>

<template>
  <CalibrationModal
    :title="t('execution.calibration.dialogTitle', { animal: subject.subjectCode, arena: subject.environmentName })"
    :note="t('execution.calibration.overrideNote')"
    :context="context"
    :chain="chain"
    :create="(body) => calibration.createOverride(test.id, subject.id, body)"
    :fallback="status?.calibration || null"
    :on-current-video="(c) => c.recordingId === status?.recordingId"
    :saved-message="t('execution.calibration.saved', { animal: subject.subjectCode })"
    @close="emit('close')"
    @saved="emit('saved')"
  />
</template>
