<script setup>
// Group means with SEM whiskers, one bar per group, coloured by the group's
// role so controls and treatments read apart. Plain SVG drawn with the theme
// colours, so it follows light and dark mode and scales to a phone.
import { computed } from "vue";

const props = defineProps({
  // [{ id, code, name, role, mean, sem, n }]
  bars: { type: Array, default: () => [] },
  // Formats a value with its unit for labels and the axis.
  format: { type: Function, required: true },
  label: { type: String, default: "" },
});

const H = 220;
const TOP = 18;
const BOTTOM = 40;
const LEFT = 68;
const STEP = 76;
const BAR = 40;
const width = computed(() => LEFT + Math.max(props.bars.length, 1) * STEP + 8);
const plotted = computed(() => props.bars.filter((b) => typeof b.mean === "number"));
const range = computed(() => {
  let lo = 0;
  let hi = 0;
  for (const b of plotted.value) {
    const sem = b.sem || 0;
    lo = Math.min(lo, b.mean - sem);
    hi = Math.max(hi, b.mean + sem);
  }
  if (hi === lo) hi = lo + 1;
  return { lo, hi: hi + (hi - lo) * 0.08 };
});
const y = (value) => TOP + ((range.value.hi - value) / (range.value.hi - range.value.lo)) * (H - TOP - BOTTOM);
const ticks = computed(() => {
  const { lo, hi } = range.value;
  return [0, 0.25, 0.5, 0.75, 1].map((share) => lo + (hi - lo) * share);
});
const ROLE_COLOR = {
  HEALTHY_CONTROL: "var(--good)",
  SHAM: "var(--muted)",
  DISEASE_CONTROL: "var(--bad)",
  POSITIVE_CONTROL: "var(--warn)",
  TREATMENT: "var(--accent)",
};
const items = computed(() =>
  props.bars.map((b, i) => {
    const x = LEFT + i * STEP + (STEP - BAR) / 2;
    const has = typeof b.mean === "number";
    const top = has ? Math.min(y(b.mean), y(0)) : y(0);
    const height = has ? Math.max(1, Math.abs(y(b.mean) - y(0))) : 0;
    return { ...b, x, cx: x + BAR / 2, has, top, height, color: ROLE_COLOR[b.role] || "var(--accent)" };
  }),
);
</script>

<template>
  <figure class="group-chart" data-test="group-chart">
    <svg :viewBox="`0 0 ${width} ${H}`" role="img" :aria-label="label" preserveAspectRatio="xMinYMin meet" :style="{ maxWidth: `${width * 1.4}px` }">
      <g class="grid">
        <g v-for="tick in ticks" :key="tick">
          <line :x1="LEFT" :x2="width - 4" :y1="y(tick)" :y2="y(tick)" />
          <text :x="LEFT - 6" :y="y(tick) + 3" text-anchor="end">{{ format(tick) }}</text>
        </g>
        <line class="zero" :x1="LEFT" :x2="width - 4" :y1="y(0)" :y2="y(0)" />
      </g>
      <g v-for="bar in items" :key="bar.id" :data-test="`chart-bar-${bar.code}`">
        <title>{{ bar.code }} · {{ bar.name }}: {{ bar.has ? format(bar.mean) : "–" }} (n={{ bar.n }})</title>
        <rect v-if="bar.has" :x="bar.x" :y="bar.top" :width="BAR" :height="bar.height" rx="4" :style="{ fill: bar.color }" class="bar" />
        <g v-if="bar.has && bar.sem" class="whisker" :style="{ stroke: bar.color }">
          <line :x1="bar.cx" :x2="bar.cx" :y1="y(bar.mean + bar.sem)" :y2="y(bar.mean - bar.sem)" />
          <line :x1="bar.cx - 7" :x2="bar.cx + 7" :y1="y(bar.mean + bar.sem)" :y2="y(bar.mean + bar.sem)" />
          <line :x1="bar.cx - 7" :x2="bar.cx + 7" :y1="y(bar.mean - bar.sem)" :y2="y(bar.mean - bar.sem)" />
        </g>
        <text class="code" :x="bar.cx" :y="H - BOTTOM + 15" text-anchor="middle">{{ bar.code }}</text>
        <text class="n" :x="bar.cx" :y="H - BOTTOM + 29" text-anchor="middle">n={{ bar.n }}</text>
      </g>
    </svg>
    <figcaption v-if="label">{{ label }}</figcaption>
  </figure>
</template>

<style scoped>
.group-chart { margin: 0; min-width: 0; }
svg { width: 100%; height: auto; display: block; }
.grid line { stroke: var(--line); stroke-width: 1; }
.grid line.zero { stroke: var(--muted); }
.grid text, .n { fill: var(--muted); font-size: 10px; font-variant-numeric: tabular-nums; }
.code { fill: var(--txt); font-size: 11.5px; font-weight: 600; }
.bar { opacity: 0.75; }
.whisker line { stroke-width: 1.6; }
figcaption { font-size: 11.5px; color: var(--muted); margin-top: 4px; }
</style>
