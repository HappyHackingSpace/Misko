<script setup>
// The one video of a session. Every animal of the session is analysed from it,
// each in its own arena. Uploading again replaces it: the new video is used
// once it is verified, and the earlier ones stay listed for provenance.
import { computed, onMounted, ref } from "vue";
import { useI18n } from "vue-i18n";
import { NAlert, NButton, NCard, NCollapse, NCollapseItem, NPopconfirm, NTag, useMessage } from "naive-ui";
import { recordings, uploadVideo } from "../../api/media.js";
import { tagType, useExecutionFormat } from "../../composables/useExecutionFormat.js";
import { useVideoUpload } from "../../composables/useVideoUpload.js";
import UploadProgress from "../UploadProgress.vue";

const props = defineProps({
  test: { type: Object, required: true },
  canUpload: { type: Boolean, default: false },
});
const emit = defineEmits(["changed"]);
const { t } = useI18n();
const message = useMessage();
const fmt = useExecutionFormat();

const list = ref([]); // oldest first
const error = ref("");
const sending = useVideoUpload((file, progress) => uploadVideo(props.test.id, file, progress));
const { stage, progress } = sending;
const playing = ref(null); // { recordingId, url }
const input = ref(null);

// The video in use is the newest verified one; a newer upload that is still
// pending or was rejected does not replace it.
const inUse = computed(() => list.value.findLast((r) => r.video.status === "VERIFIED") || null);
const newest = computed(() => list.value[list.value.length - 1] || null);
const pendingReplacement = computed(() => (newest.value && newest.value !== inUse.value ? newest.value : null));
const earlier = computed(() => list.value.filter((r) => r !== inUse.value && r !== pendingReplacement.value).reverse());
const started = computed(() => ["IN_PROGRESS", "COMPLETED"].includes(props.test.status));

async function load() {
  error.value = "";
  try {
    list.value = (await recordings.list(props.test.id)).data || [];
  } catch (e) {
    error.value = e.message;
  }
}

function choose() {
  input.value?.click();
}

async function upload(event) {
  const file = event.target.files?.[0];
  event.target.value = "";
  if (!file) return;
  error.value = "";
  const recording = await sending.start(file);
  if (!recording) {
    await load();
    error.value = sending.error.value;
    return;
  }
  if (recording.video.status === "VERIFIED") message.success(t("execution.video.verified"));
  else message.error(t("execution.video.rejected", { reason: t(`execution.video.reasons.${recording.video.rejectionReason || "UNKNOWN"}`) }));
  playing.value = null;
  await load();
  emit("changed");
}

async function play(recording) {
  error.value = "";
  try {
    const { url } = await recordings.readUrl(props.test.id, recording.id);
    playing.value = { recordingId: recording.id, url };
  } catch (e) {
    error.value = e.message;
  }
}

onMounted(load);
defineExpose({ load });
</script>

<template>
  <NCard size="small" :bordered="true" :title="$t('execution.video.title')" data-test="video-panel">
    <template #header-extra>
      <template v-if="canUpload && started">
        <NPopconfirm v-if="inUse" @positive-click="choose">
          <template #trigger>
            <NButton size="small" secondary :loading="!!stage" :disabled="!!stage" data-test="video-replace">{{ $t("execution.video.replace") }}</NButton>
          </template>
          {{ $t("execution.video.replaceConfirm") }}
        </NPopconfirm>
        <NButton v-else size="small" type="primary" :loading="!!stage" :disabled="!!stage" data-test="video-upload" @click="choose">
          {{ $t("execution.video.upload") }}
        </NButton>
      </template>
      <input ref="input" type="file" accept="video/mp4,video/quicktime,video/webm" class="hidden" data-test="video-input" @change="upload" />
    </template>

    <p v-if="!started" class="muted" data-test="video-not-started">{{ $t("execution.video.notStarted") }}</p>

    <UploadProgress v-if="stage" :stage="stage" :progress="progress" />

    <div v-if="inUse" class="current" data-test="video-current" :data-recording="inUse.id">
      <div class="line">
        <strong class="file" :title="inUse.video.fileName">{{ inUse.video.fileName }}</strong>
        <NTag size="small" round :bordered="false" :type="tagType(inUse.video.status)">{{ fmt.status(inUse.video.status) }}</NTag>
      </div>
      <p class="muted small">
        {{ fmt.bytes(inUse.video.sizeBytes) }} · {{ $t("execution.video.uploadedAt", { at: fmt.dateTime(inUse.createdAt) }) }}
      </p>
      <NButton v-if="playing?.recordingId !== inUse.id" size="small" quaternary data-test="video-play" @click="play(inUse)">▶ {{ $t("execution.video.play") }}</NButton>
    </div>
    <p v-else-if="started && !stage" class="muted" data-test="video-none">{{ $t("execution.video.none") }}</p>

    <NAlert v-if="pendingReplacement" :type="pendingReplacement.video.status === 'REJECTED' ? 'error' : 'warning'" :show-icon="false" data-test="video-replacement">
      <strong>{{ pendingReplacement.video.fileName }}</strong> ·
      <template v-if="pendingReplacement.video.status === 'REJECTED'">
        {{ $t("execution.video.rejected", { reason: $t(`execution.video.reasons.${pendingReplacement.video.rejectionReason || "UNKNOWN"}`) }) }}
      </template>
      <template v-else>{{ $t("execution.video.pending") }}</template>
    </NAlert>

    <video v-if="playing" :key="playing.url" class="player" :src="playing.url" controls preload="metadata" data-test="video-player" />

    <NAlert v-if="error" type="error" :title="error" data-test="video-error" />

    <NCollapse v-if="earlier.length" class="earlier">
      <NCollapseItem :title="$t('execution.video.earlier', { count: earlier.length })" name="earlier">
        <div v-for="recording in earlier" :key="recording.id" class="line earlier-row" data-test="video-earlier">
          <span class="file">{{ recording.video.fileName }}</span>
          <span class="muted small">{{ fmt.dateTime(recording.createdAt) }}</span>
          <NTag size="small" round :bordered="false" :type="tagType(recording.video.status)">{{ fmt.status(recording.video.status) }}</NTag>
        </div>
      </NCollapseItem>
    </NCollapse>
  </NCard>
</template>

<style scoped>
.hidden { display: none; }
.current { display: flex; flex-direction: column; align-items: flex-start; gap: 4px; margin-bottom: 10px; }
.line { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; min-width: 0; }
.file { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; max-width: 100%; }
.small { font-size: 12px; margin: 0; }
.player { width: 100%; max-height: 420px; border-radius: 10px; background: var(--panel2); margin: 8px 0; }
.earlier { margin-top: 10px; }
.earlier-row { padding: 4px 0; }
</style>
