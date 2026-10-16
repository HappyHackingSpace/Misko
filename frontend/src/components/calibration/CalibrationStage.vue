<script setup>
// One frame of a video with a drawing layer on top. The layer's coordinates
// are the video's own pixels whatever size the frame is shown at, so what is
// drawn is what the API stores. The video is only ever paused on a frame: a
// scrubber and frame steps pick which one.
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { NButton, NSlider } from "naive-ui";
import { useExecutionFormat } from "../../composables/useExecutionFormat.js";
import { clamp } from "./geometry.js";

const props = defineProps({
  src: { type: String, default: "" },
  frameWidth: { type: Number, default: 0 },
  frameHeight: { type: Number, default: 0 },
  // Seconds into the video of the frame shown.
  time: { type: Number, default: 0 },
  // A frame step when the video does not say its rate.
  frameStep: { type: Number, default: 1 / 30 },
  cursor: { type: String, default: "crosshair" },
});
const emit = defineEmits(["update:time", "metadata", "press", "move", "release", "error"]);
const fmt = useExecutionFormat();

const video = ref(null);
const svg = ref(null);
const duration = ref(0);
const loaded = ref(false);
// Source pixels per screen pixel, so handles keep their size on screen.
const scale = ref(1);
let observer = null;

const width = computed(() => props.frameWidth || 0);
const height = computed(() => props.frameHeight || 0);
const ready = computed(() => width.value > 0 && height.value > 0);
// The whole frame stays in view: as wide as the page allows, at most 70% of
// the screen's height tall.
const frameStyle = computed(() =>
  ready.value ? { aspectRatio: `${width.value} / ${height.value}`, width: `min(100%, calc(70vh * ${width.value / height.value}))` } : { minHeight: "160px" },
);

function measure() {
  if (!svg.value || !width.value) return;
  const rect = svg.value.getBoundingClientRect();
  if (rect.width > 0) scale.value = width.value / rect.width;
}

function onMetadata() {
  const v = video.value;
  duration.value = Number.isFinite(v.duration) ? v.duration : 0;
  loaded.value = true;
  emit("metadata", { width: v.videoWidth, height: v.videoHeight, duration: duration.value });
  // Show the frame asked for, not the first one the browser decoded.
  seek(Math.min(props.time || 0, duration.value || 0));
}

function seek(seconds) {
  const v = video.value;
  if (!v || !loaded.value) return;
  const target = clamp(seconds, 0, duration.value || 0);
  v.currentTime = target;
  emit("update:time", target);
}

const step = (frames) => seek((video.value?.currentTime || 0) + frames * props.frameStep);

watch(() => props.src, () => {
  loaded.value = false;
  duration.value = 0;
});
watch(width, measure);

function sourcePoint(event) {
  const rect = svg.value.getBoundingClientRect();
  return {
    x: clamp(((event.clientX - rect.left) / rect.width) * width.value, 0, width.value),
    y: clamp(((event.clientY - rect.top) / rect.height) * height.value, 0, height.value),
  };
}

function handle(name, event) {
  if (!ready.value) return;
  if (name === "press") {
    if (event.button !== undefined && event.button !== 0) return;
    svg.value.setPointerCapture?.(event.pointerId);
    event.preventDefault();
  }
  emit(name, { ...sourcePoint(event), target: event.target?.dataset || {}, event });
}

onMounted(() => {
  observer = new ResizeObserver(measure);
  if (svg.value) observer.observe(svg.value);
  measure();
});
watch(svg, (el) => {
  if (el && observer) {
    observer.observe(el);
    measure();
  }
});
onBeforeUnmount(() => observer?.disconnect());
defineExpose({ scale });
</script>

<template>
  <div class="stage" data-test="calibration-stage">
    <div v-if="ready || src" class="frame" :style="frameStyle">
      <video
        v-if="src"
        ref="video"
        :src="src"
        class="video"
        preload="auto"
        muted
        playsinline
        data-test="calibration-video"
        @loadedmetadata="onMetadata"
        @error="emit('error')"
      />
      <svg
        v-if="ready"
        ref="svg"
        class="layer"
        :viewBox="`0 0 ${width} ${height}`"
        preserveAspectRatio="none"
        :style="{ cursor }"
        data-test="calibration-layer"
        @pointerdown="handle('press', $event)"
        @pointermove="handle('move', $event)"
        @pointerup="handle('release', $event)"
        @pointercancel="handle('release', $event)"
      >
        <slot :scale="scale" />
      </svg>
    </div>
    <div v-else class="placeholder muted" data-test="calibration-no-frame">
      <slot name="empty" />
    </div>

    <div v-if="src && loaded" class="scrub" data-test="calibration-scrubber">
      <NButton size="tiny" secondary data-test="frame-back" :aria-label="$t('calibration.stage.previousFrame')" @click="step(-1)">
        {{ $t("calibration.stage.previousFrame") }}
      </NButton>
      <NSlider
        class="slider"
        :value="time"
        :min="0"
        :max="duration || 0"
        :step="0.001"
        :tooltip="false"
        data-test="frame-slider"
        @update:value="seek"
      />
      <NButton size="tiny" secondary data-test="frame-forward" :aria-label="$t('calibration.stage.nextFrame')" @click="step(1)">
        {{ $t("calibration.stage.nextFrame") }}
      </NButton>
      <span class="muted time" data-test="frame-time">
        {{ $t("calibration.stage.time", { time: fmt.number(time, 3), duration: fmt.number(duration, 1) }) }}
      </span>
    </div>
  </div>
</template>

<style scoped>
.stage { display: flex; flex-direction: column; gap: 8px; min-width: 0; }
.frame {
  position: relative; width: 100%; overflow: hidden; align-self: center;
  border-radius: 8px; border: 1px solid var(--line); background: var(--panel2);
}
.video { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: fill; display: block; }
.layer { position: absolute; inset: 0; width: 100%; height: 100%; touch-action: none; user-select: none; }
.placeholder {
  display: flex; align-items: center; justify-content: center; text-align: center;
  min-height: 160px; padding: 16px; border-radius: 8px; border: 1px dashed var(--line); background: var(--panel2);
}
.scrub { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.slider { flex: 1 1 160px; min-width: 120px; }
.time { font-size: 12px; font-variant-numeric: tabular-nums; }
</style>
