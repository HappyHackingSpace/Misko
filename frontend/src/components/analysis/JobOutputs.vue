<script setup>
// What a succeeded job published beyond its metrics: the overlay video the
// worker drew and the events the engine derived. Clicking an event plays it
// on the overlay. Loaded when shown, from the analysis service's job routes.
import { computed, onMounted, ref } from "vue";
import { jobs } from "../../api/analysis.js";
import EventTimeline from "../EventTimeline.vue";

const props = defineProps({
  jobId: { type: String, required: true },
  // Event labels by type in the panel's language.
  eventLabels: { type: Object, default: () => ({}) },
});

const job = ref(null);
const url = ref("");
const error = ref("");
const loading = ref(true);
const player = ref(null);
const playing = ref(null);
let stopAt = null;
let refreshed = false;

const events = computed(() => job.value?.result?.events || []);
// Overlay time of analysed time zero (the clip start).
const offset = computed(() => (job.value?.result?.pair?.outputOffsetUs || 0) / 1_000_000);

async function loadUrl() {
  url.value = (await jobs.outputUrl(props.jobId, "ANALYZED_VIDEO")).url;
}

onMounted(async () => {
  try {
    job.value = await jobs.get(props.jobId);
    await loadUrl();
  } catch (e) {
    error.value = e.message;
  } finally {
    loading.value = false;
  }
});

// A signed read expires; ask for a fresh one once.
async function onError() {
  if (refreshed) return;
  refreshed = true;
  try {
    await loadUrl();
  } catch (e) {
    error.value = e.message;
  }
}

async function play(event) {
  playing.value = event;
  const video = player.value;
  if (!video) return;
  video.currentTime = offset.value + event.startUs / 1_000_000;
  stopAt = event.endUs > event.startUs ? offset.value + event.endUs / 1_000_000 : null;
  if (stopAt == null) return video.pause();
  try {
    await video.play();
  } catch {
    // Autoplay can be refused; the viewer can still press play.
  }
}

function onTime() {
  if (stopAt != null && player.value.currentTime >= stopAt - 0.001) {
    stopAt = null;
    player.value.pause();
  }
}
</script>

<template>
  <div class="outputs" data-test="job-outputs">
    <p v-if="loading" class="muted">{{ $t("common.loading") }}</p>
    <p v-else-if="error" class="muted" data-test="job-outputs-error">{{ error }}</p>
    <template v-else>
      <figure v-if="url">
        <video ref="player" :src="url" controls playsinline muted preload="metadata" data-test="overlay-video" @timeupdate="onTime" @error="onError"></video>
        <figcaption>{{ $t("analysis.analyzed") }}</figcaption>
      </figure>
      <div>
        <h5>{{ $t("analysis.events") }} ({{ events.length }})</h5>
        <EventTimeline :events="events" :selected="playing" :labels="eventLabels" @play="play" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.outputs { display: grid; grid-template-columns: minmax(0, 3fr) minmax(0, 2fr); gap: 12px; }
figure { margin: 0; display: flex; flex-direction: column; gap: 4px; }
video { width: 100%; max-height: 320px; object-fit: contain; background: #000; border-radius: 10px; }
figcaption, h5 { font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: .04em; margin: 0 0 4px; }
@media (max-width: 760px) { .outputs { grid-template-columns: minmax(0, 1fr); } }
</style>
