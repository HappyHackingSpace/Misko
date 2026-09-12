<script setup>
// Published events of one analysis run, in time order. Clicking an event asks
// the paired player to play exactly that interval; the selected event stays
// highlighted. Keyboard users reach every event with Tab and start it with
// Enter or Space, because each row is a button.
import { computed } from "vue";
import { useI18n } from "vue-i18n";

const props = defineProps({
  // [{ type, kind, startUs, endUs, confidence, trialId }]
  events: { type: Array, default: () => [] },
  selected: { type: Object, default: null },
});
const emit = defineEmits(["play"]);
const { t, te } = useI18n();

const PALETTE = ["#4cc2ff", "#46d369", "#ffb454", "#c792ea", "#ff6b81", "#5bd1c5", "#e0c46c"];

const colorFor = computed(() => {
  const map = {};
  let i = 0;
  for (const event of props.events) {
    if (!(event.type in map)) map[event.type] = PALETTE[i++ % PALETTE.length];
  }
  return map;
});

const ordered = computed(() =>
  [...props.events].sort((a, b) => a.startUs - b.startUs || a.type.localeCompare(b.type)),
);

// "Movement 00:12-00:18" reads the same way in both languages.
function clock(us) {
  const total = Math.max(0, Math.round(us / 1_000_000));
  return `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
}

const label = (type) => (te(`events.${type}`) ? t(`events.${type}`) : type);
const range = (event) => (event.kind === "POINT" ? clock(event.startUs) : `${clock(event.startUs)}-${clock(event.endUs)}`);
const isSelected = (event) =>
  props.selected && props.selected.startUs === event.startUs && props.selected.type === event.type;
</script>

<template>
  <ol class="timeline" v-if="ordered.length" data-test="event-timeline">
    <li v-for="(event, i) in ordered" :key="`${event.type}-${event.startUs}-${i}`">
      <button
        type="button"
        class="event"
        :class="{ selected: isSelected(event) }"
        :style="{ '--dot': colorFor[event.type] }"
        :aria-pressed="isSelected(event)"
        data-test="event"
        :data-event-type="event.type"
        :data-start-us="event.startUs"
        :data-end-us="event.endUs"
        @click="emit('play', event)"
      >
        <span class="node"></span>
        <span class="label">{{ label(event.type) }}</span>
        <span class="range">{{ range(event) }}</span>
        <span class="confidence">{{ Math.round(event.confidence * 100) }}%</span>
      </button>
    </li>
  </ol>
  <p v-else class="muted">{{ $t("analysis.noEvents") }}</p>
</template>

<style scoped>
.timeline { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
.event {
  display: grid;
  grid-template-columns: 14px 1fr auto auto;
  align-items: center;
  gap: 10px;
  width: 100%;
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 9px;
  padding: 7px 10px;
  color: var(--txt);
  cursor: pointer;
}
.event:hover { background: var(--panel2); }
.event.selected { border-color: var(--dot); background: color-mix(in srgb, var(--dot) 12%, transparent); }
.event:focus-visible { outline: 2px solid var(--dot); outline-offset: 1px; }
.node { width: 11px; height: 11px; border-radius: 50%; background: var(--dot); }
.label { font-size: 13px; }
.range { font-size: 12px; color: var(--muted); font-variant-numeric: tabular-nums; }
.confidence { font-size: 11px; color: var(--muted); font-variant-numeric: tabular-nums; }
</style>
