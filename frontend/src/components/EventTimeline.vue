<script setup>
// Published events of one analysis run, in time order. Clicking an event asks
// the paired player to play exactly that interval; the selected event stays
// highlighted. Keyboard users reach every event with Tab and start it with
// Enter or Space, because each row is a button.
import { computed } from "vue";
import { useI18n } from "vue-i18n";
import { eventColors } from "../composables/eventColors.js";

const props = defineProps({
  // [{ type, kind, startUs, endUs, confidence, trialId }]
  events: { type: Array, default: () => [] },
  selected: { type: Object, default: null },
  // Only these types are listed; null lists every type.
  types: { type: Array, default: null },
});
const emit = defineEmits(["play"]);
const { t, te } = useI18n();

const colorFor = computed(() => eventColors(props.events));

const ordered = computed(() =>
  props.events
    .filter((event) => !props.types || props.types.includes(event.type))
    .sort((a, b) => a.startUs - b.startUs || a.type.localeCompare(b.type)),
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
.timeline {
  list-style: none;
  margin: 0;
  padding: 4px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 240px;
  overflow-y: auto;
  border: 1px solid var(--line);
  border-radius: 10px;
  background: var(--panel2);
}
.event {
  display: grid;
  grid-template-columns: 10px 1fr auto auto;
  align-items: center;
  gap: 10px;
  width: 100%;
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: 7px;
  padding: 6px 9px;
  color: var(--txt);
  cursor: pointer;
  font-size: 12.5px;
}
.event:hover { background: var(--panel); }
.event.selected { border-color: var(--dot); background: color-mix(in srgb, var(--dot) 14%, var(--panel)); }
.event:focus-visible { outline: 2px solid var(--dot); outline-offset: 1px; }
.node { width: 8px; height: 8px; border-radius: 50%; background: var(--dot); }
.label { font-weight: 600; }
.range { color: var(--muted); font-variant-numeric: tabular-nums; font-size: 11.5px; }
.confidence { color: var(--muted); font-variant-numeric: tabular-nums; font-size: 10.5px; min-width: 30px; text-align: right; }
</style>
