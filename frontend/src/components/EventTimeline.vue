<script setup>
// Vertical, event-based activity timeline. Each item is a logged event placed in
// time order with its second (t) in a left gutter, a connector line, a colored
// node per event type, and an optional payload detail. Read-only: editing of
// events lives in the separate event list next to it.
import { computed } from "vue";

const props = defineProps({
  // [{ t:Number, type:String, label:String, detail:String }]
  items: { type: Array, default: () => [] },
});

// Stable color per distinct event type, drawn from a small palette.
const PALETTE = [
  "#4cc2ff", "#46d369", "#ffb454", "#c792ea", "#ff6b81", "#5bd1c5", "#e0c46c",
];
const colorFor = computed(() => {
  const map = {};
  let i = 0;
  for (const it of props.items) {
    if (!(it.type in map)) map[it.type] = PALETTE[i++ % PALETTE.length];
  }
  return map;
});

const ordered = computed(() =>
  [...props.items].sort((a, b) => (a.t ?? 0) - (b.t ?? 0))
);
</script>

<template>
  <ol class="timeline" v-if="ordered.length">
    <li class="event" v-for="(ev, i) in ordered" :key="i">
      <span class="time">{{ ev.t }}s</span>
      <span class="node" :style="{ '--dot': colorFor[ev.type] }"></span>
      <div class="body">
        <span class="label">{{ ev.label }}</span>
        <span class="detail" v-if="ev.detail">{{ ev.detail }}</span>
      </div>
    </li>
  </ol>
  <p v-else class="muted empty">—</p>
</template>

<style scoped>
.timeline { list-style: none; margin: 0; padding: 0; }
.event {
  display: grid;
  grid-template-columns: 48px 16px 1fr;
  align-items: start;
  gap: 8px;
  position: relative;
  padding-bottom: 14px;
}
.event:last-child { padding-bottom: 0; }
.time { font-size: 12px; color: var(--muted); text-align: right; padding-top: 1px; font-variant-numeric: tabular-nums; }
.node {
  width: 11px; height: 11px; border-radius: 50%;
  background: var(--dot, var(--txt));
  margin: 3px auto 0;
  box-shadow: 0 0 0 3px color-mix(in srgb, var(--dot, var(--txt)) 22%, transparent);
  z-index: 1;
}
/* connector line through the node column */
.event:not(:last-child)::before {
  content: "";
  position: absolute;
  left: calc(48px + 8px + 8px - 1px);
  top: 12px;
  bottom: -3px;
  width: 2px;
  background: var(--line);
}
.body { display: flex; flex-direction: column; gap: 2px; min-width: 0; }
.label { font-size: 13px; }
.detail { font-size: 12px; color: var(--muted); word-break: break-word; }
.empty { margin: 0; }
</style>
