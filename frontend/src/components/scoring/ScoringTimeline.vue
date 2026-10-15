<script setup>
// The scored events of one animal on the length of the clip, one lane per
// event type and label: points as ticks, intervals as bars, intervals still
// running as a bar up to the playhead. Clicking the track seeks; clicking an
// event selects it and seeks to its start.
import { computed } from "vue";

const props = defineProps({
  // [{ key, title }] in the order of the scoring buttons.
  lanes: { type: Array, required: true },
  // [{ id, lane, kind, startUs, endUs, invalid }]
  events: { type: Array, required: true },
  // [{ lane, startUs }]
  running: { type: Array, default: () => [] },
  durationUs: { type: Number, default: 0 },
  currentUs: { type: Number, default: 0 },
  selected: { type: String, default: "" },
});
const emit = defineEmits(["seek", "select"]);

const span = computed(() => Math.max(props.durationUs, 1));
const pct = (us) => `${Math.min(Math.max(us / span.value, 0), 1) * 100}%`;
const width = (from, to) => `${(Math.min(Math.max(to, from), span.value) - Math.min(from, span.value)) / span.value * 100}%`;

function seek(event) {
  const rect = event.currentTarget.getBoundingClientRect();
  const fraction = Math.min(Math.max((event.clientX - rect.left) / rect.width, 0), 1);
  emit("seek", Math.round(fraction * props.durationUs));
}

const ofLane = (key) => props.events.filter((e) => e.lane === key);
const runningOf = (key) => props.running.filter((r) => r.lane === key);
</script>

<template>
  <div class="timeline" data-test="scoring-timeline">
    <div v-for="lane in lanes" :key="lane.key" class="lane" :data-lane="lane.key">
      <span class="lane-title" :title="lane.title">{{ lane.title }}</span>
      <div class="track" @click="seek">
        <template v-for="e in ofLane(lane.key)" :key="e.id">
          <span
            v-if="e.kind === 'INTERVAL'"
            class="bar"
            :class="{ selected: e.id === selected, invalid: e.invalid }"
            :style="{ left: pct(e.startUs), width: width(e.startUs, e.endUs) }"
            data-test="timeline-event"
            @click.stop="emit('select', e.id)"
          ></span>
          <span
            v-else
            class="tick"
            :class="{ selected: e.id === selected, invalid: e.invalid }"
            :style="{ left: pct(e.startUs) }"
            data-test="timeline-event"
            @click.stop="emit('select', e.id)"
          ></span>
        </template>
        <span
          v-for="r in runningOf(lane.key)"
          :key="`run-${r.lane}`"
          class="bar running"
          :style="{ left: pct(r.startUs), width: width(r.startUs, currentUs) }"
          data-test="timeline-running"
        ></span>
        <span class="playhead" :style="{ left: pct(currentUs) }"></span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.timeline { display: flex; flex-direction: column; gap: 6px; }
.lane { display: grid; grid-template-columns: minmax(70px, 160px) 1fr; gap: 8px; align-items: center; min-width: 0; }
.lane-title { font-size: 12px; color: var(--muted); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.track { position: relative; height: 22px; border: 1px solid var(--line); border-radius: 6px; background: var(--panel2); cursor: pointer; overflow: hidden; }
.bar { position: absolute; z-index: 1; top: 3px; bottom: 3px; min-width: 2px; border-radius: 3px; background: var(--accent); opacity: 0.7; }
.tick { position: absolute; z-index: 1; top: 2px; bottom: 2px; width: 5px; margin-left: -2px; border-radius: 2px; background: var(--accent); }
.bar.selected, .tick.selected { background: var(--good); opacity: 1; }
.bar.invalid, .tick.invalid { background: var(--bad); }
.bar.running { background: repeating-linear-gradient(45deg, var(--warn), var(--warn) 4px, transparent 4px, transparent 8px); opacity: 1; pointer-events: none; }
.playhead { position: absolute; top: 0; bottom: 0; width: 1px; background: var(--muted); pointer-events: none; }
</style>
