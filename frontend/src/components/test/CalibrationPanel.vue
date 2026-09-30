<script setup>
// The calibration of one recording: which calibration it is analyzed with, where
// that comes from, and the form to enter one for this video alone. A recording
// uses the calibration of its environment unless one was entered for it, for
// example because its camera or arena moved.
import { computed, ref } from "vue";
import { NButton, NDescriptions, NDescriptionsItem, NTag } from "naive-ui";
import CalibrationForm from "../CalibrationForm.vue";

const props = defineProps({
  recording: { type: Object, required: true },
  // The calibration status of the recording, as the API reports it.
  status: { type: Object, default: null },
  // The calibrations entered for this recording, oldest first.
  own: { type: Array, default: () => [] },
  canEnter: { type: Boolean, default: false },
  saving: { type: Boolean, default: false },
  disabled: { type: Boolean, default: false },
  tagType: { type: Function, required: true },
});
const emit = defineEmits(["save"]);

const open = ref(false);

// The latest is the one no other calibration supersedes.
const latestOwn = computed(() => {
  const superseded = new Set(props.own.map((c) => c.supersedesId).filter(Boolean));
  return props.own.findLast((c) => !superseded.has(c.id)) || null;
});
// What to show: the calibration the video is analyzed with, or, while it waits
// because its own calibration was rejected, that rejected one.
const shown = computed(() => props.status?.calibration || latestOwn.value);
const source = computed(() => props.status?.source || "");
const notRequired = computed(() => props.status?.status === "NOT_REQUIRED");
const enterable = computed(() => props.recording.video.status === "VERIFIED" && !notRequired.value);

const centimetres = (value) => `${Number(value).toFixed(2)} cm`;

function save(body) {
  // A later calibration of the same video names the one it replaces; the first
  // one entered for a video replaces nothing, even though the environment's
  // calibration was in use.
  // The form closes only once the API accepted it, so a refusal leaves the points
  // in place to be corrected.
  emit("save", { ...body, supersedesId: latestOwn.value?.id || "" }, () => (open.value = false));
}
</script>

<template>
  <div class="calibration" data-test="calibration" :data-recording="recording.id">
    <p class="muted" v-if="notRequired" data-test="calibration-not-required">{{ $t("calibration.notRequired") }}</p>
    <template v-else>
      <!-- The measured errors are what say whether the recording can be turned
           into centimetres, so they are shown rather than a badge. -->
      <NDescriptions
        v-if="shown"
        :column="2"
        label-placement="left"
        size="small"
        bordered
        data-test="calibration-summary"
        :data-calibration-status="shown.status"
        :data-calibration-source="shown.scope"
      >
        <NDescriptionsItem :label="$t('calibration.source')">
          <span data-test="calibration-source">{{ $t(`calibration.sources.${shown.scope}`) }}</span>
        </NDescriptionsItem>
        <NDescriptionsItem :label="$t('tests.status')">
          <NTag size="small" round :bordered="false" :type="tagType(shown.status)">{{ $t(`statuses.${shown.status}`) }}</NTag>
        </NDescriptionsItem>
        <NDescriptionsItem :label="$t('calibration.fitError')">
          <span data-test="fit-error">{{ centimetres(shown.fitRmsErrorCm) }}</span>
        </NDescriptionsItem>
        <NDescriptionsItem :label="$t('calibration.checkError')">
          <span data-test="check-error">{{ centimetres(shown.checkRmsErrorCm) }}</span>
        </NDescriptionsItem>
        <NDescriptionsItem :label="$t('calibration.maxError')">{{ centimetres(shown.checkMaxErrorCm) }}</NDescriptionsItem>
        <NDescriptionsItem :label="$t('calibration.tolerance')">
          <span data-test="tolerance">{{ centimetres(shown.toleranceCm) }}</span>
        </NDescriptionsItem>
        <NDescriptionsItem v-if="status?.drift" :label="$t('calibration.drift')">
          <span data-test="calibration-drift" :data-drift="status.drift">{{ $t(`calibration.drifts.${status.drift}`) }}</span>
        </NDescriptionsItem>
      </NDescriptions>
      <p v-else class="muted" data-test="calibration-none">{{ $t("calibration.none") }}</p>

      <p class="err" v-if="shown?.status === 'REJECTED'" data-test="calibration-rejected">
        {{ $t(source === "RECORDING" ? "calibration.rejectedOwn" : "calibration.rejected") }}
      </p>
      <!-- What is in use comes from the environment: say so, and say what that
           does not cover. -->
      <template v-if="source === 'ENVIRONMENT'">
        <p class="muted hint" data-test="calibration-inherited">{{ $t("calibration.inherited") }}</p>
        <p class="muted hint" data-test="calibration-drift-hint">{{ $t("calibration.driftHint") }}</p>
      </template>
      <p class="muted hint" v-else-if="!source && recording.video.status === 'VERIFIED'" data-test="calibration-needs-environment">
        {{ $t("calibration.needsEnvironment") }}
      </p>

      <NButton v-if="canEnter && enterable && !open" size="small" secondary class="enter" data-test="calibrate" @click="open = true">
        {{ $t(latestOwn ? "calibration.enter" : "calibration.enterOverride") }}
      </NButton>

      <CalibrationForm
        v-if="open"
        :initial="latestOwn || shown"
        :correcting="!!latestOwn"
        :with-reference-frame="true"
        :note="latestOwn ? '' : $t('calibration.overrideNote')"
        :saving="saving"
        :disabled="disabled"
        @save="save"
        @cancel="open = false"
      />
    </template>
  </div>
</template>

<style scoped>
.hint { margin: 8px 0 0; }
.enter { margin-top: 12px; }
</style>
