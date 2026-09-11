<script setup>
// Side panel that plays one analysis run: the original recording and the
// analyzed video of that run, on the same timeline.
//
// Times come from the API as microseconds from the recording (clip) start. The
// pair says how to map them: source time = sourceOffsetUs + recording time,
// output time = outputOffsetUs + recording time. Playing an event seeks both
// videos to its start and stops at its end.
//
// Signed URLs expire. When one does, the panel fetches a new pair and restores
// the position it was at, so a refresh does not send the viewer back to zero.
import { computed, onBeforeUnmount, ref, watch } from "vue";
import { analysis } from "../api/endpoints.js";

const props = defineProps({
  runId: { type: String, required: true },
  // The event to play, or null to leave the videos where they are.
  play: { type: Object, default: null },
});
const emit = defineEmits(["error"]);

const pair = ref(null);
const original = ref(null);
const analyzed = ref(null);
const loading = ref(false);
const showAnalyzed = ref(true);
let stopAt = null;

const sourceOffset = computed(() => (pair.value?.sourceOffsetUs ?? 0) / 1_000_000);
const outputOffset = computed(() => (pair.value?.outputOffsetUs ?? 0) / 1_000_000);

async function loadPair(keepPosition = false) {
  const position = keepPosition ? analyzed.value?.currentTime : null;
  loading.value = true;
  try {
    pair.value = await analysis.videoPair(props.runId);
    if (position != null) {
      // Restore after the new sources have loaded their metadata.
      await Promise.resolve();
      restore(position);
    }
  } catch (error) {
    emit("error", error.message);
  } finally {
    loading.value = false;
  }
}

function restore(position) {
  for (const video of [original.value, analyzed.value]) {
    if (!video) continue;
    const apply = () => (video.currentTime = position);
    if (video.readyState >= 1) apply();
    else video.addEventListener("loadedmetadata", apply, { once: true });
  }
}

// A media error usually means the signed URL expired; ask for a fresh pair once.
let refreshing = false;
async function onMediaError() {
  if (refreshing) return;
  refreshing = true;
  await loadPair(true);
  refreshing = false;
}

function seek(recordingUs) {
  const recordingSeconds = recordingUs / 1_000_000;
  if (original.value) original.value.currentTime = sourceOffset.value + recordingSeconds;
  if (analyzed.value) analyzed.value.currentTime = outputOffset.value + recordingSeconds;
}

function onTimeUpdate() {
  if (stopAt == null || !analyzed.value) return;
  if (analyzed.value.currentTime >= stopAt - 0.001) {
    stopAt = null;
    analyzed.value.pause();
    original.value?.pause();
  }
}

async function playEvent(event) {
  if (!event) return;
  if (!pair.value) await loadPair();
  seek(event.startUs);
  // A point event has no duration: seek to it and hold still.
  stopAt = event.endUs > event.startUs ? outputOffset.value + event.endUs / 1_000_000 : null;
  if (stopAt == null) {
    analyzed.value?.pause();
    original.value?.pause();
    return;
  }
  try {
    await Promise.all([analyzed.value?.play(), original.value?.play()].filter(Boolean));
  } catch {
    // Autoplay can be refused; the viewer can still press play.
  }
}

watch(() => props.runId, () => loadPair(), { immediate: true });
watch(() => props.play, (event) => playEvent(event));
onBeforeUnmount(() => {
  stopAt = null;
});

defineExpose({ playEvent, seek });
</script>

<template>
  <div class="panel" data-test="video-pair">
    <div class="head">
      <h5>{{ $t("analysis.pairedVideo") }}</h5>
      <label class="toggle">
        <input type="checkbox" v-model="showAnalyzed" data-test="toggle-analyzed" />
        {{ $t("analysis.showAnalyzed") }}
      </label>
    </div>
    <p v-if="loading && !pair" class="muted">{{ $t("common.loading") }}</p>
    <div v-else-if="pair" class="videos">
      <figure>
        <video
          ref="original"
          :src="pair.originalUrl"
          data-test="original-video"
          preload="metadata"
          controls
          playsinline
          muted
          @error="onMediaError"
        ></video>
        <figcaption>{{ $t("analysis.original") }}</figcaption>
      </figure>
      <figure v-show="showAnalyzed">
        <video
          ref="analyzed"
          :src="pair.analyzedUrl"
          data-test="analyzed-video"
          preload="metadata"
          controls
          playsinline
          muted
          @timeupdate="onTimeUpdate"
          @error="onMediaError"
        ></video>
        <figcaption>{{ $t("analysis.analyzed") }}</figcaption>
      </figure>
    </div>
  </div>
</template>

<style scoped>
.panel { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.head { display: flex; align-items: center; justify-content: space-between; gap: 10px; }
.head h5 { margin: 0; color: var(--muted); font-size: 12px; text-transform: uppercase; letter-spacing: .04em; }
.toggle { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
.videos { display: flex; flex-direction: column; gap: 10px; }
figure { margin: 0; display: flex; flex-direction: column; gap: 4px; }
video { width: 100%; background: #000; border-radius: 10px; }
figcaption { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; }
</style>
