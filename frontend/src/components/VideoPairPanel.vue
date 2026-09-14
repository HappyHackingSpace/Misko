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
import { computed, nextTick, onBeforeUnmount, ref, watch } from "vue";
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
// Where the reader last was, in analyzed video time. A player whose source
// fails resets itself to zero before it reports the error, so by then its own
// position is gone; this is what a refreshed pair is restored to.
let lastPosition = 0;

const sourceOffset = computed(() => (pair.value?.sourceOffsetUs ?? 0) / 1_000_000);
const outputOffset = computed(() => (pair.value?.outputOffsetUs ?? 0) / 1_000_000);

async function loadPair(keepPosition = false) {
  const position = keepPosition ? lastPosition : null;
  loading.value = true;
  try {
    pair.value = await analysis.videoPair(props.runId);
    // Wait for the new sources to be on the elements. Before that the players
    // still hold the old ones, and a position set now would be thrown away.
    await nextTick();
    // Then ask for the load outright. Writing the src attribute is meant to
    // restart a player, but WebKit does not always act on it when the element
    // already holds a source: it stays at readyState 0 and never plays, so
    // switching runs on Safari or an iPad shows a frame that never moves.
    for (const video of [original.value, analyzed.value]) video?.load();
    if (position != null) restore(position);
  } catch (error) {
    emit("error", error.message);
  } finally {
    loading.value = false;
  }
}

function restore(position) {
  for (const video of [original.value, analyzed.value]) {
    if (!video) continue;
    // The new source reports its duration on loadedmetadata, and only then does
    // a position stick. Seeking a player that is still loading is ignored, so
    // the position is applied again once the metadata is in.
    const apply = () => (video.currentTime = position);
    if (video.readyState >= 1) apply();
    video.addEventListener("loadedmetadata", apply, { once: true });
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
  lastPosition = outputOffset.value + recordingSeconds;
  if (analyzed.value) analyzed.value.currentTime = lastPosition;
}

// Remembers where the reader is. A player that has lost its source reports zero,
// which is why the position is kept here rather than read back when it is needed.
function rememberPosition() {
  if (analyzed.value && analyzed.value.readyState > 0) lastPosition = analyzed.value.currentTime;
}

function onTimeUpdate() {
  if (!analyzed.value) return;
  rememberPosition();
  if (stopAt == null) return;
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

watch(
  () => props.runId,
  () => {
    // Another run is another pair of videos: nothing of the last one carries over.
    stopAt = null;
    lastPosition = 0;
    loadPair();
  },
  { immediate: true },
);
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
          @seeked="rememberPosition"
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
