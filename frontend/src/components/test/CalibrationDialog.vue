<script setup>
// Calibrates one animal's arena by hand on the session's current video, for
// when the camera or the arena moved since the arena's default calibration.
// It overrides the default for this video only. The animal's overrides form
// one chain over the session's videos, so a correction names the latest.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NModal, NSpin, useMessage } from "naive-ui";
import { calibration } from "../../api/media.js";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";
import CalibrationForm from "../CalibrationForm.vue";

const props = defineProps({
  testId: { type: String, required: true },
  subject: { type: Object, required: true },
  // The animal's calibration status, carrying the calibration in use.
  status: { type: Object, default: null },
});
const emit = defineEmits(["close", "saved"]);
const { t } = useI18n();
const message = useMessage();
const fmt = useExecutionFormat();

const overrides = ref([]);
const loading = ref(true);
const saving = ref(false);
const error = ref("");

// The latest override is the one no other supersedes.
const latest = computed(() => {
  const superseded = new Set(overrides.value.map((c) => c.supersedesId).filter(Boolean));
  return overrides.value.findLast((c) => !superseded.has(c.id)) || null;
});
// Camera, frame and crop are carried over from what is in use or was tried last.
const initial = computed(() => latest.value || props.status?.calibration || null);

async function save(body) {
  saving.value = true;
  error.value = "";
  try {
    if (latest.value) body.supersedesId = latest.value.id;
    const stored = await calibration.createOverride(props.testId, props.subject.id, body);
    if (stored.status === "VALID") {
      message.success(t("execution.calibration.saved", { animal: props.subject.subjectCode }));
    } else {
      message.warning(t("execution.calibration.rejected", { error: fmt.number(stored.checkMaxErrorCm, 2), tolerance: fmt.number(stored.toleranceCm, 2) }));
    }
    emit("saved");
  } catch (e) {
    error.value = e.message;
  } finally {
    saving.value = false;
  }
}

onMounted(async () => {
  try {
    overrides.value = (await calibration.overrides(props.testId, props.subject.id)).data || [];
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
    style="width: 760px; max-width: 96vw"
    data-test="calibration-dialog"
    @update:show="(v) => !v && emit('close')"
  >
    <NSpin :show="loading">
      <p class="muted">{{ $t("execution.calibration.overrideNote") }}</p>
      <CalibrationForm
        v-if="!loading"
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
