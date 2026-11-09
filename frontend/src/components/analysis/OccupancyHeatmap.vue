<script setup>
// Where the animal spent its time: tracked seconds per cell of the arena
// floor, computed by the API from the published trajectory, with the
// calibrated zones drawn over it. Paradigms filmed from the side have no floor
// and show nothing.
import { computed, ref, watch } from "vue";
import { useI18n } from "vue-i18n";
import { jobs } from "../../api/analysis.js";
import { useNumbers } from "./format.js";

const props = defineProps({
  jobId: { type: String, required: true },
  // The job's calibrated zones; their centimetre shapes are outlined.
  zones: { type: Array, default: () => [] },
  zoneLabel: { type: Function, default: (key) => key },
});
const { t } = useI18n();
const fmt = useNumbers();

const grid = ref(null);
const error = ref("");

watch(
  () => props.jobId,
  async (id) => {
    grid.value = null;
    error.value = "";
    try {
      grid.value = await jobs.occupancy(id);
    } catch (e) {
      if (e.code !== "analysis.noArenaFloor") error.value = e.message;
    }
  },
  { immediate: true },
);

const max = computed(() => Math.max(0, ...(grid.value?.seconds || [])));
const box = computed(() => {
  const g = grid.value;
  return g && { x: g.minXCm, y: g.minYCm, width: g.columns * g.cellCm, height: g.rows * g.cellCm };
});
// Opacity grows with the square root so short visits stay visible next to
// the corner the animal sat in.
const cells = computed(() => {
  const g = grid.value;
  if (!g || !max.value) return [];
  return g.seconds.flatMap((seconds, i) => {
    if (!seconds) return [];
    const column = i % g.columns;
    const row = Math.floor(i / g.columns);
    return [{ i, seconds, x: g.minXCm + column * g.cellCm, y: g.minYCm + row * g.cellCm, opacity: Math.sqrt(seconds / max.value) }];
  });
});
const outlines = computed(() =>
  props.zones.flatMap((zone, i) => {
    const shape = zone.centimeters || {};
    if (shape.circle) return [{ id: `${zone.key}-${i}`, key: zone.key, circle: shape.circle }];
    if (shape.polygon?.length) return [{ id: `${zone.key}-${i}`, key: zone.key, points: shape.polygon.map((p) => `${p.x},${p.y}`).join(" ") }];
    return [];
  }),
);
const seconds = (value) => fmt.metric(value, "s");
</script>

<template>
  <figure v-if="grid" class="heatmap" data-test="occupancy-heatmap" :data-max="max">
    <svg
      :viewBox="`${box.x} ${box.y} ${box.width} ${box.height}`"
      preserveAspectRatio="xMidYMid meet"
      role="img"
      :aria-label="t('analysis.heatmap.aria', { max: seconds(max) })"
    >
      <rect class="floor" :x="box.x" :y="box.y" :width="box.width" :height="box.height" />
      <rect
        v-for="cell in cells"
        :key="cell.i"
        class="cell"
        :x="cell.x"
        :y="cell.y"
        :width="grid.cellCm"
        :height="grid.cellCm"
        :fill-opacity="cell.opacity"
        data-test="occupancy-cell"
      >
        <title>{{ seconds(cell.seconds) }}</title>
      </rect>
      <g class="zones">
        <template v-for="zone in outlines" :key="zone.id">
          <circle v-if="zone.circle" :cx="zone.circle.x" :cy="zone.circle.y" :r="zone.circle.radius" data-test="occupancy-zone">
            <title>{{ zoneLabel(zone.key) }}</title>
          </circle>
          <polygon v-else :points="zone.points" data-test="occupancy-zone">
            <title>{{ zoneLabel(zone.key) }}</title>
          </polygon>
        </template>
      </g>
    </svg>
    <figcaption>
      <span>{{ t("analysis.heatmap.title") }}</span>
      <span class="scale" aria-hidden="true">
        <span>0</span><span class="ramp"></span><span>{{ seconds(max) }}</span>
      </span>
      <span class="muted">{{ t("analysis.heatmap.cell", { size: fmt.metric(grid.cellCm, "cm") }) }}</span>
    </figcaption>
  </figure>
  <p v-else-if="error" class="muted" data-test="occupancy-error">{{ error }}</p>
</template>

<style scoped>
/* One hue, light to dark: the blue of the sequential ramp over the panel. */
.heatmap { --heat: #2a78d6; margin: 0; display: flex; flex-direction: column; gap: 6px; }
svg { width: 100%; max-height: 320px; }
.floor { fill: var(--panel2); stroke: var(--line); stroke-width: 1; vector-effect: non-scaling-stroke; }
.cell { fill: var(--heat); }
.zones circle, .zones polygon { fill: none; stroke: var(--txt); stroke-width: 1.5; stroke-dasharray: 4 3; vector-effect: non-scaling-stroke; }
figcaption { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; font-size: 11.5px; color: var(--muted); }
figcaption > span:first-child { text-transform: uppercase; letter-spacing: .04em; font-size: 11px; }
.scale { display: inline-flex; align-items: center; gap: 6px; font-variant-numeric: tabular-nums; }
.ramp { width: 80px; height: 8px; border-radius: 4px; background: linear-gradient(to right, color-mix(in srgb, var(--heat) 0%, var(--panel2)), var(--heat)); border: 1px solid var(--line); }
</style>
