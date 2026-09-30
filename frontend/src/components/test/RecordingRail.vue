<script setup>
// The videos of a test, as a short list to choose from. A test can hold several
// recordings, each with its own calibration and analysis, so the page shows one
// at a time and this is where it is chosen. Each entry carries one line that says
// what stands between the video and its result.
defineProps({
  recordings: { type: Array, default: () => [] },
  selectedId: { type: String, default: "" },
  // Per recording id: { stage, tone }, see TestDetail.
  stages: { type: Object, default: () => ({}) },
  canAdd: { type: Boolean, default: false },
});
const emit = defineEmits(["select", "add"]);
</script>

<template>
  <div class="rail">
    <button v-if="canAdd" type="button" class="add" data-test="upload-open" @click="emit('add')">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.6" stroke-linecap="round"><line x1="12" y1="5" x2="12" y2="19" /><line x1="5" y1="12" x2="19" y2="12" /></svg>
      {{ $t("tests.addVideo") }}
    </button>

    <ul v-if="recordings.length" class="list" data-test="recordings">
      <li
        v-for="(recording, index) in recordings"
        :key="recording.id"
        data-test="recording"
        :data-recording="recording.id"
        :data-filename="recording.video.fileName"
        :data-video-status="recording.video.status"
        :data-selected="recording.id === selectedId"
      >
        <button
          type="button"
          class="item"
          :class="{ active: recording.id === selectedId }"
          :title="recording.video.fileName || recording.video.id"
          @click="emit('select', recording.id)"
        >
          <span class="name">{{ $t("tests.videoN", { n: index + 1 }) }}</span>
          <span class="stage" :data-tone="stages[recording.id]?.tone" :data-stage="stages[recording.id]?.stage">
            <span class="dot"></span>{{ stages[recording.id] ? $t(`tests.stage.${stages[recording.id].stage}`) : "" }}
          </span>
        </button>
      </li>
    </ul>
    <p v-else class="muted empty">{{ $t("tests.noRecordings") }}</p>
  </div>
</template>

<style scoped>
.rail { display: flex; flex-direction: column; gap: 10px; min-width: 0; }
.add { display: flex; align-items: center; justify-content: center; gap: 8px; width: 100%; padding: 8px 12px; border-radius: 10px; }
.list { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; }
.item {
  display: flex; flex-direction: column; align-items: stretch; gap: 3px; width: 100%; text-align: left;
  padding: 9px 12px; border-radius: 10px; font-weight: 400; background: var(--panel); border: 1px solid var(--line);
}
.item:hover { border-color: var(--accent); }
.item.active { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 10%, var(--panel)); }
.name { font-weight: 700; font-size: 14px; }
.stage { display: flex; align-items: center; gap: 6px; font-size: 12px; color: var(--muted); }
.dot { width: 8px; height: 8px; border-radius: 50%; background: var(--muted); flex-shrink: 0; }
.stage[data-tone="good"] .dot { background: var(--good); }
.stage[data-tone="warn"] .dot { background: var(--warn); }
.stage[data-tone="bad"] .dot { background: var(--bad); }
.stage[data-tone="info"] .dot { background: var(--accent); }
.empty { margin: 4px 2px; }
</style>
