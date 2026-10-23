<script setup>
// The trial's video: upload it straight to storage, see it verified and play
// it. A new upload replaces it; earlier analyses keep the video they used.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NPopconfirm, NTag, useMessage } from "naive-ui";
import { trials, uploadTrialVideo } from "../../api/trials.js";
import { tagType, useExecutionFormat } from "../../composables/useExecutionFormat.js";
import { useVideoUpload } from "../../composables/useVideoUpload.js";
import UploadProgress from "../UploadProgress.vue";

const props = defineProps({
  // The trial detail: its id and video.
  trial: { type: Object, required: true },
  canRun: { type: Boolean, default: false },
});
// "changed" carries the trial detail after an upload.
const emit = defineEmits(["changed"]);
const { t } = useI18n();
const message = useMessage();
const fmt = useExecutionFormat();
const sending = useVideoUpload((file, progress) => uploadTrialVideo(props.trial.id, file, progress));
const { stage, progress } = sending;

const input = ref(null);
const url = ref("");
const error = ref("");
const video = computed(() => props.trial.video);
const verified = computed(() => video.value?.status === "VERIFIED");
const reason = (v) => t(`execution.video.reasons.${v.rejectionReason || "UNKNOWN"}`);

async function loadUrl() {
  url.value = "";
  if (!verified.value) return;
  try {
    url.value = (await trials.videoUrl(props.trial.id)).url;
  } catch (e) {
    error.value = e.message;
  }
}
// A signed read expires; a player error asks for a fresh one once per video.
let refreshed = false;
function onPlayerError() {
  if (refreshed) return;
  refreshed = true;
  loadUrl();
}
watch(
  () => video.value?.id + video.value?.status,
  () => {
    refreshed = false;
    loadUrl();
  },
  { immediate: true },
);

async function upload(event) {
  const file = event.target.files?.[0];
  event.target.value = "";
  if (!file) return;
  error.value = "";
  const detail = await sending.start(file);
  if (!detail) {
    error.value = sending.error.value;
    return;
  }
  if (detail.video?.status === "VERIFIED") message.success(t("trials.video.verified"));
  else message.error(t("execution.video.rejected", { reason: reason(detail.video || {}) }));
  emit("changed", detail);
}

const choose = () => input.value?.click();
</script>

<template>
  <NCard size="small" :title="$t('trials.video.title')" data-test="trial-video">
    <template #header-extra>
      <template v-if="canRun">
        <NPopconfirm v-if="video" @positive-click="choose">
          <template #trigger>
            <NButton size="small" secondary :loading="!!stage" :disabled="!!stage" data-test="trial-video-replace">{{ $t("trials.video.replace") }}</NButton>
          </template>
          {{ $t("trials.video.replaceConfirm") }}
        </NPopconfirm>
        <NButton v-else size="small" type="primary" :loading="!!stage" :disabled="!!stage" data-test="trial-video-upload" @click="choose">
          {{ $t("trials.video.upload") }}
        </NButton>
      </template>
      <input ref="input" type="file" accept="video/mp4,video/quicktime,video/webm" class="hidden" data-test="trial-video-input" @change="upload" />
    </template>

    <UploadProgress v-if="stage" :stage="stage" :progress="progress" />

    <div v-if="video" class="current" data-test="trial-video-current" :data-status="video.status">
      <div class="line">
        <strong class="file" :title="video.fileName">{{ video.fileName }}</strong>
        <NTag size="small" round :bordered="false" :type="tagType(video.status)">{{ fmt.status(video.status) }}</NTag>
      </div>
      <p class="muted small">{{ fmt.bytes(video.sizeBytes) }} · {{ $t("execution.video.uploadedAt", { at: fmt.dateTime(video.createdAt) }) }}</p>
      <NAlert v-if="video.status === 'REJECTED'" type="error" :show-icon="false">{{ $t("execution.video.rejected", { reason: reason(video) }) }}</NAlert>
      <video v-if="url" :key="url" class="player" :src="url" controls playsinline preload="metadata" data-test="trial-video-player" @error="onPlayerError" />
    </div>
    <p v-else-if="!stage" class="muted" data-test="trial-video-none">{{ $t("trials.video.none") }}</p>

    <NAlert v-if="error" type="error" :title="error" data-test="trial-video-error" />
  </NCard>
</template>

<style scoped>
.hidden { display: none; }
.current { display: flex; flex-direction: column; gap: 6px; min-width: 0; }
.line { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; min-width: 0; }
.file { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.small { font-size: 12px; margin: 0; }
.player { width: 100%; max-height: 360px; border-radius: 10px; background: #000; }
</style>
